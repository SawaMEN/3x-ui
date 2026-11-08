package service

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// Export the actual policy over every protocol accepted by the API model.
// This is inventory evidence, not a substitute for the binary traffic gates.
func TestStage5CapabilityInventory(t *testing.T) {
	field, ok := reflect.TypeOf(model.Inbound{}).FieldByName("Protocol")
	if !ok {
		t.Fatal("missing protocol field")
	}
	var protocols []string
	for _, rule := range strings.Split(field.Tag.Get("validate"), ",") {
		if strings.HasPrefix(rule, "oneof=") {
			protocols = strings.Fields(strings.TrimPrefix(rule, "oneof="))
		}
	}
	if len(protocols) == 0 {
		t.Fatal("missing model protocol catalog")
	}
	type row struct {
		Core     string `json:"core"`
		Protocol string `json:"protocol"`
		Allowed  bool   `json:"policy_allowed"`
	}
	var rows []row
	for _, core := range []string{CoreTypeXray, CoreTypeSingBox, CoreTypeSingBoxUpstream} {
		for _, p := range protocols {
			rows = append(rows, row{core, p, CoreSupportsInboundProtocol(core, model.Protocol(p))})
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("STAGE5_CAPABILITY_OUTPUT"); path != "" {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("catalog: %d protocol models, %d core-policy pairs", len(protocols), len(rows))
}
