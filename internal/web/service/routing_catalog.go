package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/routingcatalog"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
	"github.com/SawaMEN/3x-ui/v3/internal/xray/geodata"
)

const routingListMaxBytes = 4 << 20

var routingListWriteMu sync.Mutex

type RoutingCatalogCategory struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	Available bool   `json:"available"`
}

type RoutingCatalogService struct{}

// Builtins use installed geodata, including the panel's existing updater. No
// dependency on a device-specific community API or hard-coded external IPs.
func (*RoutingCatalogService) Categories() []RoutingCatalogCategory {
	var geo GeodataService
	out := []RoutingCatalogCategory{}
	for _, code := range []string{"youtube", "telegram", "discord", "category-ai-!cn", "openai", "google", "github", "netflix", "spotify", "category-games", "category-social-media-!cn", "category-ads-all"} {
		token := "geosite:" + code
		out = append(out, RoutingCatalogCategory{ID: code, Token: token, Available: len(geo.Validate(false, []string{token})) == 0})
	}
	return out
}

type RoutingListInput struct {
	Id             int    `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	UpdateInterval int    `json:"updateInterval"`
}

func validateRoutingListURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(raw) > 4096 {
		return errors.New("list URL must be HTTPS without credentials or fragment")
	}
	return nil
}

func (*RoutingCatalogService) List() ([]model.RoutingList, error) {
	rows := []model.RoutingList{}
	err := database.GetDB().Omit("entries", "last_attempt").Order("id ASC").Find(&rows).Error
	return rows, err
}

func (*RoutingCatalogService) Entries(id int) (routingcatalog.Entries, error) {
	var row model.RoutingList
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return routingcatalog.Entries{}, err
	}
	var entries routingcatalog.Entries
	err := json.Unmarshal([]byte(row.Entries), &entries)
	return entries, err
}

func downloadRoutingList(ctx context.Context, rawURL string) (routingcatalog.Entries, error) {
	if err := validateRoutingListURL(rawURL); err != nil {
		return routingcatalog.Entries{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// Catalog sources never inherit another operation's private-network opt-in.
	ctx = netsafe.ContextWithAllowPrivate(ctx, false)
	transport := &http.Transport{DialContext: netsafe.SSRFGuardedDialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many list redirects")
		}
		return validateRoutingListURL(req.URL.String())
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return routingcatalog.Entries{}, err
	}
	req.Header.Set("User-Agent", "3x-ui-routing-catalog/1.0")
	// Administrators intentionally choose source hosts. The guarded dialer
	// resolves, validates and dials the same public IP on every connection;
	// redirects remain HTTPS and the transport has no proxy bypass.
	// codeql[go/request-forgery]

	resp, err := client.Do(req)
	if err != nil {
		// The URL can contain a private subscription token. Never persist it
		// inside an error that later appears in the list view or logs.
		if errors.Is(err, netsafe.ErrPrivateAddressBlocked) {
			return routingcatalog.Entries{}, netsafe.ErrPrivateAddressBlocked
		}
		return routingcatalog.Entries{}, errors.New("routing list download failed (network, TLS or redirect error)")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return routingcatalog.Entries{}, fmt.Errorf("routing list returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, routingListMaxBytes+1))
	if err != nil {
		return routingcatalog.Entries{}, errors.New("cannot read routing list response")
	}
	if len(body) > routingListMaxBytes {
		return routingcatalog.Entries{}, errors.New("routing list exceeds 4 MiB")
	}
	return routingcatalog.Parse(bytes.NewReader(body))
}

func (*RoutingCatalogService) Save(ctx context.Context, in RoutingListInput) (*model.RoutingList, error) {
	in.Name, in.URL = strings.TrimSpace(in.Name), strings.TrimSpace(in.URL)
	if in.Id < 0 || utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 120 {
		return nil, errors.New("list name must be 1–120 characters")
	}
	if in.UpdateInterval != 0 && (in.UpdateInterval < 3600 || in.UpdateInterval > 604800) {
		return nil, errors.New("update interval must be 0 or 3600–604800 seconds")
	}
	if err := validateRoutingListURL(in.URL); err != nil {
		return nil, err
	}
	routingListWriteMu.Lock()
	defer routingListWriteMu.Unlock()
	db := database.GetDB()
	var row model.RoutingList
	if in.Id > 0 {
		if err := db.First(&row, in.Id).Error; err != nil {
			return nil, err
		}
	} else {
		var count int64
		if err := db.Model(&model.RoutingList{}).Count(&count).Error; err != nil {
			return nil, err
		}
		if count >= 100 {
			return nil, errors.New("routing catalog may contain at most 100 custom sources")
		}
	}
	entries, err := downloadRoutingList(ctx, in.URL)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	row.Name, row.URL, row.UpdateInterval = in.Name, in.URL, in.UpdateInterval
	row.Entries = string(encoded)
	row.DomainCount, row.IPCount = len(entries.Domains), len(entries.IPs)
	row.LastUpdated, row.LastAttempt, row.LastError = time.Now().Unix(), time.Now().Unix(), ""
	if err := db.Save(&row).Error; err != nil {
		return nil, err
	}
	row.Entries = ""
	return &row, nil
}

// Refresh only replaces a cached policy after the entire response validates.
func (*RoutingCatalogService) Refresh(ctx context.Context, id int) (bool, error) {
	routingListWriteMu.Lock()
	defer routingListWriteMu.Unlock()
	db := database.GetDB()
	var row model.RoutingList
	if err := db.First(&row, id).Error; err != nil {
		return false, err
	}
	now := time.Now().Unix()
	entries, err := downloadRoutingList(ctx, row.URL)
	if err != nil {
		if updateErr := db.Model(&row).Updates(map[string]any{"last_attempt": now, "last_error": err.Error()}).Error; updateErr != nil {
			return false, updateErr
		}
		return false, err
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return false, err
	}
	changed := row.Entries != string(encoded)
	err = db.Model(&row).Updates(map[string]any{"entries": string(encoded), "domain_count": len(entries.Domains), "ip_count": len(entries.IPs), "last_updated": now, "last_attempt": now, "last_error": ""}).Error
	return changed, err
}

func (s *RoutingCatalogService) RefreshDue(ctx context.Context) (bool, error) {
	var rows []model.RoutingList
	now := time.Now().Unix()
	if err := database.GetDB().Select("id").Where("update_interval > 0 AND last_attempt + update_interval <= ?", now).Order("id ASC").Find(&rows).Error; err != nil {
		return false, err
	}
	changed := false
	var failures []error
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		updated, err := s.Refresh(ctx, row.Id)
		changed = changed || updated
		if err != nil {
			failures = append(failures, fmt.Errorf("list %d: %w", row.Id, err))
		}
	}
	return changed, errors.Join(failures...)
}

func (*RoutingCatalogService) Delete(id int) error {
	routingListWriteMu.Lock()
	defer routingListWriteMu.Unlock()
	template, err := (&SettingService{}).GetXrayConfigTemplate()
	if err != nil {
		return err
	}
	ids, err := routingListIDs(template)
	if err != nil {
		return err
	}
	for _, used := range ids {
		if used == id {
			return errors.New("remove this list's routing rules and save the configuration before deleting its source")
		}
	}
	native, err := (&SettingService{}).GetSingBoxConfigTemplate()
	if err != nil {
		return err
	}
	nativeIDs, err := routingListIDs(native)
	if err != nil {
		return err
	}
	for _, used := range nativeIDs {
		if used == id {
			return errors.New("remove this list's native routing rules and save the configuration before deleting its source")
		}
	}
	res := database.GetDB().Delete(&model.RoutingList{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("routing list not found")
	}
	return nil
}

func routingListID(rule map[string]any) (int, error) {
	value, ok := rule["routingListId"]
	if !ok {
		return 0, nil
	}
	n, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil || n <= 0 {
		return 0, errors.New("routingListId must be a positive integer")
	}
	return n, nil
}

func routingListIDs(template string) ([]int, error) {
	if strings.TrimSpace(template) == "" {
		return nil, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(template), &cfg); err != nil {
		return nil, err
	}
	ids, seen := []int{}, map[int]bool{}
	rules := append(routingRulesFromCfg(cfg), nativeCatalogRules(cfg)...)
	for _, rule := range rules {
		id, err := routingListID(rule)
		if err != nil {
			return nil, err
		}
		if id > 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, nil
}

func loadRoutingLists(template string) ([]model.RoutingList, error) {
	ids, err := routingListIDs(template)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	var rows []model.RoutingList
	err = database.GetDB().Where("id IN ?", ids).Find(&rows).Error
	return rows, err
}

// materializeRoutingLists resolves cached sources at generation time, never on
// the network. Domain and IP entries produce separate rules (OR, not AND).
// A missing/invalid list fails closed: it must never become a catch-all rule.
func materializeRoutingLists(template string, sources []model.RoutingList) (string, error) {
	ids, err := routingListIDs(template)
	if err != nil || len(ids) == 0 {
		return template, err
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(template), &cfg); err != nil {
		return "", err
	}
	byID := map[int]model.RoutingList{}
	for _, row := range sources {
		byID[row.Id] = row
	}
	var out []map[string]any
	for _, rule := range routingRulesFromCfg(cfg) {
		id, err := routingListID(rule)
		if err != nil {
			return "", err
		}
		if id == 0 {
			out = append(out, rule)
			continue
		}
		if rule["enabled"] == false {
			continue
		}
		if hasRoutingMatcherValue(rule["domain"]) || hasRoutingMatcherValue(rule["ip"]) {
			return "", fmt.Errorf("routing list %d manages its own domain/IP matchers; use another rule for manual matchers", id)
		}
		row, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("routing list %d is missing", id)
		}
		var entries routingcatalog.Entries
		if err := json.Unmarshal([]byte(row.Entries), &entries); err != nil || len(entries.Domains)+len(entries.IPs) == 0 {
			return "", fmt.Errorf("routing list %d has no valid cache", id)
		}
		if len(entries.Domains)+len(entries.IPs) > routingcatalog.MaxEntries {
			return "", fmt.Errorf("routing list %d exceeds entry limit", id)
		}
		for _, values := range [][]string{entries.Domains, entries.IPs} {
			for _, value := range values {
				if strings.TrimSpace(value) == "" {
					return "", fmt.Errorf("routing list %d has an empty matcher", id)
				}
			}
		}
		for _, group := range []struct {
			key    string
			values []string
		}{{"domain", entries.Domains}, {"ip", entries.IPs}} {
			key, values := group.key, group.values
			if len(values) == 0 {
				continue
			}
			copy := make(map[string]any, len(rule))
			for k, v := range rule {
				copy[k] = v
			}
			delete(copy, "routingListId")
			delete(copy, "domain")
			delete(copy, "ip")
			copy[key] = values
			if tag, ok := copy["ruleTag"].(string); ok {
				copy["ruleTag"] = tag + "-" + key
			}
			out = append(out, copy)
		}
	}
	setRoutingRulesInCfg(cfg, out)
	raw, err := json.Marshal(cfg)
	return string(raw), err
}

// The normal reconciler validates and publishes the generated candidate. A
// manually stopped core stays stopped; sources never start another runtime.
func MarkRoutingCatalogChanged() {
	settings := &SettingService{}
	template, err := settings.GetXrayConfigTemplate()
	if err != nil {
		return
	}
	ids, err := routingListIDs(template)
	if err != nil {
		return
	}
	native, err := settings.GetSingBoxConfigTemplate()
	if err != nil {
		return
	}
	nativeIDs, err := routingListIDs(native)
	if err != nil || len(ids)+len(nativeIDs) == 0 {
		return
	}
	core, _ := settings.GetCoreType()
	if IsNativeCore(core) {
		SelectedNativeCore().SetToNeedRestart()
	} else {
		(&CoreService{}).SetToNeedRestart()
	}
}

func nativeCatalogRules(cfg map[string]any) []map[string]any {
	route, _ := cfg["route"].(map[string]any)
	raw, _ := route["rules"].([]any)
	var rules []map[string]any
	for _, item := range raw {
		if rule, ok := item.(map[string]any); ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func nativeCatalogHasReferences(template string) bool {
	var cfg map[string]any
	if json.Unmarshal([]byte(template), &cfg) != nil {
		return false
	}
	for _, rule := range nativeCatalogRules(cfg) {
		if _, ok := rule["routingListId"]; ok {
			return true
		}
		if _, ok := rule["routingListCategory"]; ok {
			return true
		}
	}
	return false
}

func materializeNativeRoutingLists(template string, sources []model.RoutingList) (string, error) {
	if !nativeCatalogHasReferences(template) {
		return template, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(template), &cfg); err != nil {
		return "", err
	}
	var out []any
	store := geodata.NewStore(assetDir())
	for _, rule := range nativeCatalogRules(cfg) {
		id, err := routingListID(rule)
		if err != nil {
			return "", err
		}
		categoryValue, categoryPresent := rule["routingListCategory"]
		if id == 0 && !categoryPresent {
			out = append(out, rule)
			continue
		}
		if rule["enabled"] == false {
			continue
		}
		if id > 0 && categoryPresent {
			return "", errors.New("catalog rule must reference either a category or a custom list")
		}
		for _, key := range []string{"domain", "domain_suffix", "domain_keyword", "domain_regex", "ip_cidr", "geosite", "geoip", "rule_set"} {
			if hasRoutingMatcherValue(rule[key]) {
				return "", fmt.Errorf("catalog manages %s; use another rule for manual matchers", key)
			}
		}
		legacyRule := map[string]any{"type": "field", "outboundTag": "catalog-target"}
		if id > 0 {
			legacyRule["routingListId"] = id
		} else {
			category, ok := categoryValue.(string)
			if !ok || category == "" {
				return "", errors.New("invalid routing list category")
			}
			legacyRule["domain"] = []string{"geosite:" + category}
		}
		legacy := map[string]any{"routing": map[string]any{"rules": []any{legacyRule}}}
		raw, err := json.Marshal(legacy)
		if err != nil {
			return "", err
		}
		resolved, err := materializeRoutingLists(string(raw), sources)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal([]byte(resolved), &legacy); err != nil {
			return "", err
		}
		translated, err := singbox.TranslateXrayRoutingWithGeoData(legacy["routing"].(map[string]any), store)
		if err != nil {
			return "", err
		}
		matches, _ := translated["rules"].([]map[string]any)
		if len(matches) == 0 {
			return "", errors.New("catalog category has no routing entries")
		}
		for _, match := range matches {
			copy := make(map[string]any, len(rule))
			for key, value := range rule {
				copy[key] = value
			}
			for _, key := range []string{"routingListId", "routingListCategory", "comment", "enabled"} {
				delete(copy, key)
			}
			matched := false
			for _, key := range []string{"domain", "domain_suffix", "domain_keyword", "domain_regex", "ip_cidr"} {
				if hasRoutingMatcherValue(match[key]) {
					copy[key] = match[key]
					matched = true
				}
			}
			if !matched {
				return "", errors.New("catalog list resolved to an empty matcher")
			}
			out = append(out, copy)
		}
	}
	cfg["route"].(map[string]any)["rules"] = out
	raw, err := json.Marshal(cfg)
	return string(raw), err
}
