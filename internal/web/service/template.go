package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

const (
	localTemplateMaxBytes = 256 << 10
	localTemplateMaxCount = 2000
)

var localTemplateTagRE = regexp.MustCompile("^[a-z0-9][a-z0-9 _-]{0,23}$")

type TemplateInput struct {
	Kind        string          `json:"kind"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Tags        []string        `json:"tags"`
	Content     json.RawMessage `json:"content"`
}

type TemplateView struct {
	Id          int            `json:"id"`
	Kind        string         `json:"kind"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Tags        []string       `json:"tags"`
	SizeBytes   int            `json:"sizeBytes"`
	CreatedAt   int64          `json:"createdAt"`
	UpdatedAt   int64          `json:"updatedAt"`
	Summary     map[string]any `json:"summary,omitempty"`
}

type TemplateDetail struct {
	TemplateView
	Content json.RawMessage `json:"content"`
}

type TemplateSanitizeResult struct {
	Kind      string          `json:"kind"`
	Content   json.RawMessage `json:"content"`
	Warnings  []string        `json:"warnings"`
	SizeBytes int             `json:"sizeBytes"`
}

var templateSecretKeys = map[string]struct{}{
	"password": {}, "secret": {}, "secretkey": {}, "privatekey": {}, "publickey": {}, "presharedkey": {},
	"psk": {}, "token": {}, "apikey": {}, "authtoken": {}, "authorization": {},
	"uuid": {}, "authstr": {}, "authstring": {}, "mldsa65seed": {}, "shortid": {},
}

var templateKeyNormalizer = strings.NewReplacer("_", "", "-", "")

var templateHostKeys = map[string]struct{}{
	"publichost": {}, "apilisten": {}, "serverhost": {}, "publicaddress": {}, "externalproxy": {},
}

var templateCollectionKeys = map[string]struct{}{
	"clients": {}, "peers": {}, "users": {}, "accounts": {},
}

func normalizeTemplateTags(tags []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, min(len(tags), 8))
	for _, raw := range tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" || seen[tag] {
			continue
		}
		if !localTemplateTagRE.MatchString(tag) {
			return nil, fmt.Errorf("invalid tag %q", raw)
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) > 8 {
			return nil, errors.New("at most 8 tags are allowed")
		}
	}
	return out, nil
}

func parseTemplateObject(raw []byte) (map[string]any, error) {
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("content must be a JSON object: %w", err)
	}
	if obj == nil {
		return nil, errors.New("content must be a JSON object")
	}
	return obj, nil
}

func isMeaningfulTemplateValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

func sanitizeTemplateValue(v any, path, protocol string, warnings *[]string) any {
	switch x := v.(type) {
	case map[string]any:
		if value, ok := x["protocol"].(string); ok {
			protocol = strings.ToLower(value)
		} else if value, ok := x["type"].(string); ok {
			protocol = strings.ToLower(value)
		}
		out := make(map[string]any, len(x))
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := x[k]
			lk := strings.ToLower(strings.TrimSpace(k))
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			_, secret := templateSecretKeys[templateKeyNormalizer.Replace(lk)]
			secret = secret || (lk == "id" && (protocol == "vless" || protocol == "vmess" || protocol == "tuic"))
			secret = secret || (lk == "auth" && (protocol == "hysteria" || protocol == "hysteria2"))
			secret = secret || (lk == "decryption" && protocol == "vless" && child != "none")
			if secret {
				if isMeaningfulTemplateValue(child) {
					*warnings = append(*warnings, childPath+": secret removed")
				}
				continue
			}
			if _, host := templateHostKeys[lk]; host {
				if isMeaningfulTemplateValue(child) {
					*warnings = append(*warnings, childPath+": host-specific value removed")
				}
				continue
			}
			if _, collection := templateCollectionKeys[lk]; collection {
				if arr, ok := child.([]any); ok && len(arr) > 0 {
					*warnings = append(*warnings, fmt.Sprintf("%s: %d entry(s) removed", childPath, len(arr)))
				}
				out[k] = []any{}
				continue
			}
			if lk == "certificates" {
				if arr, ok := child.([]any); ok && len(arr) > 0 {
					*warnings = append(*warnings, childPath+": certificate paths removed")
				}
				out[k] = []any{}
				continue
			}
			if lk == "servername" {
				if s, ok := child.(string); ok && strings.TrimSpace(s) != "" {
					*warnings = append(*warnings, childPath+": domain removed")
				}
				out[k] = ""
				continue
			}
			if lk == "shortids" {
				if arr, ok := child.([]any); ok && len(arr) > 0 {
					*warnings = append(*warnings, childPath+": Reality short IDs removed")
				}
				out[k] = []any{}
				continue
			}
			out[k] = sanitizeTemplateValue(child, childPath, protocol, warnings)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, child := range x {
			out[i] = sanitizeTemplateValue(child, fmt.Sprintf("%s[%d]", path, i), protocol, warnings)
		}
		return out
	default:
		return v
	}
}

func sanitizeTemplateContent(kind string, raw []byte) ([]byte, []string, error) {
	if kind != "inbound" && kind != "xray_config" {
		return nil, nil, errors.New("kind must be inbound or xray_config")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil, errors.New("content is required")
	}
	if len(raw) > localTemplateMaxBytes {
		return nil, nil, fmt.Errorf("content is larger than %d KiB", localTemplateMaxBytes>>10)
	}
	obj, err := parseTemplateObject(raw)
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	clean := sanitizeTemplateValue(obj, "", "", &warnings)
	encoded, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("encode sanitized template: %w", err)
	}
	if len(encoded) > localTemplateMaxBytes {
		return nil, nil, fmt.Errorf("sanitized content is larger than %d KiB", localTemplateMaxBytes>>10)
	}
	return encoded, warnings, nil
}

func templateSummary(kind string, content []byte) map[string]any {
	obj, err := parseTemplateObject(content)
	if err != nil {
		return nil
	}
	result := map[string]any{}
	if kind == "inbound" {
		if p, ok := obj["protocol"].(string); ok && p != "" {
			result["protocol"] = p
		}
		if s, ok := obj["streamSettings"].(map[string]any); ok {
			if n, ok := s["network"].(string); ok && n != "" {
				result["network"] = n
			}
			if sec, ok := s["security"].(string); ok && sec != "" {
				result["security"] = sec
			}
		}
	}
	if kind == "xray_config" {
		if in, ok := obj["inbounds"].([]any); ok {
			result["inbounds"] = len(in)
		}
		if out, ok := obj["outbounds"].([]any); ok {
			result["outbounds"] = len(out)
		}
		if r, ok := obj["routing"].(map[string]any); ok {
			if rules, ok := r["rules"].([]any); ok {
				result["routingRules"] = len(rules)
			}
		}
	}
	return result
}

func templateToView(t *model.LocalTemplate) TemplateView {
	var tags []string
	_ = json.Unmarshal([]byte(t.Tags), &tags)
	if tags == nil {
		tags = []string{}
	}
	return TemplateView{
		Id: t.Id, Kind: t.Kind, Title: t.Title, Description: t.Description, Tags: tags,
		SizeBytes: t.SizeBytes, CreatedAt: t.CreatedAt.Unix(), UpdatedAt: t.UpdatedAt.Unix(),
		Summary: templateSummary(t.Kind, []byte(t.Content)),
	}
}

func (s *TemplateService) validateMeta(in *TemplateInput) ([]string, error) {
	if in.Kind != "inbound" && in.Kind != "xray_config" {
		return nil, errors.New("kind must be inbound or xray_config")
	}
	in.Title = strings.TrimSpace(in.Title)
	if n := utf8.RuneCountInString(in.Title); n < 1 || n > 120 {
		return nil, errors.New("title must be between 1 and 120 characters")
	}
	in.Description = strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(in.Description) > 1000 {
		return nil, errors.New("description must not exceed 1000 characters")
	}
	return normalizeTemplateTags(in.Tags)
}

type TemplateService struct{}

func (s *TemplateService) List(kind, query string, limit, offset int) ([]TemplateView, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	db := database.GetDB().Model(&model.LocalTemplate{})
	if kind != "" {
		db = db.Where("kind = ?", kind)
	}
	if q := strings.TrimSpace(strings.ToLower(query)); q != "" {
		like := "%" + q + "%"
		db = db.Where("LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(tags) LIKE ?", like, like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.LocalTemplate
	if err := db.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]TemplateView, 0, len(rows))
	for i := range rows {
		items = append(items, templateToView(&rows[i]))
	}
	return items, total, nil
}

func (s *TemplateService) Get(id int) (*TemplateDetail, error) {
	var row model.LocalTemplate
	if err := database.GetDB().First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("template not found")
		}
		return nil, err
	}
	return &TemplateDetail{TemplateView: templateToView(&row), Content: json.RawMessage(row.Content)}, nil
}

func (s *TemplateService) Sanitize(in TemplateInput) (*TemplateSanitizeResult, error) {
	clean, warnings, err := sanitizeTemplateContent(in.Kind, in.Content)
	if err != nil {
		return nil, err
	}
	return &TemplateSanitizeResult{Kind: in.Kind, Content: json.RawMessage(clean), Warnings: warnings, SizeBytes: len(clean)}, nil
}

func (s *TemplateService) Save(in TemplateInput) (*TemplateView, []string, error) {
	tags, err := s.validateMeta(&in)
	if err != nil {
		return nil, nil, err
	}
	clean, warnings, err := sanitizeTemplateContent(in.Kind, in.Content)
	if err != nil {
		return nil, nil, err
	}
	db := database.GetDB()
	var count int64
	if err := db.Model(&model.LocalTemplate{}).Count(&count).Error; err != nil {
		return nil, nil, err
	}
	if count >= localTemplateMaxCount {
		return nil, nil, errors.New("local template limit reached")
	}
	row := &model.LocalTemplate{
		Kind: in.Kind, Title: in.Title, Description: in.Description,
		SizeBytes: len(clean), Content: string(clean),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	encodedTags, _ := json.Marshal(tags)
	row.Tags = string(encodedTags)
	if err := db.Create(row).Error; err != nil {
		return nil, nil, err
	}
	result := templateToView(row)
	return &result, warnings, nil
}

func (s *TemplateService) Delete(id int) error {
	res := database.GetDB().Delete(&model.LocalTemplate{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("template not found")
	}
	return nil
}
