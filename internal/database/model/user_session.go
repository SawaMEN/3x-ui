package model

import "time"

// UserSession tracks a login session without persisting the raw cookie session ID.
// SessionIdHash is SHA-256 of the random ID stored inside the signed/encrypted cookie.
type UserSession struct {
	Id            int        `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId        int        `json:"-" gorm:"index;not null"`
	SessionIdHash string     `json:"-" gorm:"uniqueIndex;not null"`
	IpAddress     string     `json:"ipAddress"`
	UserAgent     string     `json:"userAgent" gorm:"type:text"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastSeenAt    time.Time  `json:"lastSeenAt"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
}
