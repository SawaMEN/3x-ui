package sub

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestXhttpExtraPrecedenceAcrossSubscriptions(t *testing.T) {
	raw := map[string]any{"path": "/audit", "host": "edge.example", "mode": "packet-up", "scMaxEachPostBytes": "999999", "extra": map[string]any{"path": "/ignored", "scMaxEachPostBytes": "65536", "scMinPostsIntervalMs": "40-80", "xmux": map[string]any{"maxConcurrency": "4-8"}}}
	extra := buildXhttpExtra(raw)
	if extra["scMaxEachPostBytes"] != "65536" || extra["scMinPostsIntervalMs"] != "40-80" {
		t.Fatal(extra)
	}
	params := map[string]string{}
	applyXhttpExtraParams(raw, params)
	if params["path"] != "/audit" || params["host"] != "edge.example" {
		t.Fatal(params)
	}
	clash := buildXhttpClashOpts(raw)
	if clash["path"] != "/audit" || clash["sc-max-each-post-bytes"] != "65536" {
		t.Fatal(clash)
	}
	data, _ := json.Marshal(map[string]any{"network": "xhttp", "xhttpSettings": raw})
	svc := NewSubJsonService("", "", "", "", nil)
	stream := svc.streamData(string(data), "")
	effective := stream["xhttpSettings"].(map[string]any)
	if effective["path"] != "/audit" || effective["scMaxEachPostBytes"] != "65536" {
		t.Fatal(effective)
	}
	if _, ok := effective["extra"]; ok {
		t.Fatal("extra was not flattened")
	}
	if raw["extra"].(map[string]any)["path"] != "/ignored" {
		t.Fatal("mutated source")
	}
}

func TestXhttpJSONKeepsNumericPostLimits(t *testing.T) {
	svc := NewSubJsonService("", "", "", "", nil)
	stream := svc.streamData(`{"network":"xhttp","xhttpSettings":{"scMaxEachPostBytes":65536,"scMinPostsIntervalMs":60}}`, "")
	xhttp := stream["xhttpSettings"].(map[string]any)
	extra := buildXhttpExtra(xhttp)
	if extra["scMaxEachPostBytes"] != float64(65536) || extra["scMinPostsIntervalMs"] != float64(60) {
		t.Fatal(extra)
	}
	opts := buildXhttpClashOpts(xhttp)
	if opts["sc-max-each-post-bytes"] != "65536" || opts["sc-min-posts-interval-ms"] != "60" {
		t.Fatal(opts)
	}
	if xhttp["scMaxEachPostBytes"] != float64(65536) || xhttp["scMinPostsIntervalMs"] != float64(60) {
		t.Fatal(xhttp)
	}
}

func TestXhttpJSONNeverStacksGlobalMux(t *testing.T) {
	svc := NewSubJsonService(`{"enabled":true,"concurrency":8}`, "", "", "", nil)
	inbound := &model.Inbound{Listen: "1.2.3.4", Port: 443, Protocol: model.VLESS, Settings: `{"decryption":"none"}`, StreamSettings: `{"network":"xhttp","security":"none","xhttpSettings":{"path":"/audit","mode":"auto"}}`}
	configs := svc.getConfig(&SubService{}, inbound, model.Client{ID: "11111111-1111-4111-8111-111111111111"}, "example.test")
	if len(configs) == 0 {
		t.Fatal("no outbounds")
	}
	for _, config := range configs {
		var out map[string]any
		if err := json.Unmarshal(config, &out); err != nil {
			t.Fatal(err)
		}
		if _, exists := out["mux"]; exists {
			t.Fatal("XHTTP stacked with global Mux", out)
		}
	}
}
