package gateway

import (
	"fmt"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// The panel has already initialized its connection pool. Reinitializing it on
// every status poll leaks pools and runs database migrations during requests.
// CLI operations still initialize the database once when necessary.
var databaseInitMu sync.Mutex

func ensureDatabase() error {
	databaseInitMu.Lock()
	defer databaseInitMu.Unlock()
	if database.GetDB() != nil {
		return nil
	}
	return database.InitDB(config.GetDBPath())
}

func validateManagedGatewayConflicts() error {
	var count int64
	if err := database.GetDB().Model(&model.Inbound{}).Where("enable = ? AND node_id IS NULL AND (port = ? OR tag = ?)", true, inboundPort, inboundTag).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("a local panel inbound already uses Gateway port %d or tag %q", inboundPort, inboundTag)
	}
	return nil
}
