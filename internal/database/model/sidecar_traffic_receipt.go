package model

// SidecarTrafficReceipt makes durable sidecar journal replay idempotent.
type SidecarTrafficReceipt struct {
	ID string `gorm:"primaryKey"`
}
