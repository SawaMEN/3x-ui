package xray

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/util/json_util"
)

func TestPanelInboundAdaptationKeepsWireSanitizationAtXrayBoundary(t *testing.T) {
	panel := &coremodel.InboundConfig{Protocol: "vless", Settings: json_util.RawMessage(`{"encryption":"client-secret","decryption":"server-secret","fallbacks":[]}`)}
	raw, err := json.Marshal(panel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"encryption"`) {
		t.Fatal("panel representation lost client encryption")
	}
	wire, err := json.Marshal(InboundFromPanel(panel))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), `"encryption"`) || strings.Contains(string(wire), `"fallbacks"`) {
		t.Fatalf("Xray wire sanitation lost: %s", wire)
	}
	if !strings.Contains(string(panel.Settings), `"encryption"`) {
		t.Fatal("Xray adapter mutated panel settings")
	}
}
