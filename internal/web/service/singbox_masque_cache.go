package service

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// WARP registration must survive core restarts even when saved through JSON/API.
func ensureWARPMASQUECache(cfg *singbox.Config) error {
	hasWARP := false
	for _, outbound := range cfg.Outbounds {
		if outbound["type"] == "masque" {
			hasWARP = true
			break
		}
	}
	if !hasWARP {
		return nil
	}
	experimental := maps.Clone(cfg.Experimental)
	if experimental == nil {
		experimental = map[string]any{}
	}
	var cache map[string]any
	if raw := experimental["cache_file"]; raw != nil {
		var ok bool
		cache, ok = raw.(map[string]any)
		if !ok {
			return fmt.Errorf("WARP MASQUE cache_file must be a JSON object")
		}
		cache = maps.Clone(cache)
	}
	if cache == nil {
		cache = map[string]any{}
	}
	cache["enabled"], cache["store_masque_config"] = true, true
	// The core's default cache.db path depends on its working directory.
	// Persist WARP device registration beside the managed core configuration
	// without overriding a user-provided path.
	if path, ok := cache["path"]; !ok {
		cache["path"] = filepath.Join(filepath.Dir(singbox.GetConfigPath()), "warp-masque-cache.db")
	} else if value, ok := path.(string); ok && strings.TrimSpace(value) == "" {
		cache["path"] = filepath.Join(filepath.Dir(singbox.GetConfigPath()), "warp-masque-cache.db")
	}
	experimental["cache_file"] = cache
	cfg.Experimental = experimental
	return nil
}
