package service

import "github.com/SawaMEN/3x-ui/v3/internal/database/model"

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
