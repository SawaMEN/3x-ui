package controller

import "github.com/SawaMEN/3x-ui/v3/internal/web/service"

// markSelectedCoreNeedRestart routes a config-change notification to the
// core that this panel currently runs. This is especially important on a
// node: master sync writes local inbounds/clients, so marking Xray while
// the node actually runs sing-box leaves the live sing-box config stale.
func markSelectedCoreNeedRestart(settings *service.SettingService, xrayService *service.XrayService, singBoxService *service.SingBoxService) {
	coreType, err := settings.GetCoreType()
	if err == nil && coreType == service.CoreTypeSingBox {
		singBoxService.SetToNeedRestart()
		return
	}
	xrayService.SetToNeedRestart()
}
