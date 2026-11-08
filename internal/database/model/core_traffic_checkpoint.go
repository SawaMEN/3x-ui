package model

// CoreTrafficCheckpoint is committed atomically with the corresponding usage.
// One row per source counter bounds storage across core restarts.
type CoreTrafficCheckpoint struct {
	Name      string `gorm:"primaryKey"`
	Epoch     string
	Value     int64
	TrafficID int // Stable client_traffics owner across email edits; zero for inbounds.
}
