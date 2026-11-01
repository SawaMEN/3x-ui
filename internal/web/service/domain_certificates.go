package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

type domainACME struct {
	listener bool
	path     string
	values   map[string]string
}

type domainACMEBackup struct {
	Original string `json:"original"`
	Webroot  string `json:"webroot"`
}

var acmeAssignment = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)='([^']*)'$`)

func readDomainACME(path string) (domainACME, error) {
	raw, err := os.ReadFile(path)
	state := domainACME{path: path, values: map[string]string{}}
	if err != nil {
		return state, err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if fields := acmeAssignment.FindStringSubmatch(line); fields != nil {
			state.values[fields[1]] = fields[2]
		}
	}
	return state, nil
}

func domainACMEHomes() []string {
	home, _ := os.UserHomeDir()
	if home == "" {
		if current, err := user.Current(); err == nil {
			home = current.HomeDir
		}
	}
	homes := []string{filepath.Join(home, ".acme.sh")}
	for _, name := range []string{"LE_CONFIG_HOME", "LE_WORKING_DIR"} {
		if path := os.Getenv(name); filepath.IsAbs(path) {
			homes = append(homes, path)
		}
	}
	return homes
}

func domainACMEStates(homes []string) []domainACME {
	var states []domainACME
	seen := map[string]bool{}
	for _, home := range homes {
		entries, _ := os.ReadDir(home)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), "_ecc")
			path := filepath.Join(home, entry.Name(), name+".conf")
			if seen[path] {
				continue
			}
			seen[path] = true
			if state, err := readDomainACME(path); err == nil {
				states = append(states, state)
			}
		}
	}
	return states
}

func acmeScheduled(homes []string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "crontab", "-l").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "--cron") {
			continue
		}
		line = strings.NewReplacer("\"", "", "'", "").Replace(line)
		for _, home := range homes {
			if strings.Contains(line, filepath.Join(home, "acme.sh")) {
				return true
			}
		}
	}
	return false
}

func inheritedDomainCertificates(cfg domainfront.DomainFrontConfig, settings *entity.AllSetting, states []domainACME, scheduled bool) map[string]domainfront.CertificateFiles {
	candidates := []domainfront.CertificateFiles{
		{CertFile: settings.SubCertFile, KeyFile: settings.SubKeyFile, Source: "subscription", Renewal: "external"},
		{CertFile: settings.WebCertFile, KeyFile: settings.WebKeyFile, Source: "panel", Renewal: "external"},
	}
	for _, state := range states {
		candidates = append(candidates, domainfront.CertificateFiles{CertFile: state.values["Le_RealFullChainPath"], KeyFile: state.values["Le_RealKeyPath"], Source: "installer", Renewal: "acme.sh"})
	}
	result := map[string]domainfront.CertificateFiles{}
	for _, d := range cfg.Domains {
		if d.TLS != "auto" && d.TLS != "existing" {
			continue
		}
		for _, candidate := range candidates {
			if candidate.CertFile == "" || candidate.KeyFile == "" {
				continue
			}
			pair, err := tls.LoadX509KeyPair(candidate.CertFile, candidate.KeyFile)
			if err != nil {
				continue
			}
			cert, err := x509.ParseCertificate(pair.Certificate[0])
			if err != nil || cert.VerifyHostname(d.Name) != nil {
				continue
			}
			for _, state := range states {
				if filepath.Clean(state.values["Le_RealFullChainPath"]) == filepath.Clean(candidate.CertFile) && filepath.Clean(state.values["Le_RealKeyPath"]) == filepath.Clean(candidate.KeyFile) {
					candidate.Renewal, candidate.AutoRenew = "acme.sh", scheduled
					if !scheduled {
						candidate.Warning = "acme_schedule_missing"
					}
				}
			}
			result[strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d.Name)), ".")] = candidate
			break
		}
	}
	return result
}

func (s *SettingService) domainCertificateFiles(cfg domainfront.DomainFrontConfig) (map[string]domainfront.CertificateFiles, []domainACME, error) {
	settings, err := s.GetAllSetting()
	if err != nil {
		return nil, nil, err
	}
	homes := domainACMEHomes()
	states := domainACMEStates(homes)
	for i := range states {
		cert, key := states[i].values["Le_RealFullChainPath"], states[i].values["Le_RealKeyPath"]
		states[i].listener = cert != "" && key != "" && (cert == settings.WebCertFile && key == settings.WebKeyFile || cert == settings.SubCertFile && key == settings.SubKeyFile)
	}
	return inheritedDomainCertificates(cfg, settings, states, acmeScheduled(homes)), states, nil
}

func writeACMEWebroot(state domainACME, root string) error {
	raw, err := os.ReadFile(state.path)
	if err != nil {
		return err
	}
	line := "Le_Webroot='" + root + "'"
	if strings.ContainsAny(root, "'\r\n") {
		return fmt.Errorf("invalid ACME webroot path")
	}
	lines := strings.Split(string(raw), "\n")
	found := false
	for i, value := range lines {
		if strings.HasPrefix(value, "Le_Webroot=") {
			lines[i], found = line, true
		}
	}
	if !found {
		return fmt.Errorf("ACME configuration has no Le_Webroot: %s", state.path)
	}
	info, err := os.Stat(state.path)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(state.path), ".xui-acme-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.WriteString(strings.Join(lines, "\n")); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), state.path)
}

func prepareDomainACME(cfg domainfront.DomainFrontConfig, files map[string]domainfront.CertificateFiles, states []domainACME) (map[string]string, func(), error) {
	roots := map[string]string{}
	var undo []func()
	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}
	root := filepath.Join(config.GetDBFolderPath(), "domain-acme-webroot")
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, rollback, err
	}
	root = absolute
	for _, state := range states {
		needed := cfg.Enabled && state.listener
		for _, d := range cfg.Domains {
			file, ok := files[d.Name]
			if cfg.Enabled && d.Enabled && ok && file.Renewal == "acme.sh" && file.CertFile == state.values["Le_RealFullChainPath"] {
				needed = true
			}
		}
		backupPath := state.path + ".xui-webroot.json"
		var backup domainACMEBackup
		backupRaw, _ := os.ReadFile(backupPath)
		_ = json.Unmarshal(backupRaw, &backup)
		original := state.values["Le_Webroot"]
		if needed && (original == "no" || original == "alpn") {
			if err := os.MkdirAll(filepath.Join(root, ".well-known", "acme-challenge"), 0o700); err != nil {
				rollback()
				return nil, rollback, err
			}
			backup = domainACMEBackup{Original: original, Webroot: root}
			raw, _ := json.Marshal(backup)
			if err := os.WriteFile(backupPath, raw, 0o600); err != nil {
				rollback()
				return nil, rollback, err
			}
			if err := writeACMEWebroot(state, root); err != nil {
				_ = os.Remove(backupPath)
				rollback()
				return nil, rollback, err
			}
			undo = append(undo, func() { _ = writeACMEWebroot(state, original); _ = os.Remove(backupPath) })
			state.values["Le_Webroot"] = root
		} else if !needed && backup.Webroot != "" && original == backup.Webroot {
			if err := writeACMEWebroot(state, backup.Original); err != nil {
				rollback()
				return nil, rollback, err
			}
			_ = os.Remove(backupPath)
			undo = append(undo, func() { _ = os.WriteFile(backupPath, backupRaw, 0o600); _ = writeACMEWebroot(state, original) })
		}
		webroot := state.values["Le_Webroot"]
		if needed && filepath.IsAbs(webroot) {
			for _, name := range strings.Split(state.values["Le_Domain"]+","+state.values["Le_Alt"], ",") {
				if name != "" && !strings.Contains(name, "*") {
					roots[strings.ToLower(name)] = webroot
				}
			}
		}
	}
	return roots, rollback, nil
}

func certificateHost(certFile, fallback string) string {
	if fallback != "" {
		return fallback
	}
	raw, err := os.ReadFile(certFile)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err == nil && len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	return ""
}

func seedDomainFront(settings *entity.AllSetting) domainfront.DomainFrontConfig {
	cfg := domainfront.DefaultConfig()
	add := func(name string, panel bool) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || net.ParseIP(name) != nil || strings.ContainsAny(name, ":/*") {
			return
		}
		for i := range cfg.Domains {
			if cfg.Domains[i].Name == name {
				cfg.Domains[i].Panel = cfg.Domains[i].Panel || panel
				if panel {
					cfg.Domains[i].Mode = "direct"
				}
				return
			}
		}
		mode := "sub_link_only"
		if panel {
			mode = "direct"
		}
		cfg.Domains = append(cfg.Domains, domainfront.DomainFrontDomain{Name: name, Mode: mode, TLS: "auto", Enabled: true, Panel: panel, InboundIDs: []int{}})
	}
	add(certificateHost(settings.WebCertFile, settings.WebDomain), true)
	add(certificateHost(settings.SubCertFile, settings.SubDomain), false)
	if u, err := url.Parse(settings.SubPublicURL); err == nil && u.Scheme == "https" && u.Hostname() != "" {
		add(u.Hostname(), false)
		cfg.SubscriptionDomain = strings.ToLower(u.Hostname())
		if port := u.Port(); port != "" {
			_, _ = fmt.Sscan(port, &cfg.PublicPort)
		}
		if settings.SubCDNEnable {
			for i := range cfg.Domains {
				if cfg.Domains[i].Name == cfg.SubscriptionDomain {
					cfg.Domains[i].Mode = "cdn"
				}
			}
		}
	}
	return cfg
}

func (s *SettingService) existingDomainCertificates() []domainfront.DomainFrontCertificate {
	settings, err := s.GetAllSetting()
	if err != nil {
		return nil
	}
	homes := domainACMEHomes()
	states, scheduled := domainACMEStates(homes), acmeScheduled(homes)
	cfg := domainfront.DefaultConfig()
	files := map[string]domainfront.CertificateFiles{}
	for _, candidate := range []domainfront.CertificateFiles{
		{Source: "panel", CertFile: settings.WebCertFile, KeyFile: settings.WebKeyFile, Renewal: "external"},
		{Source: "subscription", CertFile: settings.SubCertFile, KeyFile: settings.SubKeyFile, Renewal: "external"},
	} {
		if candidate.CertFile == "" || candidate.KeyFile == "" {
			continue
		}
		for _, state := range states {
			if candidate.CertFile == state.values["Le_RealFullChainPath"] && candidate.KeyFile == state.values["Le_RealKeyPath"] {
				candidate.Renewal, candidate.AutoRenew = "acme.sh", scheduled
				if !scheduled {
					candidate.Warning = "acme_schedule_missing"
				}
			}
		}
		cfg.Domains = append(cfg.Domains, domainfront.DomainFrontDomain{Name: candidate.Source, Enabled: true, TLS: "auto"})
		files[candidate.Source] = candidate
	}
	return domainfront.NewManager("").StatusWithCertificates(cfg, files).Certificates
}
