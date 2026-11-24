package service

import (
	"cmp"
	"slices"
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
	SubscriptionTotal     int64                        `json:"subscriptionTotal"`
	AllSubscriptionsTotal int64                        `json:"allSubscriptionsTotal"`
	AutoRotateTotal       int64                        `json:"autoRotateTotal"`
	PendingOperations     int64                        `json:"pendingOperations"`
	SubscriptionPage      int                          `json:"subscriptionPage"`
	PageSize              int                          `json:"pageSize"`
	Subscriptions         []model.ManagerSubscription  `json:"subscriptions"`
	Relays                []model.ManagerRelay         `json:"relays"`
	Domains               []model.ManagerDomain        `json:"domains"`
	Operations            []model.ManagerOperation     `json:"operations"`
	ActiveOperations      []model.ManagerOperation     `json:"activeOperations"`
	OperationTotal        int64                        `json:"operationTotal"`
	Clients               map[string]ManagerClientInfo `json:"clients"`
}

func (s *ManagerService) Snapshot(owner, page int, state, kind string, options ...ManagerListOptions) (ManagerSnapshot, error) {
	db := database.GetDB()
	v := ManagerSnapshot{Subscriptions: []model.ManagerSubscription{}, Relays: []model.ManagerRelay{}, Domains: []model.ManagerDomain{}, Operations: []model.ManagerOperation{}, ActiveOperations: []model.ManagerOperation{}, Clients: map[string]ManagerClientInfo{}}
	query := db.Model(&model.ManagerSubscription{}).Where("user_id = ?", owner)
	var totals struct {
		AllSubscriptionsTotal int64
		AutoRotateTotal       int64
	}
	if err := db.Model(&model.ManagerSubscription{}).Where("user_id = ?", owner).
		Select("COUNT(*) AS all_subscriptions_total, COALESCE(SUM(CASE WHEN enabled = TRUE AND auto_rotate = TRUE THEN 1 ELSE 0 END), 0) AS auto_rotate_total").
		Scan(&totals).Error; err != nil {
		return v, err
	}
	v.AllSubscriptionsTotal, v.AutoRotateTotal = totals.AllSubscriptionsTotal, totals.AutoRotateTotal
	v.SubscriptionTotal = v.AllSubscriptionsTotal
	v.SubscriptionPage, v.PageSize = 1, 25
	if len(options) > 0 {
		v.SubscriptionPage = max(1, options[0].Page)
		if needle := strings.TrimSpace(options[0].Search); needle != "" {
			parts, args := textColumnSearchConditions([]string{"name", "note"}, caseVariants(needle))
			query = query.Where("("+strings.Join(parts, " OR ")+")", args...)
			if err := query.Count(&v.SubscriptionTotal).Error; err != nil {
				return v, err
			}
		}
	}
	if len(options) == 0 || managerPageExists(v.SubscriptionPage, v.PageSize, v.SubscriptionTotal) {
		if len(options) > 0 {
			query = query.Offset((v.SubscriptionPage - 1) * v.PageSize).Limit(v.PageSize)
		}
		if err := query.Order("id DESC").Find(&v.Subscriptions).Error; err != nil {
			return v, err
		}
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
	if err := db.Model(&model.ManagerOperation{}).Where("user_id = ? AND state IN ?", owner, []string{"queued", "running"}).Count(&v.PendingOperations).Error; err != nil {
		return v, err
	}
	// Active jobs on the visible subscription page must not depend on
	// journal pagination or its independently selected status filters.
	if len(v.Subscriptions) > 0 {
		subscriptionIDs := make([]int, 0, len(v.Subscriptions))
		for _, sub := range v.Subscriptions {
			subscriptionIDs = append(subscriptionIDs, sub.ID)
		}
		for _, batch := range chunkInts(subscriptionIDs, sqlInChunk) {
			var operations []model.ManagerOperation
			if err := db.Where("user_id = ? AND subscription_id IN ? AND state IN ?", owner, batch, []string{"queued", "running"}).Order("id DESC").Find(&operations).Error; err != nil {
				return v, err
			}
			v.ActiveOperations = append(v.ActiveOperations, operations...)
		}
		slices.SortFunc(v.ActiveOperations, func(a, b model.ManagerOperation) int { return cmp.Compare(b.ID, a.ID) })
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
		slices.Sort(ids)
		ids = slices.Compact(ids)
		var clients []model.ClientRecord
		for _, batch := range chunkInts(ids, sqlInChunk) {
			var rows []model.ClientRecord
			if err := db.Select("id", "email", "total_gb", "expiry_time", "enable").Where("id IN ?", batch).Find(&rows).Error; err != nil {
				return v, err
			}
			clients = append(clients, rows...)
		}
		emails := make([]string, 0, len(clients))
		for _, c := range clients {
			emails = append(emails, c.Email)
		}
		var traffic []*coremodel.ClientTraffic
		for _, batch := range chunkStrings(emails, sqlInChunk) {
			var rows []*coremodel.ClientTraffic
			if err := db.Select("email", "up", "down").Where("email IN ?", batch).Find(&rows).Error; err != nil {
				return v, err
			}
			traffic = append(traffic, rows...)
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
	page = max(1, page)
	if !managerPageExists(page, 20, v.OperationTotal) {
		return v, nil
	}
	return v, q.Order("id DESC").Offset((page - 1) * 20).Limit(20).Find(&v.Operations).Error
}

// Guard offset multiplication and skip queries for pages beyond the result set.
func managerPageExists(page, pageSize int, total int64) bool {
	return total > 0 && int64(page-1) <= (total-1)/int64(pageSize)
}
