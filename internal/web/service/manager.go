package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/link"
)

var managerMu sync.Mutex

type (
	ManagerService    struct{}
	ManagerClientInfo struct {
		Email      string `json:"email"`
		Up         int64  `json:"up"`
		Down       int64  `json:"down"`
		Total      int64  `json:"total"`
		ExpiryTime int64  `json:"expiryTime"`
		Enabled    bool   `json:"enabled"`
	}
)

type ManagerSnapshot struct {
	Subscriptions  []model.ManagerSubscription  `json:"subscriptions"`
	Relays         []model.ManagerRelay         `json:"relays"`
	Domains        []model.ManagerDomain        `json:"domains"`
	Operations     []model.ManagerOperation     `json:"operations"`
	OperationTotal int64                        `json:"operationTotal"`
	Clients        map[string]ManagerClientInfo `json:"clients"`
}

func (s *ManagerService) Snapshot(owner, page int, state, kind string) (ManagerSnapshot, error) {
	db := database.GetDB()
	v := ManagerSnapshot{Subscriptions: []model.ManagerSubscription{}, Relays: []model.ManagerRelay{}, Domains: []model.ManagerDomain{}, Operations: []model.ManagerOperation{}, Clients: map[string]ManagerClientInfo{}}
	if err := db.Where("user_id = ?", owner).Order("id DESC").Find(&v.Subscriptions).Error; err != nil {
		return v, err
	}
	if err := db.Where("user_id = ?", owner).Find(&v.Relays).Error; err != nil {
		return v, err
	}
	if err := db.Where("user_id = ?", owner).Order("name").Find(&v.Domains).Error; err != nil {
		return v, err
	}
	ids := []int{}
	for _, sub := range v.Subscriptions {
		if sub.ClientID > 0 {
			ids = append(ids, sub.ClientID)
		}
	}
	if len(ids) > 0 {
		var clients []model.ClientRecord
		if err := db.Where("id IN ?", ids).Find(&clients).Error; err != nil {
			return v, err
		}
		emails := []string{}
		for _, c := range clients {
			emails = append(emails, c.Email)
		}
		var traffic []*coremodel.ClientTraffic
		if err := db.Where("email IN ?", emails).Find(&traffic).Error; err != nil {
			return v, err
		}
		overlayGlobalTraffic(db, traffic)
		byEmail := map[string]*coremodel.ClientTraffic{}
		for _, tr := range traffic {
			byEmail[tr.Email] = tr
		}
		for _, c := range clients {
			info := ManagerClientInfo{Email: c.Email, Total: c.TotalGB, ExpiryTime: c.ExpiryTime, Enabled: c.Enable}
			if tr := byEmail[c.Email]; tr != nil {
				info.Up, info.Down = tr.Up, tr.Down
			}
			v.Clients[strconv.Itoa(c.Id)] = info
		}
	}
	q := db.Model(&model.ManagerOperation{}).Where("user_id = ?", owner)
	if state != "" {
		q = q.Where("state = ?", state)
	}
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if err := q.Count(&v.OperationTotal).Error; err != nil {
		return v, err
	}
	return v, q.Order("id DESC").Offset((max(1, page) - 1) * 20).Limit(20).Find(&v.Operations).Error
}

func (s *ManagerService) Save(owner int, in model.ManagerSubscription) (*model.ManagerSubscription, error) {
	managerMu.Lock()
	defer managerMu.Unlock()
	db := database.GetDB()
	if in.ID > 0 {
		var old model.ManagerSubscription
		if err := db.Where("id = ? AND user_id = ?", in.ID, owner).First(&old).Error; err != nil {
			return nil, err
		}
		var n int64
		if err := db.Model(&model.ManagerOperation{}).Where("subscription_id = ? AND state IN ?", in.ID, []string{"queued", "running"}).Count(&n).Error; err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, fmt.Errorf("subscription has a pending operation")
		}
		if in.ClientID != old.ClientID {
			return nil, fmt.Errorf("client cannot be changed after creation")
		}
		in.Token, in.OwnClient, in.Published, in.LastRotation, in.CreatedAt = old.Token, old.OwnClient, old.Published, old.LastRotation, old.CreatedAt
	} else {
		in.Token = uuid.NewString()
		in.OwnClient = false
		in.Published = nil
		in.LastRotation = 0
		in.CreatedAt = 0
	}
	in.UserID = owner
	for i := range in.Entries {
		in.Entries[i].ActiveInboundID = 0
	}
	if err := s.validate(&in); err != nil {
		return nil, err
	}
	in.NextRotation = time.Now().Add(time.Duration(in.IntervalMinutes) * time.Minute).UnixMilli()
	return &in, db.Save(&in).Error
}

func (s *ManagerService) validate(in *model.ManagerSubscription) error {
	if in.ID < 0 || in.ClientID < 0 {
		return fmt.Errorf("invalid subscription or client ID")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 256 || len(in.Note) > 4096 || len(in.Entries) < 1 || len(in.Entries) > 20 {
		return fmt.Errorf("provide a name and 1–20 connections")
	}
	if in.IntervalMinutes < 10 || in.IntervalMinutes > 525600 {
		return fmt.Errorf("interval must be 10–525600 minutes")
	}
	for _, u := range []string{in.SupportURL, in.WebsiteURL} {
		if u != "" && (!isHTTPURL(u) || strings.ContainsAny(u, "\r\n")) {
			return fmt.Errorf("support and website must be HTTP(S) URLs")
		}
	}
	var err error
	if in.Happ, err = managerRouting("happ", in.Happ); err != nil {
		return err
	}
	if in.Incy, err = managerRouting("incy", in.Incy); err != nil {
		return err
	}
	if in.ClientID > 0 {
		if err := database.GetDB().First(&model.ClientRecord{}, in.ClientID).Error; err != nil {
			return err
		}
	}
	for i := range in.Entries {
		e := &in.Entries[i]
		if len(e.Label) > 256 || len(e.Flag) > 16 || strings.ContainsAny(e.Label+e.Flag, "\r\n") {
			return fmt.Errorf("invalid connection name")
		}
		if e.Mode == "custom" || e.CustomLink != "" {
			if len(e.CustomLink) > 65536 || strings.ContainsAny(e.CustomLink, "\r\n") {
				return fmt.Errorf("invalid external link")
			}
			if _, err := link.ParseLink(e.CustomLink); err != nil {
				return err
			}
			e.Mode = "custom"
			e.TemplateID = 0
			e.RelayID = 0
			continue
		}
		if e.Mode != "existing" && e.Mode != "rotate" && e.Mode != "generate" {
			return fmt.Errorf("select automatic generation, template rotation, or an existing connection")
		}
		ib, err := managerEntrySource(*e, in.UserID)
		if err != nil {
			return fmt.Errorf("entry %d: %w", i+1, err)
		}
		if e.Mode == "existing" {
			if e.NodeID != nil || e.SNI != "" || (e.Port != "" && e.Port != strconv.Itoa(ib.Port)) {
				return fmt.Errorf("use rotation mode to override node, SNI or port")
			}
			e.ActiveInboundID = ib.Id
		} else {
			if e.Port == "" {
				e.Port = "random"
			}
			if e.Port != "random" {
				p, err := strconv.Atoi(e.Port)
				if err != nil || p < 1 || p > 65535 {
					return fmt.Errorf("invalid port")
				}
				if in.AutoRotate {
					return fmt.Errorf("automatic rotation requires random ports")
				}
			}
			if e.NodeID != nil {
				ib.NodeID = e.NodeID
				if *e.NodeID == 0 {
					ib.NodeID = nil
				}
			}
			if err := validateInboundRuntimeProtocol(ib.Protocol, ib); err != nil {
				return err
			}
			if e.SNI != "" && e.SNI != "random" {
				if _, err := managerDomain(e.SNI); err != nil {
					return err
				}
			}
		}
		if e.RelayID > 0 {
			var r model.ManagerRelay
			if err := database.GetDB().Where("id = ? AND user_id = ?", e.RelayID, in.UserID).First(&r).Error; err != nil {
				return err
			}
			if !r.Installed || ib.NodeID == nil || *ib.NodeID != r.NodeID {
				return fmt.Errorf("relay must be installed and point to the selected node")
			}
		}
		if ib.NodeID != nil {
			var n model.Node
			if err := database.GetDB().First(&n, *ib.NodeID).Error; err != nil {
				return err
			}
			if name, err := managerDomain(n.Address); err == nil {
				if err := s.ImportDomains(in.UserID, name, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func managerRouting(scheme, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || value == scheme+"://routing/off" {
		return value, nil
	}
	payload, action := value, "add"
	for _, prefix := range []string{scheme + "://routing/", scheme + "://autorouting/"} {
		if strings.HasPrefix(value, prefix) {
			parts := strings.SplitN(strings.TrimPrefix(value, prefix), "/", 2)
			if len(parts) != 2 || (parts[0] != "add" && parts[0] != "onadd") {
				return "", fmt.Errorf("invalid routing action")
			}
			action, payload = parts[0], parts[1]
		}
	}
	if scheme == "incy" && isHTTPURL(payload) && !strings.ContainsAny(payload, " \t\r\n") {
		return "incy://autorouting/" + action + "/" + payload, nil
	}
	data := []byte(payload)
	if !strings.HasPrefix(payload, "{") {
		var err error
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", fmt.Errorf("invalid routing profile")
		}
	}
	var obj map[string]any
	if len(data) > 65536 || json.Unmarshal(data, &obj) != nil || obj == nil {
		return "", fmt.Errorf("routing profile must be a JSON object")
	}
	data, _ = json.Marshal(obj)
	return scheme + "://routing/" + action + "/" + base64.StdEncoding.EncodeToString(data), nil
}

func managerDomain(raw string) (string, error) {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if u, err := url.Parse(host); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") || len(host) > 253 {
		return "", fmt.Errorf("invalid SNI domain")
	}
	for _, p := range strings.Split(host, ".") {
		if p == "" || len(p) > 63 || p[0] == '-' || p[len(p)-1] == '-' {
			return "", fmt.Errorf("invalid domain")
		}
		for _, r := range p {
			letter := r >= 'a' && r <= 'z'
			digit := r >= '0' && r <= '9'
			if !letter && !digit && r != '-' {
				return "", fmt.Errorf("invalid domain")
			}
		}
	}
	return host, nil
}

func (s *ManagerService) ImportDomains(owner int, raw string, replace bool) error {
	if len(raw) > 1<<20 {
		return fmt.Errorf("import exceeds 1 MiB")
	}
	rows := []model.ManagerDomain{}
	for _, v := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t' }) {
		if net.ParseIP(strings.Trim(v, "[]")) != nil {
			continue
		}
		name, err := managerDomain(v)
		if err != nil {
			return err
		}
		rows = append(rows, model.ManagerDomain{UserID: owner, Name: name})
	}
	if len(rows) > 10000 {
		return fmt.Errorf("maximum 10000 domains")
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if replace {
			if err := tx.Where("user_id = ?", owner).Delete(&model.ManagerDomain{}).Error; err != nil {
				return err
			}
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 100).Error
	})
}

func (s *ManagerService) Enqueue(owner, id int, kind string) (*model.ManagerOperation, error) {
	managerMu.Lock()
	defer managerMu.Unlock()
	return s.enqueue(owner, id, kind)
}

func (s *ManagerService) enqueue(owner, id int, kind string) (*model.ManagerOperation, error) {
	if kind != "rotate" && kind != "delete" {
		return nil, fmt.Errorf("invalid operation")
	}
	var sub model.ManagerSubscription
	db := database.GetDB()
	if err := db.Where("id = ? AND user_id = ?", id, owner).First(&sub).Error; err != nil {
		return nil, err
	}
	if kind == "rotate" && !sub.Enabled {
		return nil, fmt.Errorf("subscription is disabled")
	}
	key := fmt.Sprintf("subscription/%d", id)
	var old model.ManagerOperation
	if err := db.Where("active_key = ?", key).First(&old).Error; err == nil {
		return &old, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	op := &model.ManagerOperation{UserID: owner, SubscriptionID: id, Kind: kind, State: "queued", ActiveKey: &key, Total: len(sub.Entries)}
	return op, db.Create(op).Error
}

func (s *ManagerService) EnqueueAll(owner int) error {
	managerMu.Lock()
	defer managerMu.Unlock()
	var subs []model.ManagerSubscription
	if err := database.GetDB().Where("user_id = ? AND enabled = ? AND auto_rotate = ?", owner, true, true).Find(&subs).Error; err != nil {
		return err
	}
	for _, sub := range subs {
		if _, err := s.enqueue(owner, sub.ID, "rotate"); err != nil {
			return err
		}
	}
	return nil
}

func (s *ManagerService) Retry(owner, id int) (*model.ManagerOperation, error) {
	var op model.ManagerOperation
	if err := database.GetDB().Where("id = ? AND user_id = ?", id, owner).First(&op).Error; err != nil {
		return nil, err
	}
	if op.State != "failed" {
		return nil, fmt.Errorf("only failed operations may be retried; inspect interrupted operations first")
	}
	if op.RelayID > 0 {
		return s.EnqueueRelay(owner, op.RelayID, op.Kind)
	}
	return s.Enqueue(owner, op.SubscriptionID, op.Kind)
}

func ServeManagerSubscription(w http.ResponseWriter, r *http.Request, token string) {
	if _, err := uuid.Parse(token); err != nil {
		http.NotFound(w, r)
		return
	}
	db := database.GetDB()
	var sub model.ManagerSubscription
	if err := db.Where("token = ? AND enabled = ?", token, true).First(&sub).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	var client model.ClientRecord
	var traffic *coremodel.ClientTraffic
	if sub.ClientID > 0 {
		if err := db.First(&client, sub.ClientID).Error; err != nil || !client.Enable || client.ExpiryTime > 0 && client.ExpiryTime < time.Now().UnixMilli() {
			http.NotFound(w, r)
			return
		}
		var err error
		traffic, err = (&InboundService{}).GetClientTrafficByEmail(client.Email)
		if err != nil {
			http.Error(w, "traffic unavailable", http.StatusServiceUnavailable)
			return
		}
		if traffic != nil && client.TotalGB > 0 && (traffic.Up >= client.TotalGB || traffic.Down >= client.TotalGB-traffic.Up) {
			http.NotFound(w, r)
			return
		}
	}
	provider := subLinkProvider()
	if provider == nil {
		http.Error(w, "renderer unavailable", http.StatusServiceUnavailable)
		return
	}
	format := r.URL.Query().Get("format")
	config := format == "singbox" || format == "hiddify" || format == "xray"
	if format != "" && format != "raw" && format != "base64" && !config {
		http.Error(w, "unknown format", http.StatusBadRequest)
		return
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	out := []string{}
	var configBody, configType string
	if config {
		renderer, ok := provider.(interface {
			ManagerFormat(string, model.ManagerSubscription, model.ClientRecord, string) (string, string, error)
		})
		if !ok {
			http.Error(w, "format unavailable", http.StatusServiceUnavailable)
			return
		}
		var err error
		configBody, configType, err = renderer.ManagerFormat(host, sub, client, format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotAcceptable)
			return
		}
	}

	for _, e := range sub.Published {
		if config {
			break
		}
		if e.CustomLink != "" {
			out = append(out, e.CustomLink)
			continue
		}
		if e.ActiveInboundID < 1 || sub.ClientID < 1 {
			continue
		}
		var ib model.Inbound
		if err := db.Where("id = ? AND user_id = ? AND enable = ?", e.ActiveInboundID, sub.UserID, true).First(&ib).Error; err != nil {
			continue
		}
		var n int64
		if err := db.Model(&model.ClientInbound{}).Where("client_id = ? AND inbound_id = ?", client.Id, ib.Id).Count(&n).Error; err != nil || n == 0 {
			continue
		}
		if e.RelayID > 0 {
			var relay model.ManagerRelay
			if err := db.Where("id = ? AND user_id = ? AND installed = ?", e.RelayID, sub.UserID, true).First(&relay).Error; err != nil {
				continue
			}
			ib.ShareAddrStrategy, ib.ShareAddr = "custom", relay.Address
		}
		ib.Remark = strings.TrimSpace(e.Flag + " " + e.Label)
		if ib.Remark == "" {
			ib.Remark = sub.Name
		}
		if p, ok := provider.(interface {
			ManagerLinksForClient(string, *model.Inbound, string, string, bool) []string
		}); ok {
			links := p.ManagerLinksForClient(host, &ib, client.Email, ib.Remark, e.RelayID > 0)
			if len(links) == 0 {
				http.Error(w, "A selected protocol requires JSON. Use ?format=singbox or ?format=hiddify", http.StatusNotAcceptable)
				return
			}
			out = append(out, links...)
		} else {
			out = append(out, provider.LinksForClient(host, &ib, client.Email)...)
		}
	}
	for _, p := range []string{sub.Happ, sub.Incy} {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 && configBody == "" {
		http.Error(w, "subscription is not generated", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("profile-title", "base64:"+base64.StdEncoding.EncodeToString([]byte(sub.Name)))
	w.Header().Set("profile-update-interval", strconv.Itoa(max(1, sub.IntervalMinutes/60)))
	if sub.Note != "" {
		w.Header().Set("announce", "base64:"+base64.StdEncoding.EncodeToString([]byte(sub.Note)))
	}
	if sub.SupportURL != "" {
		w.Header().Set("support-url", sub.SupportURL)
	}
	if sub.WebsiteURL != "" {
		w.Header().Set("profile-web-page-url", sub.WebsiteURL)
	}
	if traffic != nil {
		w.Header().Set("subscription-userinfo", fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", traffic.Up, traffic.Down, client.TotalGB, max(0, client.ExpiryTime/1000)))
	}
	body := strings.Join(out, "\n")
	if r.URL.Query().Get("format") != "raw" {
		body = base64.StdEncoding.EncodeToString([]byte(body))
	}
	if config {
		body = configBody
		w.Header().Set("Content-Type", configType)
	}
	_, _ = w.Write([]byte(body))
}
