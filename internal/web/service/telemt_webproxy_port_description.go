package service

import (
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// telemtWebPortDescription reports the observed socket owner plus configured
// panel components that may compete for the port. A database record by itself
// does not prove that a process is currently listening.
func telemtWebPortDescription(port int, processOwner string) string {
	parts := make([]string, 0, 3)
	if processOwner != "" {
		parts = append(parts, "процесс: "+processOwner)
	}
	if panelPort, err := (&SettingService{}).GetPort(); err == nil && panelPort == port {
		parts = append(parts, "панель 3x-ui")
	}
	db := database.GetDB()
	if db != nil {
		var inbounds []model.Inbound
		if err := db.Where("port = ? AND enable = ? AND node_id IS NULL", port, true).Limit(4).Find(&inbounds).Error; err == nil {
			for _, inbound := range inbounds {
				parts = append(parts, fmt.Sprintf("подключение 3x-ui «%s» (%s, ID %d)", inbound.Tag, inbound.Protocol, inbound.Id))
			}
		}
	}
	if len(parts) == 0 {
		return "неизвестный процесс"
	}
	return strings.Join(parts, "; ")
}
