package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type telemtWebReloadEnvelope struct {
	OK bool `json:"ok"`
	Data struct {
		ReloadID uint64 `json:"reload_id"`
		State string `json:"state"`
	} `json:"data"`
}

// User-only WEB profile changes can be activated by Telemt without restarting
// the process. Changes to admission capacity remain restart-only.
func (s TelemtService) SaveWebUsersHotReload(cfg TelemtConfig) error {
	rendered, err := renderTelemtConfig(cfg)
	if err != nil {
		return err
	}
	if err := ensureTelemtConfig(); err != nil {
		return err
	}
	previous, err := os.ReadFile(telemtConfigPath)
	if err != nil {
		return err
	}
	if bytes.Equal(previous, []byte(rendered)) {
		return nil
	}
	if webProfileCapacity(previous) != webProfileCapacity([]byte(rendered)) {
		return s.SaveConfig(cfg)
	}

	temp := telemtConfigPath + ".users.tmp"
	if err := os.WriteFile(temp, []byte(rendered), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, telemtConfigPath); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if systemctl("is-active", "--quiet", telemtServiceName) != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := telemtWebReloadRuntime(ctx); err == nil {
		return nil
	} else {
		// Restore the exact on-disk snapshot if a new user/profile list fails.
		rollbackTmp := filepath.Join(filepath.Dir(telemtConfigPath), ".telemt-web-rollback.tmp")
		if writeErr := os.WriteFile(rollbackTmp, previous, 0o600); writeErr != nil {
			return fmt.Errorf("Telemt hot reload rejected; failed to restore old config: %w", writeErr)
		}
		if renameErr := os.Rename(rollbackTmp, telemtConfigPath); renameErr != nil {
			return fmt.Errorf("Telemt hot reload rejected; failed to restore old config: %w", renameErr)
		}
		recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer recoveryCancel()
		if recoveryErr := telemtWebReloadRuntime(recoveryCtx); recoveryErr != nil {
			// Last resort: keep the old service available.
			if restartErr := systemctl("restart", telemtServiceName); restartErr != nil {
				return fmt.Errorf("Telemt hot reload failed (%v); rollback reload failed (%v); restart failed: %w", err, recoveryErr, restartErr)
			}
		}
		return fmt.Errorf("Telemt user/profile reload failed; previous configuration restored: %w", err)
	}
}

func webProfileCapacity(config []byte) string {
	for _, line := range strings.Split(string(config), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "limits.max_profiles =") || strings.HasPrefix(line, "limits.max_vhosts =") {
			// Parse both fields; this small key-only comparison cannot mistake
			// profile users or unrelated payload for a capacity change.
			_ = line
		}
	}
	var maxProfiles, maxVhosts string
	for _, line := range strings.Split(string(config), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "limits.max_profiles =") {
			maxProfiles = line
		}
		if strings.HasPrefix(line, "limits.max_vhosts =") {
			maxVhosts = line
		}
	}
	return maxProfiles + "|" + maxVhosts
}

func telemtWebReloadRuntime(ctx context.Context) error {
	client := &http.Client{Timeout: 5 * time.Second, Transport: telemtMonitorTransport{ctx}}
	perform := func(method, path string) (int, telemtWebReloadEnvelope, error) {
		req, err := telemtAPIRequest(method, path)
		if err != nil {
			return 0, telemtWebReloadEnvelope{}, err
		}
		resp, err := client.Do(req.WithContext(ctx))
		if err != nil {
			return 0, telemtWebReloadEnvelope{}, err
		}
		defer resp.Body.Close()
		var body telemtWebReloadEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return resp.StatusCode, body, err
		}
		return resp.StatusCode, body, nil
	}
	code, accepted, err := perform(http.MethodPost, "/v1/system/reload")
	if err != nil || (code != http.StatusAccepted && code != http.StatusOK) || !accepted.OK {
		return fmt.Errorf("Telemt WEB reload rejected: status %d: %w", code, errors.Join(err, errors.New("invalid reload response")))
	}
	if accepted.Data.ReloadID == 0 {
		if code == http.StatusOK {
			return nil
		}
		return errors.New("Telemt did not return a reload operation ID")
	}
	for {
		code, status, err := perform(http.MethodGet, "/v1/system/reload/"+strconv.FormatUint(accepted.Data.ReloadID, 10))
		if err != nil || code != http.StatusOK || !status.OK {
			return fmt.Errorf("Telemt WEB reload status unavailable: HTTP %d, %v", code, err)
		}
		switch strings.ToLower(status.Data.State) {
		case "succeeded":
			return nil
		case "failed", "rolled_back":
			return fmt.Errorf("Telemt WEB reload operation %d %s", accepted.Data.ReloadID, status.Data.State)
		case "accepted", "preparing", "activating", "draining":
		default:
			return fmt.Errorf("unknown Telemt reload state %q", status.Data.State)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
