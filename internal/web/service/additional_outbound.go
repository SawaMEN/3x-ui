package service

import (
	"encoding/json"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/util/json_util"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

func transformAdditionalOutbounds(cfg *xray.Config) error {
	var rows []map[string]any
	if err := json.Unmarshal(cfg.OutboundConfigs, &rows); err != nil {
		return err
	}
	desired := map[string]bool{}
	for i, row := range rows {
		if !externalvpn.IsAdditionalOutbound(row) {
			continue
		}
		tag, _ := row["tag"].(string)
		bridge, err := externalvpn.EnsureOutbound(row)
		if err != nil {
			if id, managed := nodeRouteInboundID(tag); managed {
				// A missing VetroffProxy runtime must not prevent Xray from
				// starting or leak node-routed traffic through direct.
				logger.Warningf("node route %d unavailable: %v", id, err)
				rows[i] = blockedNodeRouteOutbound(id, "xray")
				continue
			}
			return fmt.Errorf("outbound %q: %w", tag, err)
		}
		desired[tag] = true
		rows[i] = bridge
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	cfg.OutboundConfigs = json_util.RawMessage(raw)
	externalvpn.KeepOutbounds(desired)
	return nil
}
