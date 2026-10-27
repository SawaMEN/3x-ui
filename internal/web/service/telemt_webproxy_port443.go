package service

import (
	"fmt"
	"strings"

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

func inspectTelemtWebPort443(state TelemtWebProxyState, requestedFrontend string) (telemtWebPort443Inspection, error) {
	owner := strings.ToLower(telemtWebPortOwner(443))
	result := telemtWebPort443Inspection{
		Free:  telemtWebPortAvailable(443),
		Owner: owner,
	}

	panelOwner, err := telemtWebPanelPort443Owner(state)
	if err != nil {
		return result, fmt.Errorf("проверка порта 443 в настройках панели: %w", err)
	}
	if panelOwner != "" {
		result.Owner = panelOwner
		return result, nil
	}

	currentFrontend := telemtWebFrontend(state)
	previousOwned := state.Enabled && telemtWebFrontendActive(state) &&
		((currentFrontend == "nginx" && telemtWebNginxOwnsPort443() && telemtWebPortOwnedOnlyBy(owner, "nginx")) ||
			(currentFrontend != "nginx" && telemtWebPortOwnedOnlyBy(owner, currentFrontend)))
	nginxShared := requestedFrontend == "nginx" && telemtWebNginxOwnsPort443() &&
		telemtWebPortOwnedOnlyBy(owner, "nginx")
	result.Available = result.Free || previousOwned || nginxShared
	return result, nil
}

func telemtWebPanelPort443Owner(state TelemtWebProxyState) (string, error) {
	settings := &SettingService{}
	panelPort, err := settings.GetPort()
	if err != nil {
		return "", err
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

	db := database.GetDB()
	if db == nil {
		return "", fmt.Errorf("база подключений недоступна")
	}
	var inbounds []model.Inbound
	if err := db.Where("node_id IS NULL AND enable = ?", true).Find(&inbounds).Error; err != nil {
		return "", err
	}
	for _, inbound := range inbounds {
		// A managed MTProto WEB inbound describes this very frontend,
		// not a second socket that would conflict with it.
		if inbound.Id == state.ManagedInboundID && state.ManagedInboundID != 0 {
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
		return fmt.Sprintf("подключение «%s» (%s, ID %d) в разделе «Подключения»", name, inbound.Protocol, inbound.Id), nil
	}
	return "", nil
}
