package naiveproxy

import "encoding/json"

// UnmarshalJSON keeps legacy/imported Naive clients compatible with the panel:
// an absent enable flag means enabled, while an explicit false stays disabled.
func (s *inboundSettings) UnmarshalJSON(data []byte) error {
	type plain inboundSettings
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var raw struct {
		Clients []json.RawMessage `json:"clients"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for i, item := range raw.Clients {
		if i >= len(decoded.Clients) {
			break
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			continue
		}
		if _, exists := fields["enable"]; !exists {
			decoded.Clients[i].Enable = true
		}
	}
	*s = inboundSettings(decoded)
	return nil
}
