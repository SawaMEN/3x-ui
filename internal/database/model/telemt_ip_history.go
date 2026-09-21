package model

// TelemtIPHistory stores persistent connection history collected by the panel.
// Traffic counters are maintained from Linux TCP socket statistics while the
// Telemt monitoring collector is running. LastSeenAt records the latest active
// observation made by the panel collector, not Telemt's approximate recent window.
type TelemtIPHistory struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	IP            string `json:"ip" gorm:"uniqueIndex;not null"`
	Users         string `json:"users" gorm:"type:text"`
	FirstSeenAt   int64  `json:"firstSeenAt" gorm:"not null;index"`
	LastSeenAt    int64  `json:"lastSeenAt" gorm:"not null;index"`
	DownloadBytes int64  `json:"downloadBytes" gorm:"not null;default:0"`
	UploadBytes   int64  `json:"uploadBytes" gorm:"not null;default:0"`
	TotalBytes    int64  `json:"totalBytes" gorm:"not null;default:0"`
}

func (TelemtIPHistory) TableName() string { return "telemt_ip_histories" }
