package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func validateInboundRuntimeProtocol(protocol model.Protocol, existing *model.Inbound) error {
	if protocol != model.NaiveProxy {
		return nil
	}
	core, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	if core == CoreTypeSingBox {
		return nil
	}
	// NaïveProxy is not an Xray-managed protocol. Keeping an existing Naïve
	// row while Xray is selected makes it look enabled in the UI while the
	// Xray config silently omits it.
	return common.NewErrorf("NaïveProxy requires sing-box as the selected core")
}

func isXrayManagedProtocol(protocol model.Protocol) bool {
	return protocol != model.VKTurnProxy &&
		protocol != model.NaiveProxy &&
		protocol != model.MTProto &&
		protocol != model.AmneziaWG &&
		protocol != model.TUIC &&
		protocol != model.Mieru
}

func isVKTurnProxyProtocol(protocol model.Protocol) bool {
	return protocol == model.VKTurnProxy
}
