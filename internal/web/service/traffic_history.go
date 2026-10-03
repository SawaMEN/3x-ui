package service

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const trafficHistoryBaseBucket int64 = 300
const trafficHistoryRetention = 30 * 24 * time.Hour

type TrafficHistorySample struct { Resource, Tag string; Up, Down int64 }
type TrafficHistoryPoint struct { T int64 `json:"t"`; Up int64 `json:"up"`; Down int64 `json:"down"` }
type trafficHistoryCounter struct { resource, tag string; up, down int64 }

var trafficHistorySnapshot = struct {
	sync.Mutex
	initialized bool
	previous map[string]trafficHistoryCounter
	lastPrune time.Time
}{previous: make(map[string]trafficHistoryCounter)}

func validTrafficHistoryResource(resource string) bool {
	return resource == "client" || resource == "inbound" || resource == "outbound"
}

func RecordTrafficHistory(samples []TrafficHistorySample, now time.Time) error {
	if len(samples) == 0 { return nil }
	if err := database.EnsureTrafficHistoryModel(); err != nil { return err }
	bucket := now.Unix() - now.Unix()%trafficHistoryBaseBucket
	rows := make([]model.TrafficHistory, 0, len(samples)*2)
	for _, sample := range samples {
		resource, tag := strings.ToLower(strings.TrimSpace(sample.Resource)), strings.TrimSpace(sample.Tag)
		if !validTrafficHistoryResource(resource) || tag == "" { continue }
		if sample.Up > 0 { rows = append(rows, model.TrafficHistory{Resource: resource, Tag: tag, DateTime: bucket, Direction: "up", Traffic: sample.Up}) }
		if sample.Down > 0 { rows = append(rows, model.TrafficHistory{Resource: resource, Tag: tag, DateTime: bucket, Direction: "down", Traffic: sample.Down}) }
	}
	if len(rows) == 0 { return nil }
	return database.GetDB().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name:"resource"},{Name:"tag"},{Name:"date_time"},{Name:"direction"}},
		DoUpdates: clause.Assignments(map[string]any{"traffic": gorm.Expr("traffic_histories.traffic + excluded.traffic")}),
	}).Create(&rows).Error
}

func historyCounterKey(resource, tag string) string { return resource + "\x00" + tag }
func counterDelta(current, previous int64) int64 {
	if current <= 0 { return 0 }
	if current >= previous { return current - previous }
	return current
}

func trafficHistoryDeltas(previous, current map[string]trafficHistoryCounter) []TrafficHistorySample {
	result := make([]TrafficHistorySample, 0, len(current))
	for key, now := range current {
		before, ok := previous[key]; if !ok { continue }
		up, down := counterDelta(now.up, before.up), counterDelta(now.down, before.down)
		if up != 0 || down != 0 { result = append(result, TrafficHistorySample{Resource: now.resource, Tag: now.tag, Up: up, Down: down}) }
	}
	return result
}

func loadTrafficHistoryCounters() (map[string]trafficHistoryCounter, error) {
	db := database.GetDB(); current := make(map[string]trafficHistoryCounter)
	var inbounds []model.Inbound
	if err := db.Model(&model.Inbound{}).Select("tag", "up", "down").Find(&inbounds).Error; err != nil { return nil, err }
	for _, v := range inbounds { if v.Tag != "" { c:=trafficHistoryCounter{resource:"inbound",tag:v.Tag,up:v.Up,down:v.Down}; current[historyCounterKey(c.resource,c.tag)] = c } }
	var clients []xray.ClientTraffic
	if err := db.Model(&xray.ClientTraffic{}).Select("email", "up", "down").Find(&clients).Error; err != nil { return nil, err }
	for _, v := range clients { if v.Email != "" { c:=trafficHistoryCounter{resource:"client",tag:v.Email,up:v.Up,down:v.Down}; current[historyCounterKey(c.resource,c.tag)] = c } }
	var outbounds []model.OutboundTraffics
	if err := db.Model(&model.OutboundTraffics{}).Select("tag", "up", "down").Find(&outbounds).Error; err != nil { return nil, err }
	for _, v := range outbounds { if v.Tag != "" { c:=trafficHistoryCounter{resource:"outbound",tag:v.Tag,up:v.Up,down:v.Down}; current[historyCounterKey(c.resource,c.tag)] = c } }
	return current, nil
}

func SnapshotTrafficHistory(now time.Time) error {
	trafficHistorySnapshot.Lock(); defer trafficHistorySnapshot.Unlock()
	current, err := loadTrafficHistoryCounters(); if err != nil { return err }
	if !trafficHistorySnapshot.initialized {
		trafficHistorySnapshot.previous=current; trafficHistorySnapshot.initialized=true; trafficHistorySnapshot.lastPrune=now; return nil
	}
	if err := RecordTrafficHistory(trafficHistoryDeltas(trafficHistorySnapshot.previous,current), now); err != nil { return err }
	trafficHistorySnapshot.previous=current
	if now.Sub(trafficHistorySnapshot.lastPrune) >= 24*time.Hour { if err:=PruneTrafficHistory(); err!=nil{return err}; trafficHistorySnapshot.lastPrune=now }
	return nil
}

func GetTrafficHistory(resource, tag string, bucketSeconds int64, maxPoints int) ([]TrafficHistoryPoint, error) {
	resource, tag = strings.ToLower(strings.TrimSpace(resource)), strings.TrimSpace(tag)
	if !validTrafficHistoryResource(resource) || tag=="" { return nil, fmt.Errorf("invalid traffic history resource or tag") }
	if bucketSeconds < trafficHistoryBaseBucket || bucketSeconds%trafficHistoryBaseBucket != 0 { return nil, fmt.Errorf("bucket must be a multiple of %d seconds", trafficHistoryBaseBucket) }
	if maxPoints < 1 { maxPoints=360 }; if maxPoints>1000 { maxPoints=1000 }
	if err:=database.EnsureTrafficHistoryModel(); err!=nil{return nil,err}
	end:=time.Now().Unix(); start:=end-bucketSeconds*int64(maxPoints); var rows []model.TrafficHistory
	if err:=database.GetDB().Where("resource = ? AND tag = ? AND date_time >= ?",resource,tag,start).Order("date_time ASC").Find(&rows).Error; err!=nil{return nil,err}
	byTime:=make(map[int64]*TrafficHistoryPoint)
	for _,row:=range rows { t:=row.DateTime-row.DateTime%bucketSeconds; p:=byTime[t]; if p==nil{p=&TrafficHistoryPoint{T:t};byTime[t]=p}; if row.Direction=="up"{p.Up+=row.Traffic}else if row.Direction=="down"{p.Down+=row.Traffic} }
	result:=make([]TrafficHistoryPoint,0,len(byTime)); for _,p:=range byTime{result=append(result,*p)}; sort.Slice(result,func(i,j int)bool{return result[i].T<result[j].T}); return result,nil
}

func PruneTrafficHistory() error {
	if err:=database.EnsureTrafficHistoryModel();err!=nil{return err}; cutoff:=time.Now().Add(-trafficHistoryRetention).Unix(); return database.GetDB().Where("date_time < ?",cutoff).Delete(&model.TrafficHistory{}).Error
}
