package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

var (
	domainCertificateRenewalMu sync.Mutex
	safeCertificateName        = regexp.MustCompile(`^(?:[A-Za-z0-9*][A-Za-z0-9.*_-]*)$`)
)

type DomainCertificateRenewalResult struct {
	ID        string `json:"id"`
	Manager   string `json:"manager"`
	Finished  bool   `json:"finished"`
	CheckedAt int64  `json:"checkedAt"`
}

// The ID comes from a locally discovered certificate, never from an arbitrary
// path or shell command supplied by a caller.
func localCertificateRenewalPlan(id string, status domainfront.DomainFrontStatus, states []domainACME, telemt *TelemtWebProxyState) (string, []string, []int, string, error) {
	items := makeCertificateInventory(status, states, telemt)
	for _, item := range items {
		if item.ID != id || item.CertFile == "" {
			continue
		}
		manager, challenge := item.Manager, item.Challenge
		if challenge == "unknown" || challenge == "http-01+tls-alpn-01" {
			return "", nil, nil, "", fmt.Errorf("certificate has no supported external challenge configuration")
		}
		ports := item.Ports
		switch manager {
		case "acme.sh":
			for _, state := range states {
				if state.values["Le_RealFullChainPath"] != item.CertFile {
					continue
				}
				domain := state.values["Le_Domain"]
				if !safeCertificateName.MatchString(domain) || strings.HasPrefix(domain, "-") {
					return "", nil, nil, "", errors.New("invalid acme.sh domain metadata")
				}
				home := filepath.Dir(filepath.Dir(state.path))
				binary := filepath.Join(home, "acme.sh")
				if _, err := os.Stat(binary); err != nil {
					return "", nil, nil, "", fmt.Errorf("acme.sh executable not found in %s", home)
				}
				args := []string{"--renew", "-d", domain, "--home", home}
				if strings.HasSuffix(filepath.Base(filepath.Dir(state.path)), "_ecc") {
					args = append(args, "--ecc")
				}
				return binary, args, ports, manager, nil
			}
		case "certbot":
			name := filepath.Base(filepath.Dir(item.CertFile))
			if !safeCertificateName.MatchString(name) || strings.HasPrefix(name, "-") || strings.Contains(name, "*") {
				return "", nil, nil, "", errors.New("invalid certbot certificate name")
			}
			if filepath.Clean(item.CertFile) != filepath.Join("/etc/letsencrypt/live", name, "fullchain.pem") {
				return "", nil, nil, "", errors.New("certificate is outside the local certbot store")
			}
			binary, err := exec.LookPath("certbot")
			if err != nil {
				return "", nil, nil, "", fmt.Errorf("certbot executable is unavailable: %w", err)
			}
			return binary, []string{"renew", "--cert-name", name, "--non-interactive", "--quiet"}, ports, manager, nil
		default:
			return "", nil, nil, "", errors.New("only local acme.sh or certbot certificates support a manual renewal")
		}
	}
	return "", nil, nil, "", errors.New("local certificate not found")
}

func RenewDomainCertificate(ctx context.Context, id string) (DomainCertificateRenewalResult, error) {
	if id == "" || len(id) > 128 {
		return DomainCertificateRenewalResult{}, errors.New("invalid certificate identifier")
	}
	domainCertificateRenewalMu.Lock()
	defer domainCertificateRenewalMu.Unlock()
	status, err := DomainFrontStatus()
	if err != nil {
		return DomainCertificateRenewalResult{}, err
	}
	states := domainACMEStates(domainACMEHomes())
	var telemt *TelemtWebProxyState
	if state, stateErr := readTelemtWebState(); stateErr == nil && state.Enabled {
		telemt = &state
	}
	binary, args, ports, manager, err := localCertificateRenewalPlan(id, status, states, telemt)
	if err != nil {
		return DomainCertificateRenewalResult{}, err
	}
	audit := func(outcome string) {
		if auditErr := RecordCertificateAudit(manager, id, "", "renew", outcome); auditErr != nil {
			logger.Debug("certificate renewal audit failed:", auditErr)
		}
	}
	audit("started")
	timeout, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	if err := WithRenewalFirewall(timeout, manager+":"+id, ports, func(operationCtx context.Context) error {
		// Never use a shell. Command output can contain account credentials;
		// do not expose it in API responses or panel logs.
		cmd := exec.CommandContext(operationCtx, binary, args...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s renewal command failed: %w", manager, err)
		}
		return nil
	}); err != nil {
		audit("failed")
		return DomainCertificateRenewalResult{}, err
	}
	audit("succeeded")
	return DomainCertificateRenewalResult{
		ID: id, Manager: manager, Finished: true, CheckedAt: time.Now().Unix(),
	}, nil
}

const domainRenewalAttemptsKey = "domainExternalRenewalAttempts"

// Only local, known certbot/acme.sh configurations can be run automatically.
// Their native renewal policies (including ACME Renewal Information) decide
// whether a certificate is actually due. At most one attempt per day per cert.
func runDueDomainCertificateRenewals(ctx context.Context) {
	status, err := DomainFrontStatus()
	if err != nil {
		logger.Debug("certificate renewal scan failed:", err)
		return
	}
	var telemt *TelemtWebProxyState
	if state, stateErr := readTelemtWebState(); stateErr == nil && state.Enabled {
		telemt = &state
	}
	items := makeCertificateInventory(status, domainACMEStates(domainACMEHomes()), telemt)
	attempts := map[string]int64{}
	raw, err := firewallSetting(domainRenewalAttemptsKey, "{}")
	if err != nil {
		return
	}
	if err := json.Unmarshal([]byte(raw), &attempts); err != nil {
		return
	}
	now := time.Now()
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		if (item.Manager != "certbot" && item.Manager != "acme.sh") ||
			item.ExpiresAt == 0 || item.ExpiresAt > now.Add(30*24*time.Hour).Unix() ||
			item.Challenge == "unknown" {
			continue
		}
		if now.Unix()-attempts[item.ID] < int64((24 * time.Hour).Seconds()) {
			continue
		}
		attempts[item.ID] = now.Unix()
		if err := saveFirewallJSON(domainRenewalAttemptsKey, attempts); err != nil {
			logger.Error("persist certificate renewal attempt:", err)
			return
		}
		if _, err := RenewDomainCertificate(ctx, item.ID); err != nil {
			logger.Error("automatic external certificate renewal failed:", err)
		}
	}
}

func RunDomainCertificateRenewalScheduler(ctx context.Context) {
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			runDueDomainCertificateRenewals(ctx)
			timer.Reset(12 * time.Hour)
		}
	}
}
