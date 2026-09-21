package model

// TelemtIPGeo keeps a persistent, localized geolocation cache.
// IP locations change rarely, so the monitor avoids repeating external lookups.
type TelemtIPGeo struct {
	Id            int     `json:"id" gorm:"primaryKey;autoIncrement"`
	IP            string  `json:"ip" gorm:"uniqueIndex;not null"`
	City          string  `json:"city"`
	Region        string  `json:"region"`
	Country       string  `json:"country"`
	CountryCode   string  `json:"countryCode"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	LastCheckedAt int64   `json:"lastCheckedAt" gorm:"not null;index"`
}

func (TelemtIPGeo) TableName() string { return "telemt_ip_geos" }
