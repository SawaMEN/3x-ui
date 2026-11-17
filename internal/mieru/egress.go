package mieru

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

// Each Mita instance enters the panel router under its original inbound tag.
// This range is separate from the TUIC and AmneziaWG bridge ranges.
func SOCKSPortForInbound(id int) int {
	return sidecaregress.Port(id)
}

var (
	passwordOnce sync.Once
	password     string
)

func SocksPassword() string {
	passwordOnce.Do(func() {
		var secret [24]byte
		if _, err := rand.Read(secret[:]); err != nil {
			panic(err)
		}
		password = hex.EncodeToString(secret[:])
	})
	return password
}

// An opaque internal identity cannot merge with a real client's user counter.
func SocksUsername() string { return "__panel_mieru_" + SocksPassword()[:16] }
