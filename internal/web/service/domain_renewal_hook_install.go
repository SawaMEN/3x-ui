package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

const certificateHookMarker = "# 3x-ui managed certificate renewal firewall hook"

func certificateHookShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func certificateHookScript(executable, action, manager string) string {
	return "#!/bin/sh\n" + certificateHookMarker + "\nset -eu\nexec " +
		certificateHookShellQuote(executable) + " certificate-hook " + action + " " + manager + "\n"
}

// Never overwrite an administrator-owned script or symlink. Only replace
// scripts carrying our explicit marker, using an atomic rename.
func ensureCertificateHookScript(path, script string) error {
	if old, err := os.Lstat(path); err == nil {
		if !old.Mode().IsRegular() {
			return fmt.Errorf("renewal hook path is not a regular file: %s", path)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !strings.Contains(string(data), certificateHookMarker) {
			return fmt.Errorf("refusing to overwrite external renewal hook: %s", path)
		}
		if string(data) == script && old.Mode().Perm() == 0o750 {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".3x-ui-renewal-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o750); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(script); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func removeOwnedCertificateHookScript(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(string(data), certificateHookMarker) {
		return os.Remove(path)
	}
	return nil
}

// acme.sh stores per-certificate hooks in its domain .conf. We only fill
// empty fields or update our own hooks; operator hooks are never replaced.
func patchACMESHRenewalHooks(raw, pre, post string) (string, bool, error) {
	lines := strings.Split(raw, "\n")
	current := map[string]string{}
	for _, line := range lines {
		for _, key := range []string{"Le_PreHook", "Le_PostHook"} {
			if strings.HasPrefix(line, key+"=") {
				current[key] = strings.Trim(strings.TrimPrefix(line, key+"="), "'")
			}
		}
	}
	if (current["Le_PreHook"] != "" && current["Le_PreHook"] != pre) ||
		(current["Le_PostHook"] != "" && current["Le_PostHook"] != post) {
		return raw, false, errors.New("existing acme.sh renewal hooks are administrator-owned")
	}
	if strings.ContainsAny(pre+post, "'\n\r") {
		return raw, false, errors.New("unsafe acme.sh hook filename")
	}
	want := map[string]string{"Le_PreHook": pre, "Le_PostHook": post}
	found := map[string]bool{}
	for i, line := range lines {
		for key, value := range want {
			if strings.HasPrefix(line, key+"=") {
				lines[i] = key + "='" + value + "'"
				found[key] = true
			}
		}
	}
	for _, key := range []string{"Le_PreHook", "Le_PostHook"} {
		if !found[key] {
			lines = append(lines, key+"='"+want[key]+"'")
		}
	}
	next := strings.Join(lines, "\n")
	return next, next != raw, nil
}

func writeACMESHRenewalHooks(state domainACME, pre, post string) error {
	info, err := os.Lstat(state.path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("acme.sh config is not a regular file: %s", state.path)
	}
	raw, err := os.ReadFile(state.path)
	if err != nil {
		return err
	}
	next, changed, err := patchACMESHRenewalHooks(string(raw), pre, post)
	if err != nil || !changed {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(state.path), ".3x-ui-acme-hook-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(next); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), state.path)
}

// InstallExternalCertificateRenewalHooks is idempotent. It manages only
// certificate hooks for locally referenced Certbot/acme.sh certificates,
// preserving all third-party scripts and custom certificate hooks.
func InstallExternalCertificateRenewalHooks() error {
	if os.Geteuid() != 0 {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	status, err := DomainFrontStatus()
	if err != nil {
		return err
	}
	var telemt *TelemtWebProxyState
	if state, readErr := readTelemtWebState(); readErr == nil && state.Enabled {
		telemt = &state
	}
	states := domainACMEStates(domainACMEHomes())
	items := makeCertificateInventory(status, states, telemt)
	local := map[string]bool{}
	certbot := false
	for _, item := range items {
		if item.Source == "remote" || item.CertFile == "" {
			continue
		}
		switch item.Manager {
		case "certbot":
			certbot = true
		case "acme.sh":
			local[item.CertFile] = true
		}
	}
	for _, action := range []string{"pre", "post"} {
		path := filepath.Join("/etc/letsencrypt/renewal-hooks", action, "3x-ui-certificate-firewall")
		if certbot {
			if err := ensureCertificateHookScript(path, certificateHookScript(exe, map[string]string{"pre": "begin", "post": "end"}[action], "certbot")); err != nil {
				return err
			}
		} else if err := removeOwnedCertificateHookScript(path); err != nil {
			return err
		}
	}
	if len(local) == 0 {
		return nil
	}
	dir := filepath.Join(config.GetDBFolderPath(), "certificate-renewal-hooks")
	prePath := filepath.Join(dir, "acme-pre")
	postPath := filepath.Join(dir, "acme-post")
	if err := ensureCertificateHookScript(prePath, certificateHookScript(exe, "begin", "acme.sh")); err != nil {
		return err
	}
	if err := ensureCertificateHookScript(postPath, certificateHookScript(exe, "end", "acme.sh")); err != nil {
		return err
	}
	for _, state := range states {
		if !local[state.values["Le_RealFullChainPath"]] {
			continue
		}
		if err := writeACMESHRenewalHooks(state, prePath, postPath); err != nil {
			// Existing hooks can contain arbitrary administrator automation;
			// report this instead of replacing or appending shell code.
			logger.Warning("acme.sh certificate hook not installed for ", state.path, ": ", err)
		}
	}
	return nil
}

func maintainCertificateRenewalHookInstall(ctxDone <-chan struct{}) {
	if err := InstallExternalCertificateRenewalHooks(); err != nil {
		logger.Warning("certificate renewal hook setup:", err)
	}
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctxDone:
			return
		case <-ticker.C:
			if err := InstallExternalCertificateRenewalHooks(); err != nil {
				logger.Warning("certificate renewal hook refresh:", err)
			}
		}
	}
}
