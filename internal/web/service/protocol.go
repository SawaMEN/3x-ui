package service

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
)

var inboundCoreSwitcher func(context.Context, string) error

// SetInboundCoreSwitcher wires the application-level core switch used when a
// sing-box-only inbound is enabled while Xray is selected. The web layer owns
// the actual process transition because it also owns both core services.
func SetInboundCoreSwitcher(fn func(context.Context, string) error) {
	inboundCoreSwitcher = fn
}

func isSingBoxOnlyInboundProtocol(protocol model.Protocol) bool {
	switch protocol {
	case model.NaiveProxy, model.AnyTLS, model.ShadowTLS:
		return true
	default:
		return false
	}
}

func inboundRuntimeProtocolError(protocol model.Protocol) error {
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

func validateInboundRuntimeProtocol(protocol model.Protocol, existing *model.Inbound) error {
	if !isSingBoxOnlyInboundProtocol(protocol) {
		return nil
	}
	core, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	if core == CoreTypeSingBox {
		return nil
	}
	return inboundRuntimeProtocolError(protocol)
}

func ensureInboundRuntimeProtocol(protocol model.Protocol) error {
	if !isSingBoxOnlyInboundProtocol(protocol) {
		return nil
	}
	core, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	if core == CoreTypeSingBox {
		return nil
	}
	if inboundCoreSwitcher == nil {
		return inboundRuntimeProtocolError(protocol)
	}
	return inboundCoreSwitcher(context.Background(), CoreTypeSingBox)
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
