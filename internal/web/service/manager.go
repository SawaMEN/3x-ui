package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/link"
)

var managerMu sync.Mutex

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
		in.Token, in.OwnClient, in.Published, in.LastRotation, in.CreatedAt, in.Retired = old.Token, old.OwnClient, old.Published, old.LastRotation, old.CreatedAt, old.Retired
	} else {
		in.Token = uuid.NewString()
		in.OwnClient = false
		in.Published = nil
		in.Retired = nil
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
	if in.GraceMinutes == 0 {
		in.GraceMinutes = 120
	}
	if in.GraceMinutes < 60 || in.GraceMinutes > 1440 {
		return fmt.Errorf("grace period must be 60–1440 minutes")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 256 || len(in.Note) > 4096 || len(in.Entries) < 1 || len(in.Entries) > 100 {
		return fmt.Errorf("provide a name and 1–100 connections")
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
		if e.RealityTarget != "" {
			if e.Mode == "existing" {
				return fmt.Errorf("a REALITY target cannot override an existing inbound")
			}
			if _, err := managerRealityTarget(e.RealityTarget); err != nil {
				return fmt.Errorf("entry %d: %w", i+1, err)
			}
			var stream struct {
				Security string `json:"security"`
			}
			if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil || stream.Security != "reality" {
				return fmt.Errorf("entry %d: a camouflage target requires REALITY security", i+1)
			}
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
	if kind != "rotate" && kind != "delete" && kind != "cleanup" {
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
	if op.State != "failed" && op.State != "cleanup_pending" {
		return nil, fmt.Errorf("only failed operations may be retried; inspect interrupted operations first")
	}
	if op.RelayID > 0 {
		return s.EnqueueRelay(owner, op.RelayID, op.Kind)
	}
	kind := op.Kind
	if op.State == "cleanup_pending" {
		kind = "cleanup"
	}
	return s.Enqueue(owner, op.SubscriptionID, kind)
}
