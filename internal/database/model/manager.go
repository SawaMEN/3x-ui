package model

type ManagerInboundConfig struct {
	ShareAddrStrategy string   `json:"shareAddrStrategy"`
	ShareAddr         string   `json:"shareAddr"`
	SubSortIndex      int      `json:"subSortIndex"`
	Protocol          Protocol `json:"protocol"`
	Settings          string   `json:"settings"`
	StreamSettings    string   `json:"streamSettings"`
	Sniffing          string   `json:"sniffing"`
	Listen            string   `json:"listen"`
	DisableFlow       bool     `json:"disableFlow"`
}

type ManagerEntry struct {
	Preset                string                `json:"preset,omitempty"`
	Config                *ManagerInboundConfig `json:"config,omitempty"`
	TemplateID            int                   `json:"templateId"`
	Mode                  string                `json:"mode"`
	NodeID                *int                  `json:"nodeId,omitempty"`
	Port                  string                `json:"port"`
	SNI                   string                `json:"sni"`
	SNIBackups            []string              `json:"sniBackups,omitempty"`
	RealityTarget         string                `json:"realityTarget,omitempty"`
	Label                 string                `json:"label"`
	Flag                  string                `json:"flag"`
	RelayID               int                   `json:"relayId"`
	CustomLink            string                `json:"customLink"`
	ActiveInboundID       int                   `json:"activeInboundId"`
	SNIHealthFailures     int                   `json:"sniHealthFailures,omitempty"`
	SNIHealthCheckedAt    int64                 `json:"sniHealthCheckedAt,omitempty"`
	SNIHealthLastSwitchAt int64                 `json:"sniHealthLastSwitchAt,omitempty"`
}

type ManagerRetiredGeneration struct {
	Entries   []ManagerEntry `json:"entries"`
	ExpiresAt int64          `json:"expiresAt"`
}

type ManagerSubscription struct {
	ID                        int                        `json:"id" gorm:"primaryKey"`
	UserID                    int                        `json:"-" gorm:"index;not null"`
	Token                     string                     `json:"token" gorm:"uniqueIndex;not null"`
	Name                      string                     `json:"name"`
	Note                      string                     `json:"note"`
	SupportURL                string                     `json:"supportUrl"`
	WebsiteURL                string                     `json:"websiteUrl"`
	ClientID                  int                        `json:"clientId"`
	OwnClient                 bool                       `json:"ownClient"`
	Enabled                   bool                       `json:"enabled"`
	AutoRotate                bool                       `json:"autoRotate"`
	MonitorSNI                bool                       `json:"monitorSni"`
	MonitorSNIIntervalMinutes int                        `json:"monitorSniIntervalMinutes"`
	NextSNIHealthCheck        int64                      `json:"nextSniHealthCheck" gorm:"index"`
	IntervalMinutes           int                        `json:"intervalMinutes"`
	GraceMinutes              int                        `json:"graceMinutes"`
	Retired                   []ManagerRetiredGeneration `json:"-" gorm:"serializer:json;type:text"`
	NextRotation              int64                      `json:"nextRotation" gorm:"index"`
	LastRotation              int64                      `json:"lastRotation"`
	Happ                      string                     `json:"happ"`
	Incy                      string                     `json:"incy"`
	Entries                   []ManagerEntry             `json:"entries" gorm:"serializer:json;type:text"`
	Published                 []ManagerEntry             `json:"published" gorm:"serializer:json;type:text"`
	CreatedAt                 int64                      `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt                 int64                      `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

type ManagerOperation struct {
	ID             int     `json:"id" gorm:"primaryKey"`
	UserID         int     `json:"-" gorm:"index"`
	SubscriptionID int     `json:"subscriptionId" gorm:"index"`
	RelayID        int     `json:"relayId"`
	Kind           string  `json:"kind"`
	State          string  `json:"state" gorm:"index"`
	ActiveKey      *string `json:"-" gorm:"uniqueIndex"`
	Completed      int     `json:"completed"`
	Total          int     `json:"total"`
	Attempts       int     `json:"attempts"`
	Message        string  `json:"message"`
	CreatedAt      int64   `json:"createdAt" gorm:"autoCreateTime:milli;index"`
	FinishedAt     int64   `json:"finishedAt"`
}

type ManagerRelay struct {
	ID         int    `json:"id" gorm:"primaryKey"`
	UserID     int    `json:"-" gorm:"index"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	SSHPort    int    `json:"sshPort"`
	Username   string `json:"username"`
	HostKey    string `json:"hostKey"`
	Credential string `json:"-"`
	KeyAuth    bool   `json:"keyAuth"`
	NodeID     int    `json:"nodeId"`
	Installed  bool   `json:"installed"`
}

type ManagerDomain struct {
	ID     int    `json:"id" gorm:"primaryKey"`
	UserID int    `json:"-" gorm:"uniqueIndex:idx_manager_domain,priority:1"`
	Name   string `json:"name" gorm:"uniqueIndex:idx_manager_domain,priority:2"`
}
