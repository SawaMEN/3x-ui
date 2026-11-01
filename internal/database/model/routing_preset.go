package model

import "time"

// RoutingPreset stores a reusable Xray routing.rules array for one panel user.
type RoutingPreset struct {
	Id          int       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId      int       `json:"-" gorm:"index;not null"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	Rules       string    `json:"-" gorm:"type:text;not null"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
