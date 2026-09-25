package model

import "time"

// LocalTemplate stores a reusable, sanitized configuration template in the panel database.
// Content never contains live client credentials because the template service sanitizes it on write.
type LocalTemplate struct {
	Id          int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Kind        string    `json:"kind" gorm:"index;not null"`
	Title       string    `json:"title" gorm:"not null"`
	Description string    `json:"description"`
	Tags        string    `json:"-" gorm:"type:text"`
	Content     string    `json:"content" gorm:"type:text;not null"`
	SizeBytes   int       `json:"sizeBytes"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
