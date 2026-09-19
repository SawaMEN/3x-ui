package service

import (
  "context"
  "crypto/rand"
  "encoding/hex"
  "errors"
  "fmt"
  "os"
  "os/exec"
  "path/filepath"
  "strconv"
  "strings"
  "time"
)

const (
  telemtConfigPath = "/etc/x-ui/telemt.toml"
  telemtServiceName = "telemt.service"
  telemtBinaryPath = "/usr/local/x-ui/bin/telemt"
)

type TelemtConfig struct {
  Enabled bool `json:"enabled"`
  Port int `json:"port"`
  Secret string `json:"secret"`
  IPv4 bool `json:"ipv4"`
  IPv6 bool `json:"ipv6"`
  FastMode bool `json:"fastMode"`
  Classic bool `json:"classic"`
  Secure bool `json:"secure"`
  TLS bool `json:"tls"`
  UpstreamType string `json:"upstreamType"`
}

type TelemtStatus struct {
  Installed bool `json:"installed"`
  Active bool `json:"active"`
  Enabled bool `json:"enabled"`
  Configured bool `json:"configured"`
  Version string `json:"version"`
}

type TelemtProxy struct {
  Name string `json:"name"`
  Secret string `json:"secret"`
  Host string `json:"host"`
  Port int `json:"port"`
  TLS bool `json:"tls"`
  Link string `json:"link"`
}

type TelemtCreateRequest struct {
  Name string `json:"name"`
  Host string `json:"host"`
}

type TelemtService struct{}

func defaultTelemtConfig() TelemtConfig {
  return TelemtConfig{Port: 8443, IPv4: true, IPv6: true, FastMode: true, TLS: true, UpstreamType: "direct"}
}

func renderTelemtConfig(c TelemtConfig) (string, error) {
  if c.Port < 1 || c.Port > 65535 { return "", errors.New("telemt: invalid port") }
  if !c.IPv4 && !c.IPv6 { return "", errors.New("telemt: enable IPv4 or IPv6") }
  if len(c.Secret) != 32 { return "", errors.New("telemt: secret must contain exactly 32 hexadecimal characters") }
  if _, err := hex.DecodeString(c.Secret); err != nil { return "", errors.New("telemt: secret must be hexadecimal") }
  if c.UpstreamType != "direct" { return "", errors.New("telemt: only direct upstream is supported by the panel") }

  listeners := ""
  if c.IPv4 { listeners += "[[server.listeners]]\nip = \"0.0.0.0\"\n\n" }
  if c.IPv6 { listeners += "[[server.listeners]]\nip = \"::\"\n\n" }
  return fmt.Sprintf("[general]\nfast_mode = %t\nuse_middle_proxy = false\nlog_level = \"normal\"\n\n[general.modes]\nclassic = %t\nsecure = %t\ntls = %t\n\n[general.links]\nshow = [\"xui\"]\n\n[network]\nipv4 = %t\nipv6 = %t\n\n[server]\nport = %d\n\n%s[access]\nreplay_check_len = 65536\nignore_time_skew = false\n\n[access.users]\nxui = \"%s\"\n\n[[upstreams]]\ntype = \"direct\"\nweight = 1\nenabled = true\n", c.FastMode, c.Classic, c.Secure, c.TLS, c.IPv4, c.IPv6, c.Port, listeners, strings.ToLower(c.Secret)), nil
}

func ensureTelemtConfig() error {
  if err := os.MkdirAll(filepath.Dir(telemtConfigPath), 0700); err != nil {
    return fmt.Errorf("telemt: create config directory: %w", err)
  }
  if _, err := os.Stat(telemtConfigPath); err == nil { return nil } else if !errors.Is(err, os.ErrNotExist) { return err }

  secretBytes := make([]byte, 16)
  if _, err := rand.Read(secretBytes); err != nil { return fmt.Errorf("telemt: generate secret: %w", err) }
  c := defaultTelemtConfig()
  c.Secret = hex.EncodeToString(secretBytes)
  data, err := renderTelemtConfig(c)
  if err != nil { return err }
  if err := os.WriteFile(telemtConfigPath, []byte(data), 0600); err != nil {
    return fmt.Errorf("telemt: create config: %w", err)
  }
  return nil
}

func (TelemtService) Status() TelemtStatus {
  _, binErr := os.Stat(telemtBinaryPath)
  _, cfgErr := os.Stat(telemtConfigPath)
  version := ""
  if binErr == nil { version = telemtVersion() }
  return TelemtStatus{Installed: binErr == nil, Active: systemctl("is-active", "--quiet", telemtServiceName) == nil, Enabled: systemctl("is-enabled", "--quiet", telemtServiceName) == nil, Configured: cfgErr == nil, Version: version}
}

func telemtVersion() string {
  for _, arg := range []string{"--version", "-V"} {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    out, err := exec.CommandContext(ctx, telemtBinaryPath, arg).CombinedOutput()
    cancel()
    if err == nil && strings.TrimSpace(string(out)) != "" { return strings.TrimSpace(string(out)) }
  }
  return ""
}

func (TelemtService) GetConfig() (TelemtConfig, error) {
  if err := ensureTelemtConfig(); err != nil { return TelemtConfig{}, err }
  b, err := os.ReadFile(telemtConfigPath)
  if err != nil { return TelemtConfig{}, err }
  c := defaultTelemtConfig()
  for _, line := range strings.Split(string(b), "\n") {
    line = strings.TrimSpace(line)
    if strings.HasPrefix(line, "port = ") { c.Port, _ = strconv.Atoi(strings.Trim(strings.TrimPrefix(line, "port = "), `"`)) }
    if strings.HasPrefix(line, "xui = ") { c.Secret = strings.Trim(strings.TrimPrefix(line, "xui = "), `"`) }
    if strings.HasPrefix(line, "ipv4 = ") { c.IPv4 = strings.TrimSpace(strings.TrimPrefix(line, "ipv4 = ")) == "true" }
    if strings.HasPrefix(line, "ipv6 = ") { c.IPv6 = strings.TrimSpace(strings.TrimPrefix(line, "ipv6 = ")) == "true" }
    if strings.HasPrefix(line, "fast_mode = ") { c.FastMode = strings.TrimSpace(strings.TrimPrefix(line, "fast_mode = ")) == "true" }
    if strings.HasPrefix(line, "classic = ") { c.Classic = strings.TrimSpace(strings.TrimPrefix(line, "classic = ")) == "true" }
    if strings.HasPrefix(line, "secure = ") { c.Secure = strings.TrimSpace(strings.TrimPrefix(line, "secure = ")) == "true" }
    if strings.HasPrefix(line, "tls = ") { c.TLS = strings.TrimSpace(strings.TrimPrefix(line, "tls = ")) == "true" }
    if strings.HasPrefix(line, "type = ") { c.UpstreamType = strings.Trim(strings.TrimPrefix(line, "type = "), `"`) }
  }
  c.Enabled = TelemtService{}.Status().Enabled
  return c, nil
}

func (TelemtService) SaveConfig(c TelemtConfig) error {
  data, err := renderTelemtConfig(c)
  if err != nil { return err }
  if err := ensureTelemtConfig(); err != nil { return err }
  old, oldErr := os.ReadFile(telemtConfigPath)
  wasActive := systemctl("is-active", "--quiet", telemtServiceName) == nil
  if err := os.WriteFile(telemtConfigPath, []byte(data), 0600); err != nil { return err }
  if err := systemctl("daemon-reload"); err != nil { return err }
  if wasActive {
    if err := systemctl("restart", telemtServiceName); err != nil {
      if oldErr == nil { _ = os.WriteFile(telemtConfigPath, old, 0600); _ = systemctl("daemon-reload"); _ = systemctl("restart", telemtServiceName) }
      return fmt.Errorf("telemt: new configuration was rejected: %w", err)
    }
  }
  return nil
}

func (TelemtService) CreateProxy(req TelemtCreateRequest) (TelemtProxy, error) {
  name := strings.TrimSpace(req.Name)
  host := strings.TrimSpace(req.Host)
  if name == "" { return TelemtProxy{}, errors.New("telemt: proxy name is required") }
  if host == "" { return TelemtProxy{}, errors.New("telemt: public host is required") }
  if strings.ContainsAny(name, "=\n\r\"") || strings.ContainsAny(host, " \t\n\r\"") { return TelemtProxy{}, errors.New("telemt: invalid proxy name or host") }

  if err := ensureTelemtConfig(); err != nil { return TelemtProxy{}, err }
  b, err := os.ReadFile(telemtConfigPath)
  if err != nil { return TelemtProxy{}, err }
  secretBytes := make([]byte, 16)
  if _, err := rand.Read(secretBytes); err != nil { return TelemtProxy{}, err }
  secret := hex.EncodeToString(secretBytes)

  username := strings.ToLower(strings.Map(func(r rune) rune {
    if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' { return r }
    if r >= 'A' && r <= 'Z' { return r + ('a' - 'A') }
    return '_'
  }, name))
  username = strings.Trim(username, "_-")
  if username == "" { username = "proxy" }

  text := string(b)
  section := "[access.users]"
  idx := strings.Index(text, section)
  if idx < 0 { return TelemtProxy{}, errors.New("telemt: access.users section is missing") }
  insertAt := len(text)
  if next := strings.Index(text[idx+len(section):], "\n["); next >= 0 { insertAt = idx + len(section) + next + 1 }
  for i := 2; strings.Contains(text[idx:insertAt], username+" = "); i++ { username = fmt.Sprintf("%s-%d", username, i) }
  entry := fmt.Sprintf("%s = \"%s\"\n", username, secret)
  old := append([]byte(nil), b...)
  text = text[:insertAt] + entry + text[insertAt:]
  if err := os.WriteFile(telemtConfigPath, []byte(text), 0600); err != nil { return TelemtProxy{}, err }
  if err := systemctl("daemon-reload"); err != nil { return TelemtProxy{}, err }
  if systemctl("is-active", "--quiet", telemtServiceName) == nil {
    if err := systemctl("restart", telemtServiceName); err != nil {
      _ = os.WriteFile(telemtConfigPath, old, 0600); _ = systemctl("daemon-reload"); _ = systemctl("restart", telemtServiceName)
      return TelemtProxy{}, fmt.Errorf("telemt: proxy configuration was rejected: %w", err)
    }
  }
  cfg, err := TelemtService{}.GetConfig()
  if err != nil { return TelemtProxy{}, err }
  return TelemtProxy{Name: name, Secret: secret, Host: host, Port: cfg.Port, TLS: cfg.TLS, Link: fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", host, cfg.Port, secret)}, nil
}

func (TelemtService) Apply(action string) error {
  switch action { case "start", "stop", "restart", "enable", "disable": return systemctl(action, telemtServiceName); default: return errors.New("telemt: unsupported action") }
}

func systemctl(args ...string) error {
  ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
  defer cancel()
  return exec.CommandContext(ctx, "systemctl", args...).Run()
}
