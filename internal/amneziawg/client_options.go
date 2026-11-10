package amneziawg

import (
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// ClientServerSettings returns a copy for export; server handshake parameters
// and live device configuration are never changed by a client override.
func ClientServerSettings(server ServerSettings, options *model.AWGClientOptions) (ServerSettings, error) {
	if options == nil {
		return server, nil
	}
	for _, f := range []struct {
		p   *int
		dst *int
	}{
		{options.Jc, &server.Jc}, {options.Jmin, &server.Jmin}, {options.Jmax, &server.Jmax},
	} {
		if f.p != nil {
			*f.dst = *f.p
		}
	}
	for _, f := range []struct {
		p   *string
		dst *string
	}{
		{options.I1, &server.I1}, {options.I2, &server.I2}, {options.I3, &server.I3}, {options.I4, &server.I4}, {options.I5, &server.I5},
	} {
		if f.p != nil {
			if err := ValidateConfigValue("AWG client signature", *f.p); err != nil {
				return server, err
			}
			*f.dst = *f.p
		}
	}
	if options.MTU != nil {
		if *options.MTU < 1280 || *options.MTU > 1500 {
			return server, fmt.Errorf("AWG client MTU must be 1280–1500")
		}
		server.MTU = *options.MTU
	}
	if err := ValidateObfuscation(server.Obfuscation()); err != nil {
		return server, fmt.Errorf("AWG client options: %w", err)
	}
	return server, nil
}
