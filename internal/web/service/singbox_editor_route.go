package service

import (
	"encoding/json"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Old editor snapshots persisted injected sniff rules. Remove exact copies
// only: modified user rules and intentional terminal catch-alls stay intact.
func removeGeneratedSniffRules(cfg *singbox.Config, generated []any) {
	if cfg.Route == nil || len(generated) == 0 {
		return
	}
	keys := make(map[string]bool, len(generated))
	for _, rule := range generated {
		data, _ := json.Marshal(rule)
		keys[string(data)] = true
	}
	var rules []any
	for _, rule := range singBoxRouteRules(cfg.Route) {
		data, _ := json.Marshal(rule)
		if !keys[string(data)] {
			rules = append(rules, rule)
		}
	}
	cfg.Route["rules"] = rules
}
