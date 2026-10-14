package model

// RoutingList caches a user supplied domain/IP list. Rules refer to its ID so
// updates do not rewrite the user's template or change rule order/targets.
type RoutingList struct {
	Id             int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	UpdateInterval int    `json:"updateInterval"`
	Entries        string `json:"-" gorm:"type:text"`
	DomainCount    int    `json:"domainCount"`
	IPCount        int    `json:"ipCount"`
	LastUpdated    int64  `json:"lastUpdated"`
	LastAttempt    int64  `json:"-"`
	LastError      string `json:"lastError"`
}
