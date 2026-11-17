package model

import "encoding/json"

// Read the accidental name emitted by older builds while always publishing
// xrayState. A parent and child panel can be upgraded independently.
func (n *NodeSummary) UnmarshalJSON(data []byte) error {
	type plain NodeSummary
	var decoded struct {
		*plain
		State       *string `json:"xrayState"`
		LegacyState *string `json:"presenceRegistry"`
	}
	decoded.plain = (*plain)(n)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.State != nil {
		n.XrayState = *decoded.State
	} else if decoded.LegacyState != nil {
		n.XrayState = *decoded.LegacyState
	}
	return nil
}
