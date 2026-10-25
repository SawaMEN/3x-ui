package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Compatibility entry point; production injects every protocol together.
func injectSingBoxMieruEgress(cfg *singbox.Config, inbounds []*model.Inbound) error {
	var selected []*model.Inbound
	for _, ib := range inbounds {
		if ib != nil && ib.Protocol == model.Mieru {
			selected = append(selected, ib)
		}
	}
	// Keep public listeners in collision checks without creating other relays.
	for _, ib := range inbounds {
		if ib != nil && ib.Protocol != model.Mieru {
			copy := *ib
			copy.Protocol = model.HTTP
			selected = append(selected, &copy)
		}
	}
	return injectSingBoxSidecarEgress(cfg, selected)
}
