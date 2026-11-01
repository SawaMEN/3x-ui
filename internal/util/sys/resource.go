package sys

import (
	"os"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
)

const lowResourceMemoryThreshold = 2 << 30 // 2 GiB

// IsLowResourceMode returns true when explicitly enabled through
// XUI_LOW_RESOURCE or automatically on small VDS hosts (<= 2 GiB RAM).
// Explicit false/off values disable the automatic profile.
func IsLowResourceMode() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("XUI_LOW_RESOURCE")))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}

	info, err := mem.VirtualMemory()
	return err == nil && info != nil && info.Total > 0 && info.Total <= lowResourceMemoryThreshold
}
