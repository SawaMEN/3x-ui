package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
)

func validateInboundRuntimeProtocol(protocol model.Protocol, existing *model.Inbound) error {
	if protocol != model.NaiveProxy && protocol != model.AnyTLS && protocol != model.ShadowTLS {
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
	switch protocol {
	case model.NaiveProxy:
		return common.NewErrorf("NaïveProxy requires sing-box as the selected core")
	case model.AnyTLS:
		return common.NewErrorf("AnyTLS requires sing-box as the selected core")
	case model.ShadowTLS:
		return common.NewErrorf("ShadowTLS requires sing-box as the selected core")
	default:
		return common.NewErrorf("%s requires sing-box as the selected core", protocol)
	}
}

func isXrayManagedProtocol(protocol model.Protocol) bool {
	return protocol != model.VKTurnProxy &&
		protocol != model.NaiveProxy &&
		protocol != model.AnyTLS &&
		protocol != model.ShadowTLS &&
		protocol != model.MTProto &&
		protocol != model.AmneziaWG &&
		protocol != model.TUIC &&
		protocol != model.Mieru &&
		protocol != model.Sudoku
}

func isVKTurnProxyProtocol(protocol model.Protocol) bool {
	return protocol == model.VKTurnProxy
}
