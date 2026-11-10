package service

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestNodeStateJSONContract(t *testing.T) {
	for name, value := range map[string]any{
		"stored node":  model.Node{XrayState: "running"},
		"descendant":   model.NodeSummary{XrayState: "running"},
		"API view":     toNodeView(&model.Node{XrayState: "running"}),
		"probe result": (HeartbeatPatch{XrayState: "running"}).ToUI(true),
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["xrayState"] != "running" {
				t.Fatal("node state is missing the frontend's xrayState field")
			}
			if _, exists := fields["presenceRegistry"]; exists {
				t.Fatal("internal registry name leaked into JSON contract")
			}
		})
	}
}
