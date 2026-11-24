package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// telemtWebPort443Inspection is shared by status, preflight and deployment.
// A running frontend is not proof it owns the socket: only its actual
// listeners may be reused. Configured panel listeners are checked even if
// their process is temporarily stopped.
type telemtWebPort443Inspection struct {
	Free      bool
	Available bool
	Owner     string
}

func inspectTelemtWebPort443(state TelemtWebProxyState, requestedFrontend string, editingInboundID int) (telemtWebPort443Inspection, error) {
	owner := strings.ToLower(telemtWebPortOwner(443))
	result := telemtWebPort443Inspection{
		Free:  telemtWebPortAvailable(443),
		Owner: owner,
	}

	panelOwner, err := telemtWebPanelPort443Owner(state, editingInboundID)
	if err != nil {
		return result, fmt.Errorf("проверка порта 443 в настройках панели: %w", err)
	}
	if panelOwner != "" {
		result.Owner = panelOwner
		return result, nil
	}

	previousOwned := state.Enabled && telemtIngressEntryActive(state) &&
		telemtWebPortOwnedOnlyBy(owner, telemtIngressPublicOwner(state))
	nginxShared := requestedFrontend == "nginx" && telemtWebNginxOwnsPort443() &&
		telemtWebPortOwnedOnlyBy(owner, "nginx")
	result.Available = result.Free || previousOwned || nginxShared
	return result, nil
}

func telemtWebPanelPort443Owner(state TelemtWebProxyState, editingInboundID int) (string, error) {
	settings := &SettingService{}
	panelPort, err := settings.GetPort()
	if err != nil {
		return "", err
	}
	// The runtime listener honours XUI_PORT before the persisted webPort.
	// Use its effective value when checking whether Telemt can own TCP 443.
	if override, configured, overrideErr := config.GetPortOverride(); configured && overrideErr == nil {
		if override == 443 {
			return "веб-панель 3x-ui (XUI_PORT=443 в окружении службы)", nil
		}
		panelPort = override
	}
	if panelPort == 443 {
		return "веб-панель 3x-ui (Настройки → Панель, TCP 443)", nil
	}
	subEnabled, err := settings.GetSubEnable()
	if err != nil {
		return "", err
	}
	if subEnabled {
		subPort, err := settings.GetSubPort()
		if err != nil {
			return "", err
		}
		if subPort == 443 {
			return "сервер подписок 3x-ui (Настройки → Подписки, TCP 443)", nil
		}
	}

	nativePort, err := telemtWebNativePort()
	if err != nil {
		return "", err
	}
	if nativePort == 443 && state.ManagedInboundID == 0 {
		return "обычный Telemt MTProto (server.port = 443 в telemt.toml)", nil
	}
	if reservedAPIPort() == 443 {
		return "внутренний Xray API панели (127.0.0.1:443)", nil
	}

	gateway, err := settings.GetRuntimeDomainFrontConfig()
	if err != nil {
		return "", err
	}
	if gateway.HasPublicSites() {
		if gateway.HTTPSPort == 443 {
			return "Domain Front HTTPS (Настройки → Домены, TCP 443)", nil
		}
		if gateway.HTTPPort == 443 {
			return "Domain Front HTTP (Настройки → Домены, TCP 443)", nil
		}
	}

	excludedInboundID := state.ManagedInboundID
	if editingInboundID > 0 {
		excludedInboundID = editingInboundID
	}
	return telemtWebInboundPort443Owner(excludedInboundID)
}

func telemtWebInboundPort443Owner(excludedInboundID int) (string, error) {
	db := database.GetDB()
	if db == nil {
		return "", fmt.Errorf("база подключений недоступна")
	}
	var inbounds []model.Inbound
	if err := db.Where("node_id IS NULL").Find(&inbounds).Error; err != nil {
		return "", err
	}
	for _, inbound := range inbounds {
		// The edited local MTProto inbound (or the current managed inbound
		// outside the editor) may reuse its own reservation.
		if excludedInboundID > 0 && inbound.Id == excludedInboundID && inbound.Protocol == model.MTProto {
			continue
		}
		if port, _ := spansOverlap(inboundListenerSpans(&inbound), []listenerSpan{{443, 443, transportTCP}}); port == 0 {
			continue
		}
		name := strings.TrimSpace(inbound.Remark)
		if name == "" {
			name = strings.TrimSpace(inbound.Tag)
		}
		if name == "" {
			name = fmt.Sprintf("#%d", inbound.Id)
		}
		status := ""
		if !inbound.Enable {
			status = ", отключено"
		}
		return fmt.Sprintf("подключение «%s» (%s, ID %d%s) в разделе «Подключения»", name, inbound.Protocol, inbound.Id, status), nil
	}
	return "", nil
}

func telemtInboundUsesUDP443(inbound *model.Inbound) bool {
	port, _ := spansOverlap(inboundListenerSpans(inbound), []listenerSpan{{443, 443, transportUDP}})
	if port != 0 {
		return true
	}
	// VetrOFF HTTP/3 and split TrustTunnel may own a separate UDP bind.
	if h3 := independentUDPListener(inbound); h3 != nil {
		return h3.Port == 443
	}
	return false
}

type telemtUDP443Info struct {
	Observed     bool     `json:"observed"`
	Busy         bool     `json:"busy"`
	Owner        string   `json:"owner,omitempty"`
	Reservations []string `json:"reservations"`
}

// UDP is independent of the HTTPS TCP listener. Never claim that UDP is free
// when socket inspection fails, and never move or stop an existing QUIC service.
func inspectTelemtUDP443(ctx context.Context) telemtUDP443Info {
	info := telemtUDP443Info{Reservations: []string{}}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "ss", "-lunpH", "sport = :443").CombinedOutput(); err == nil {
		info.Observed = true
		info.Busy = strings.TrimSpace(string(out)) != ""
		info.Owner = parseTelemtWebPortOwners(string(out))
	}
	if db := database.GetDB(); db != nil {
		var inbounds []model.Inbound
		if err := db.WithContext(ctx).Where("node_id IS NULL").Select("id", "remark", "tag", "protocol", "port", "listen", "enable", "settings", "stream_settings").Find(&inbounds).Error; err == nil {
			for _, inbound := range inbounds {
				if telemtInboundUsesUDP443(&inbound) {
					label := strings.TrimSpace(inbound.Remark)
					if label == "" {
						label = inbound.Tag
					}
					if label == "" {
						label = fmt.Sprintf("#%d", inbound.Id)
					}
					if !inbound.Enable {
						label += " (disabled)"
					}
					info.Reservations = append(info.Reservations, fmt.Sprintf("%s (%s)", label, inbound.Protocol))
				}
			}
		}
	}
	return info
}
