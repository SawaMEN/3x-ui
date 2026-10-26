package tgbot

// helpers.go holds the small shared primitives the screen layer needs, so the
// router never grows a second copy of them.
import (
	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// xrayProcessOrNil returns the running Xray process, or nil; a panel whose Xray
// is down is a normal state the screens must render, not an error.
func xrayProcessOrNil() *xray.Presence {
	process := service.OnlinePresence()
	if process == nil {
		return nil
	}
	return process
}

// panelVersion is the version string the menu shows.
func panelVersion() string { return config.GetPanelVersion() }
