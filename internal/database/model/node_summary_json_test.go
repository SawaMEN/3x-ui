package model

import (
	"encoding/json"
	"testing"
)

func TestNodeSummaryStateCompatibility(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"guid":"edge","xrayState":"running"}`, "running"},
		{`{"guid":"edge","presenceRegistry":"stop"}`, "stop"},
		{`{"guid":"edge","xrayState":"running","presenceRegistry":"stop"}`, "running"},
		{`{"guid":"edge","xrayState":"","presenceRegistry":"stop"}`, ""},
	} {
		var node NodeSummary
		if err := json.Unmarshal([]byte(tc.raw), &node); err != nil {
			t.Fatal(err)
		}
		if node.XrayState != tc.want || node.Guid != "edge" {
			t.Fatal("node summary state/identity lost")
		}
		data, err := json.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		if fields["xrayState"] != tc.want {
			t.Fatal("canonical state field missing")
		}
		if _, exists := fields["presenceRegistry"]; exists {
			t.Fatal("legacy field emitted")
		}
	}
}
