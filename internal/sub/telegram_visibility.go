package sub

import (
	"encoding/json"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func telegramInboundOnPage(inbound *model.Inbound, visibility service.TelegramPageVisibility) bool {
	if inbound.Protocol != model.MTProto {
		return true
	}
	parent, subscription := mtproto.Publication(inbound.Settings)
	if !subscription || (inbound.NodeID != nil && (!parent || !visibility.Remote)) || (inbound.NodeID == nil && !visibility.Local) {
		return false
	}
	var settings struct {
		WebProxy struct {
			Enabled bool `json:"enabled"`
		} `json:"webProxy"`
	}
	if json.Unmarshal([]byte(inbound.Settings), &settings) != nil {
		return false
	}
	return !settings.WebProxy.Enabled || visibility.Web
}

func telegramLinkOnPage(link string, visibility service.TelegramPageVisibility) bool {
	if strings.HasPrefix(link, "tg://proxy?") {
		return visibility.Local
	}
	if strings.HasPrefix(link, "tg://webproxy?") {
		return visibility.Local && visibility.Web
	}
	return true
}
