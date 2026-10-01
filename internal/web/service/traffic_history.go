package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const trafficHistoryBaseBucket int64 = 300
const trafficHistoryRetention = 30 * 24 * time.Hour

// TrafficHistorySample is one traffic delta before it is folded into a bucket.
type TrafficHistorySample struct {
	Resource string
	Tag      string
	Up       int64
	Down     int64
}

// TrafficHistoryPoint is one aggregated history bucket returned to the UI.
type TrafficHistoryPoint struct {
	T    int64 `json:"t"`
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

func validTrafficHistoryResource(resource string) bool {
	switch resource {
	case "client", "inbound", "outbound":
		return true
	default:
		return false
	}
}

// RecordTrafficHistory atomically accumulates non-zero deltas in five-minute buckets.
func RecordTrafficHistory(samples []TrafficHistorySample, now time.Time) error {
	if len(samples) == 0 {
		return nil
	}
	if err := database.EnsureTrafficHistoryModel(); err != nil {
		return err
	}
	bucket := now.Unix() - now.Unix()%trafficHistoryBaseBucket
	rows := make([]model.TrafficHistory, 0, len(samples)*2)
	for _, sample := range samples {
		resource := strings.ToLower(strings.TrimSpace(sample.Resource))
		tag := strings.TrimSpace(sample.Tag)
		if !validTrafficHistoryResource(resource) || tag == "" {
			continue
		}
		if sample.Up > 0 {
			rows = append(rows, model.TrafficHistory{Resource: resource, Tag: tag, DateTime: bucket, Direction: "up", Traffic: sample.Up})
		}
		if sample.Down > 0 {
			rows = append(rows, model.TrafficHistory{Resource: resource, Tag: tag, DateTime: bucket, Direction: "down", Traffic: sample.Down})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return database.GetDB().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "resource"}, {Name: "tag"}, {Name: "date_time"}, {Name: "direction"}},
		DoUpdates: clause.Assignments(map[string]any{"traffic": gorm.Expr("traffic_history.traffic + excluded.traffic")}),
	}).Create(&rows).Error
}

// GetTrafficHistory returns summed buckets; bucketSeconds must align with storage buckets.
func GetTrafficHistory(resource, tag string, bucketSeconds int64, maxPoints int) ([]TrafficHistoryPoint, error) {
	resource = strings.ToLower(strings.TrimSpace(resource))
	tag = strings.TrimSpace(tag)
	if !validTrafficHistoryResource(resource) || tag == "" {
		return nil, fmt.Errorf("invalid traffic history resource or tag")
	}
	if bucketSeconds < trafficHistoryBaseBucket || bucketSeconds%trafficHistoryBaseBucket != 0 {
		return nil, fmt.Errorf("bucket must be a multiple of %d seconds", trafficHistoryBaseBucket)
	}
	if maxPoints < 1 {
		maxPoints = 360
	}
	if maxPoints > 1000 {
		maxPoints = 1000
	}
	if err := database.EnsureTrafficHistoryModel(); err != nil {
		return nil, err
	}
	end := time.Now().Unix()
	start := end - bucketSeconds*int64(maxPoints)
	var rows []model.TrafficHistory
	if err := database.GetDB().Where("resource = ? AND tag = ? AND date_time >= ?", resource, tag, start).
		Order("date_time ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	byTime := make(map[int64]*TrafficHistoryPoint)
	for _, row := range rows {
		t := row.DateTime - row.DateTime%bucketSeconds
		point := byTime[t]
		if point == nil {
			point = &TrafficHistoryPoint{T: t}
			byTime[t] = point
		}
		if row.Direction == "up" {
			point.Up += row.Traffic
		} else if row.Direction == "down" {
			point.Down += row.Traffic
		}
	}
	result := make([]TrafficHistoryPoint, 0, len(byTime))
	for _, point := range byTime {
		result = append(result, *point)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].T < result[j].T })
	return result, nil
}

// PruneTrafficHistory deletes buckets older than the configured retention window.
func PruneTrafficHistory() error {
	if err := database.EnsureTrafficHistoryModel(); err != nil {
		return err
	}
	cutoff := time.Now().Add(-trafficHistoryRetention).Unix()
	return database.GetDB().Where("date_time < ?", cutoff).Delete(&model.TrafficHistory{}).Error
}
