package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

const unavailableSubscriptionTag = "xui-unavailable-subscription"

var managedSubscriptionAlias = regexp.MustCompile(`^sub-auto-(?:block-|route-)?[1-9][0-9]*$`)

// Definitions owned by subscriptions must never be frozen in a manual template.
// The current subscription is the only authority, including disabled profiles.
func subscriptionOwnership(subs []*model.OutboundSubscription) map[string]bool {
	owned := map[string]bool{}
	for _, sub := range subs {
		owned[autoOutboundTag(sub.Id)] = true
		owned[autoBlockedTag(sub.Id)] = true
		var rows []map[string]any
		_ = json.Unmarshal([]byte(sub.LastFetchedOutbounds), &rows)
		for _, row := range rows {
			if tag, ok := row["tag"].(string); ok && tag != "" {
				owned[tag] = true
			}
		}
	}
	return owned
}

// Return diagnostics instead of silently changing a broken proxy route to direct.
func cleanSubscriptionSource(cfg map[string]any, owned, active map[string]bool) (bool, []string) {
	removed := map[string]bool{}
	cleaned := map[string]bool{}
	var visit func(any)
	visit = func(value any) {
		switch n := value.(type) {
		case string:
			if managedSubscriptionAlias.MatchString(n) && !active[n] {
				removed[n] = true
			}
		case []any:
			for _, v := range n {
				visit(v)
			}
		case map[string]any:
			for key, v := range n {
				if key != "headers" {
					visit(v)
				}
			}
		}
	}
	visit(cfg)
	for tag := range owned {
		if !active[tag] {
			removed[tag] = true
		}
	}
	lostDefault := false
	if rows, ok := cfg["outbounds"].([]any); ok && len(rows) > 0 {
		first, _ := rows[0].(map[string]any)
		tag, _ := first["tag"].(string)
		if (owned[tag] || managedSubscriptionAlias.MatchString(tag)) && !active[tag] {
			route, _ := cfg["route"].(map[string]any)
			if first["type"] != nil || route != nil {
				if route["final"] == nil || route["final"] == "" {
					if route == nil {
						route = map[string]any{}
						cfg["route"] = route
					}
					route["final"] = unavailableSubscriptionTag
					lostDefault = true
				}
			} else {
				lostDefault = true
			}
		}
	}
	for _, key := range []string{"outbounds", "endpoints"} {
		rows, ok := cfg[key].([]any)
		if !ok {
			continue
		}
		kept := make([]any, 0, len(rows))
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			tag, _ := row["tag"].(string)
			if owned[tag] || managedSubscriptionAlias.MatchString(tag) || tag == unavailableSubscriptionTag {
				if tag != unavailableSubscriptionTag {
					cleaned[tag] = true
				}
				continue
			}
			kept = append(kept, raw)
		}
		cfg[key] = kept
	}
	// Old full runtime exports can also contain the generated Xray loopback rule.
	// It must be regenerated from current subscription state, never accumulated.
	for _, section := range []string{"route", "routing"} {
		route, _ := cfg[section].(map[string]any)
		if rules, ok := route["rules"].([]any); ok {
			kept := make([]any, 0, len(rules))
			for _, raw := range rules {
				rule, _ := raw.(map[string]any)
				generated := false
				for _, key := range []string{"inbound", "inboundTag"} {
					if tags, ok := rule[key].([]any); ok && len(tags) == 1 {
						tag, _ := tags[0].(string)
						generated = generated || strings.HasPrefix(tag, "sub-auto-route-") && managedSubscriptionAlias.MatchString(tag)
					}
				}
				if generated {
					cleaned["runtime routing"] = true
					continue
				}
				kept = append(kept, raw)
			}
			route["rules"] = kept
		}
	}
	if lostDefault && cfg["route"] == nil {
		rows, _ := cfg["outbounds"].([]any)
		cfg["outbounds"] = append([]any{map[string]any{"tag": unavailableSubscriptionTag, "protocol": "blackhole", "settings": map[string]any{}}}, rows...)
	}
	repaired := replaceConnectionTags(cfg, removed, unavailableSubscriptionTag, "") || lostDefault
	var warnings []string
	if len(cleaned) > 0 {
		warnings = append(warnings, "Сохранённые копии подключений подписок удалены; используются актуальные данные подписок.")
	}
	if repaired {
		tags := make([]string, 0, len(removed))
		for tag := range removed {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		if len(tags) > 8 {
			tags = append(tags[:8], fmt.Sprintf("ещё %d", len(tags)-8))
		}
		warnings = append(warnings, fmt.Sprintf("Недоступные подключения %v заменены блокирующим выходом. Выберите действующий выход в маршрутизации и DNS.", tags))
	}
	needed := replaceConnectionTags(cfg, map[string]bool{unavailableSubscriptionTag: true}, unavailableSubscriptionTag, "")
	return repaired || needed, warnings
}

func addUnavailableSubscriptionOutbound(cfg map[string]any, native bool) {
	rows, _ := cfg["outbounds"].([]any)
	for _, raw := range rows {
		if row, ok := raw.(map[string]any); ok && row["tag"] == unavailableSubscriptionTag {
			return
		}
	}
	row := map[string]any{"tag": unavailableSubscriptionTag, "protocol": "blackhole", "settings": map[string]any{}}
	if native {
		row = map[string]any{"tag": unavailableSubscriptionTag, "type": "block"}
	}
	cfg["outbounds"] = append(rows, row)
}

// NormalizeSubscriptionTemplate is shared by source editors and the Xray runtime.
// It only reads subscription metadata; it does not probe networks or start cores.
func NormalizeSubscriptionTemplate(raw string, native bool) (string, error) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return "", err
	}
	if cfg == nil {
		return "", fmt.Errorf("configuration must be a JSON object")
	}
	var subs []*model.OutboundSubscription
	if err := database.GetDB().Order("priority ASC,id ASC").Find(&subs).Error; err != nil {
		return "", err
	}
	active := map[string]bool{}
	for _, sub := range subs {
		if sub.Enabled {
			if sub.AutoBalance {
				active[autoOutboundTag(sub.Id)] = true
				active[autoBlockedTag(sub.Id)] = true
			}
			var rows []map[string]any
			_ = json.Unmarshal([]byte(sub.LastFetchedOutbounds), &rows)
			for _, row := range rows {
				if tag, ok := row["tag"].(string); ok {
					active[tag] = true
				}
			}
		}
	}
	needed, _ := cleanSubscriptionSource(cfg, subscriptionOwnership(subs), active)
	infrastructureBlocked, err := infrastructureUsesBlockedSubscription()
	if err != nil {
		return "", err
	}
	if (needed || infrastructureBlocked) && !native {
		addUnavailableSubscriptionOutbound(cfg, false)
	}
	data, err := json.Marshal(cfg)
	return string(data), err
}
