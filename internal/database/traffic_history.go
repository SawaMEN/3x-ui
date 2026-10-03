package database

import (
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

var trafficHistoryMigrateOnce sync.Once
var trafficHistoryMigrateErr error

// EnsureTrafficHistoryModel creates the traffic history table once per process.
func EnsureTrafficHistoryModel() error {
	trafficHistoryMigrateOnce.Do(func() {
		trafficHistoryMigrateErr = GetDB().AutoMigrate(&model.TrafficHistory{})
	})
	return trafficHistoryMigrateErr
}
