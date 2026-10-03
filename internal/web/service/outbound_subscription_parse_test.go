package service

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseOutboundSubscriptionBodyAcceptsArbitraryXrayProtocol(t *testing.T) {
	body := []byte(`[{"protocol":"future-proxy","tag":"future","settings":{"server":"example.com"}}]`)
	outbounds, identities, issues, err := parseOutboundSubscriptionBody(body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}
	if len(outbounds) != 1 || len(identities) != 1 {
		t.Fatalf("got %d outbounds / %d identities", len(outbounds), len(identities))
	}
	if got := outbounds[0]["protocol"]; got != "future-proxy" {
		t.Fatalf("protocol = %v", got)
	}
	if got := outbounds[0]["tag"]; got != "future" {
		t.Fatalf("tag = %v", got)
	}
}

func TestParseOutboundSubscriptionBodyAcceptsArbitrarySingBoxType(t *testing.T) {
	body := []byte(`{"type":"future-transport","tag":"native","server":"example.com","server_port":443,"custom":true}`)
	outbounds, _, issues, err := parseOutboundSubscriptionBody(body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(issues) != 0 || len(outbounds) != 1 {
		t.Fatalf("outbounds=%d issues=%v", len(outbounds), issues)
	}
	out := outbounds[0]
	if got := out["protocol"]; got != "singbox:future-transport" {
		t.Fatalf("protocol = %v", got)
	}
	settings, ok := out["settings"].(map[string]any)
	if !ok {
		t.Fatalf("settings type = %T", out["settings"])
	}
	if settings["server"] != "example.com" || settings["custom"] != true {
		t.Fatalf("settings were not preserved: %#v", settings)
	}
	if _, exists := settings["type"]; exists {
		t.Fatal("native type leaked into settings")
	}
}

func TestParseOutboundSubscriptionBodyAcceptsConfigObjectAndBase64(t *testing.T) {
	raw := `{"outbounds":[{"protocol":"vless","tag":"x","settings":{"address":"127.0.0.1","port":443,"id":"b831381d-6324-4d53-ad4f-8cda48b30811"}},{"type":"direct","tag":"native-direct"}]}`
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	outbounds, identities, issues, err := parseOutboundSubscriptionBody([]byte(encoded))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(issues) != 0 || len(outbounds) != 2 || len(identities) != 2 {
		t.Fatalf("outbounds=%d identities=%d issues=%v", len(outbounds), len(identities), issues)
	}
	if outbounds[1]["protocol"] != "singbox:direct" {
		t.Fatalf("second protocol = %v", outbounds[1]["protocol"])
	}
}

func TestParseOutboundSubscriptionBodyReportsUnsupportedLink(t *testing.T) {
	body := []byte("vless://b831381d-6324-4d53-ad4f-8cda48b30811@127.0.0.1:443?security=tls#ok\nnaive+https://user:pass@example.com#unsupported")
	outbounds, _, issues, err := parseOutboundSubscriptionBody(body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(outbounds) != 1 {
		t.Fatalf("expected supported vless profile, got %d", len(outbounds))
	}
	if len(issues) != 1 {
		t.Fatalf("expected one unsupported profile issue, got %v", issues)
	}
	if !strings.Contains(issues[0], "unsupported link scheme") {
		t.Fatalf("issue does not explain unsupported profile: %q", issues[0])
	}
}

func TestParseJSONOutboundSubscriptionReportsMalformedMembers(t *testing.T) {
	body := []byte(`[{"tag":"missing-protocol"},42]`)
	outbounds, _, issues, err := parseOutboundSubscriptionBody(body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(outbounds) != 0 || len(issues) != 2 {
		t.Fatalf("outbounds=%d issues=%v", len(outbounds), issues)
	}
	if !strings.Contains(issues[0], "protocol/type is required") {
		t.Fatalf("unexpected issue: %q", issues[0])
	}
}
