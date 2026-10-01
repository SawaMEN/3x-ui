package model

// TrafficHistory stores one aggregated traffic bucket for a resource/tag pair.
type TrafficHistory struct {
	Id        uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	Resource  string `json:"resource" gorm:"uniqueIndex:idx_traffic_history_bucket,priority:1;index:idx_traffic_history_resource_tag"`
	Tag       string `json:"tag" gorm:"uniqueIndex:idx_traffic_history_bucket,priority:2;index:idx_traffic_history_resource_tag"`
	DateTime  int64  `json:"dateTime" gorm:"uniqueIndex:idx_traffic_history_bucket,priority:3;index:idx_traffic_history_date_time"`
	Direction string `json:"direction" gorm:"uniqueIndex:idx_traffic_history_bucket,priority:4"`
	Traffic   int64  `json:"traffic"`
}
