package model

// TelemtUserHistory stores persistent monitoring state per Telemt user.
// Current/recent IPs are JSON arrays so the dashboard can show the user's
// active locations without scanning socket state.
type TelemtUserHistory struct {
	Id                 int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Username           string `json:"username" gorm:"uniqueIndex;not null"`
	ActiveIPs          string `json:"activeIPs" gorm:"type:text"`
	RecentIPs          string `json:"recentIPs" gorm:"type:text"`
	CurrentConnections int    `json:"currentConnections" gorm:"not null;default:0"`
	FirstSeenAt        int64  `json:"firstSeenAt" gorm:"not null;index"`
	LastSeenAt         int64  `json:"lastSeenAt" gorm:"not null;index"`
	TotalBytes         int64  `json:"totalBytes" gorm:"not null;default:0"`
	SourceTotalBytes   int64  `json:"-" gorm:"not null;default:0"`
}

func (TelemtUserHistory) TableName() string { return "telemt_user_histories" }
