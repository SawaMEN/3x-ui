package service

import (
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

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

type ManagerListOptions struct {
	Page    int
	Search  string
	Compact bool
}

type ManagerSnapshot struct {
	SubscriptionTotal int64                        `json:"subscriptionTotal"`
	AutoRotateTotal   int64                        `json:"autoRotateTotal"`
	PendingOperations int64                        `json:"pendingOperations"`
	SubscriptionPage  int                          `json:"subscriptionPage"`
	PageSize          int                          `json:"pageSize"`
	Subscriptions     []model.ManagerSubscription  `json:"subscriptions"`
	Relays            []model.ManagerRelay         `json:"relays"`
	Domains           []model.ManagerDomain        `json:"domains"`
	Operations        []model.ManagerOperation     `json:"operations"`
	OperationTotal    int64                        `json:"operationTotal"`
	Clients           map[string]ManagerClientInfo `json:"clients"`
}

func (s *ManagerService) Snapshot(owner, page int, state, kind string, options ...ManagerListOptions) (ManagerSnapshot, error) {
	db := database.GetDB()
	v := ManagerSnapshot{Subscriptions: []model.ManagerSubscription{}, Relays: []model.ManagerRelay{}, Domains: []model.ManagerDomain{}, Operations: []model.ManagerOperation{}, Clients: map[string]ManagerClientInfo{}}
	query := db.Model(&model.ManagerSubscription{}).Where("user_id = ?", owner)
	v.SubscriptionPage, v.PageSize = 1, 25
	if len(options) > 0 {
		v.SubscriptionPage = max(1, options[0].Page)
		if needle := strings.TrimSpace(options[0].Search); needle != "" {
			parts, args := textColumnSearchConditions([]string{"name", "note"}, caseVariants(needle))
			query = query.Where("("+strings.Join(parts, " OR ")+")", args...)
		}
	}
	if err := query.Count(&v.SubscriptionTotal).Error; err != nil {
		return v, err
	}
	if len(options) > 0 {
		query = query.Offset((v.SubscriptionPage - 1) * v.PageSize).Limit(v.PageSize)
	}
	if err := query.Order("id DESC").Find(&v.Subscriptions).Error; err != nil {
		return v, err
	}
	if len(options) > 0 && options[0].Compact {
		for i := range v.Subscriptions {
			sub := &v.Subscriptions[i]
			sub.Published = nil
			sub.Retired = nil
			sub.Happ = ""
			sub.Incy = ""
			for j := range sub.Entries {
				sub.Entries[j].Config = nil
				sub.Entries[j].CustomLink = ""
			}
		}
	}
	if err := db.Model(&model.ManagerSubscription{}).Where("user_id = ? AND enabled = ? AND auto_rotate = ?", owner, true, true).Count(&v.AutoRotateTotal).Error; err != nil {
		return v, err
	}
	if err := db.Model(&model.ManagerOperation{}).Where("user_id = ? AND state IN ?", owner, []string{"queued", "running"}).Count(&v.PendingOperations).Error; err != nil {
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
