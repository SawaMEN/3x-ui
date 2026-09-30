package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

const (
	backupPath = "/etc/x-ui/gateway-xray-backup.json"

	inboundTag  = "in-tproxy"
	outboundTag = "xui-gateway-direct"
	inboundPort = 52345
)

// State describes both the Xray template and the recovery marker. Enabled is
// true when either Gateway-owned config is present or a backup from an older
// interrupted operation still exists, so the UI always offers a safe disable
// path instead of allowing a second backup to overwrite recovery data.
type State struct {
	Enabled      bool
	Configured   bool
	BackupExists bool
}

var operationMu sync.Mutex

// InboundPort returns the local port Linux TPROXY rules must redirect to.
func InboundPort() int {
	return inboundPort
}

func gatewayInbound() map[string]any {
	return map[string]any{
		// Do not bind this TPROXY listener to 127.0.0.1. Redirected packets from
		// LAN clients arrive through PREROUTING and must be accepted by Xray.
		"port":     inboundPort,
		"protocol": "dokodemo-door",
		"settings": map[string]any{
			"followRedirect": true,
			"network":        "tcp,udp",
		},
		"sniffing": map[string]any{
			"destOverride": []any{
				"http",
				"tls",
				"quic",
			},
			"enabled":   true,
			"routeOnly": true,
		},
		"streamSettings": map[string]any{
			"sockopt": map[string]any{
				"tproxy": "tproxy",
			},
		},
		"tag": inboundTag,
	}
}

func gatewayOutbound() map[string]any {
	return map[string]any{
		"protocol": "freedom",
		"settings": map[string]any{
			"finalRules": []any{
				map[string]any{
					"action": "allow",
				},
			},
		},
		"tag": outboundTag,
	}
}

func gatewayRoutingRule() map[string]any {
	return map[string]any{
		"type":        "field",
		"inboundTag":  []any{inboundTag},
		"outboundTag": outboundTag,
	}
}

func loadTemplate() (map[string]any, string, error) {
	if err := database.InitDB(config.GetDBPath()); err != nil {
		return nil, "", fmt.Errorf("initialize database: %w", err)
	}

	settings := &service.SettingService{}
	raw, err := settings.GetXrayConfigTemplate()
	if err != nil {
		return nil, "", fmt.Errorf("get Xray template: %w", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, "", fmt.Errorf("parse Xray template: %w", err)
	}

	return cfg, raw, nil
}

func saveTemplate(cfg map[string]any) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal Xray template: %w", err)
	}

	settings := &service.XraySettingService{}
	if err := settings.SaveXraySetting(string(data)); err != nil {
		return fmt.Errorf("save Xray template: %w", err)
	}
	return nil
}

func backupExists() (bool, error) {
	_, err := os.Stat(backupPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("check gateway backup: %w", err)
}

func createBackup(raw string) error {
	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf(
				"gateway backup already exists at %s; disable Gateway Mode first",
				backupPath,
			)
		}
		return fmt.Errorf("create gateway backup: %w", err)
	}

	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(backupPath)
	}
	if _, err := file.WriteString(raw); err != nil {
		cleanup()
		return fmt.Errorf("write gateway backup: %w", err)
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync gateway backup: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("close gateway backup: %w", err)
	}
	return nil
}

func removeBackup() error {
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove gateway backup: %w", err)
	}
	return nil
}

func arrayField(cfg map[string]any, key string) ([]any, error) {
	value, exists := cfg[key]
	if !exists || value == nil {
		return []any{}, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("Xray %s section must be an array", key)
	}
	return items, nil
}

func itemTag(item any) string {
	obj, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	tag, _ := obj["tag"].(string)
	return tag
}

func hasInboundTag(rule map[string]any, tag string) bool {
	values, ok := rule["inboundTag"].([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == tag {
			return true
		}
	}
	return false
}

func isGatewayRule(item any) bool {
	rule, ok := item.(map[string]any)
	if !ok {
		return false
	}
	outbound, _ := rule["outboundTag"].(string)
	return outbound == outboundTag && hasInboundTag(rule, inboundTag)
}

func hasGatewayArtifacts(cfg map[string]any) bool {
	if inbounds, ok := cfg["inbounds"].([]any); ok {
		for _, item := range inbounds {
			if itemTag(item) == inboundTag {
				return true
			}
		}
	}
	if outbounds, ok := cfg["outbounds"].([]any); ok {
		for _, item := range outbounds {
			if itemTag(item) == outboundTag {
				return true
			}
		}
	}
	if routing, ok := cfg["routing"].(map[string]any); ok {
		if rules, ok := routing["rules"].([]any); ok {
			for _, item := range rules {
				if isGatewayRule(item) {
					return true
				}
			}
		}
	}
	return false
}

func replaceTaggedItem(cfg map[string]any, key, tag string, replacement any) error {
	items, err := arrayField(cfg, key)
	if err != nil {
		return err
	}
	filtered := make([]any, 0, len(items)+1)
	for _, item := range items {
		if itemTag(item) != tag {
			filtered = append(filtered, item)
		}
	}
	cfg[key] = append(filtered, replacement)
	return nil
}

func addGatewayRoutingRule(cfg map[string]any) error {
	var routing map[string]any
	if value, exists := cfg["routing"]; exists && value != nil {
		var ok bool
		routing, ok = value.(map[string]any)
		if !ok {
			return fmt.Errorf("Xray routing section must be an object")
		}
	} else {
		routing = map[string]any{}
	}

	var rules []any
	if value, exists := routing["rules"]; exists && value != nil {
		var ok bool
		rules, ok = value.([]any)
		if !ok {
			return fmt.Errorf("Xray routing rules must be an array")
		}
	}

	filtered := make([]any, 0, len(rules)+1)
	for _, item := range rules {
		if !isGatewayRule(item) {
			filtered = append(filtered, item)
		}
	}
	routing["rules"] = append([]any{gatewayRoutingRule()}, filtered...)
	cfg["routing"] = routing
	return nil
}

func applyGatewayConfig(cfg map[string]any) error {
	if err := replaceTaggedItem(cfg, "inbounds", inboundTag, gatewayInbound()); err != nil {
		return err
	}
	if err := replaceTaggedItem(cfg, "outbounds", outboundTag, gatewayOutbound()); err != nil {
		return err
	}
	return addGatewayRoutingRule(cfg)
}

func removeTaggedItem(cfg map[string]any, key, tag string) (bool, error) {
	value, exists := cfg[key]
	if !exists || value == nil {
		return false, nil
	}
	items, ok := value.([]any)
	if !ok {
		return false, fmt.Errorf("Xray %s section must be an array", key)
	}
	filtered := make([]any, 0, len(items))
	removed := false
	for _, item := range items {
		if itemTag(item) == tag {
			removed = true
			continue
		}
		filtered = append(filtered, item)
	}
	if removed {
		cfg[key] = filtered
	}
	return removed, nil
}

func removeGatewayRoutingRule(cfg map[string]any) (bool, error) {
	value, exists := cfg["routing"]
	if !exists || value == nil {
		return false, nil
	}
	routing, ok := value.(map[string]any)
	if !ok {
		return false, fmt.Errorf("Xray routing section must be an object")
	}
	value, exists = routing["rules"]
	if !exists || value == nil {
		return false, nil
	}
	rules, ok := value.([]any)
	if !ok {
		return false, fmt.Errorf("Xray routing rules must be an array")
	}
	filtered := make([]any, 0, len(rules))
	removed := false
	for _, item := range rules {
		if isGatewayRule(item) {
			removed = true
			continue
		}
		filtered = append(filtered, item)
	}
	if removed {
		routing["rules"] = filtered
		cfg["routing"] = routing
	}
	return removed, nil
}

func removeGatewayConfig(cfg map[string]any) (bool, error) {
	removed := false
	for _, target := range []struct {
		key string
		tag string
	}{
		{key: "inbounds", tag: inboundTag},
		{key: "outbounds", tag: outboundTag},
	} {
		changed, err := removeTaggedItem(cfg, target.key, target.tag)
		if err != nil {
			return false, err
		}
		removed = removed || changed
	}
	changed, err := removeGatewayRoutingRule(cfg)
	if err != nil {
		return false, err
	}
	return removed || changed, nil
}

func getStateUnlocked() (State, error) {
	cfg, _, err := loadTemplate()
	if err != nil {
		return State{}, err
	}
	backup, err := backupExists()
	if err != nil {
		return State{}, err
	}
	configured := hasGatewayArtifacts(cfg)
	return State{
		Enabled:      configured || backup,
		Configured:   configured,
		BackupExists: backup,
	}, nil
}

func GetState() (State, error) {
	operationMu.Lock()
	defer operationMu.Unlock()
	return getStateUnlocked()
}

func IsEnabled() bool {
	state, err := GetState()
	if err == nil {
		return state.Enabled
	}
	// Preserve the old marker semantics if the database is temporarily
	// unavailable. This keeps the CLI/UI from offering a second enable that
	// could overwrite recovery data.
	exists, statErr := backupExists()
	return statErr == nil && exists
}

func Enable() error {
	operationMu.Lock()
	defer operationMu.Unlock()

	cfg, raw, err := loadTemplate()
	if err != nil {
		return err
	}
	backup, err := backupExists()
	if err != nil {
		return err
	}
	if hasGatewayArtifacts(cfg) || backup {
		return fmt.Errorf("Gateway Mode is already enabled")
	}

	if err := applyGatewayConfig(cfg); err != nil {
		return err
	}
	if err := createBackup(raw); err != nil {
		return err
	}
	if err := saveTemplate(cfg); err != nil {
		if cleanupErr := removeBackup(); cleanupErr != nil {
			return fmt.Errorf("%w; cleanup failed: %v", err, cleanupErr)
		}
		return err
	}

	fmt.Println("Xray Gateway configuration enabled.")
	return nil
}

func Disable() error {
	operationMu.Lock()
	defer operationMu.Unlock()

	cfg, _, err := loadTemplate()
	if err != nil {
		return err
	}
	backup, err := backupExists()
	if err != nil {
		return err
	}
	configured := hasGatewayArtifacts(cfg)
	if !configured && !backup {
		return fmt.Errorf("Gateway Mode is not enabled")
	}

	if configured {
		changed, err := removeGatewayConfig(cfg)
		if err != nil {
			return err
		}
		if changed {
			// Do not restore the whole pre-Gateway snapshot here. Operators may
			// legitimately edit Xray settings while Gateway Mode is enabled; only
			// Gateway-owned objects are removed so those edits are preserved.
			if err := saveTemplate(cfg); err != nil {
				return err
			}
		}
	}

	if backup {
		if err := removeBackup(); err != nil {
			return err
		}
	}

	fmt.Println("Xray Gateway configuration disabled.")
	return nil
}
