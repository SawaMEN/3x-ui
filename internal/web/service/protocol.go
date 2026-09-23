package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func validateInboundRuntimeProtocol(protocol model.Protocol) error {
	if protocol != model.NaiveProxy {
		return nil
	}
	core, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	if core != CoreTypeSingBox {
		return common.NewErrorf("NaïveProxy requires sing-box as the selected core")
	}
	return nil
}

func isXrayManagedProtocol(protocol model.Protocol) bool {
	return protocol != model.VKTurnProxy &&
		protocol != model.NaiveProxy &&
		protocol != model.MTProto &&
		protocol != model.AmneziaWG &&
		protocol != model.TUIC &&
		protocol != model.Psiphon &&
		protocol != model.Mieru
}

func isVKTurnProxyProtocol(protocol model.Protocol) bool {
	return protocol == model.VKTurnProxy
}
