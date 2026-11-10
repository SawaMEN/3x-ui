package service

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagerRealityTargetValidation(t *testing.T) {
	cases := []struct {
		input string
		want  string
		valid bool
	}{
		{"www.example.org", "www.example.org:443", true},
		{"www.example.org:8443", "www.example.org:8443", true},
		{"WWW.EXAMPLE.ORG:443", "www.example.org:443", true},
		{"https://www.example.org", "", false},
		{"localhost", "", false},
		{"127.0.0.1:443", "", false},
		{"example.com:0", "", false},
		{"example.com:65536", "", false},
		{"example.com:not-a-port", "", false},
		{"example.com:443/path", "", false},
		{"example.com\ninjected:443", "", false},
	}
	for _, tc := range cases {
		actual, err := managerRealityTarget(tc.input)
		if (err == nil) != tc.valid || (tc.valid && actual != tc.want) {
			t.Errorf("managerRealityTarget(%q) = %q, %v; want %q, valid %v",
				tc.input, actual, err, tc.want, tc.valid)
		}
	}
}

func TestManagerEntrySNIAndCamouflageTargetAreIndependent(t *testing.T) {
	svc := &ManagerService{}
	ib := &model.Inbound{
		Protocol:       model.VLESS,
		Settings:       `{"clients":[]}`,
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"target":"before.example.org:443","serverNames":["before.example.org"]}}`,
	}
	entry := model.ManagerEntry{
		SNI:           "sni.example.org",
		RealityTarget: "camouflage.example.org:8443",
	}
	if err := svc.applyEntrySNI(1, ib, entry); err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Reality struct {
			Target      string   `json:"target"`
			Dest        string   `json:"dest"`
			ServerNames []string `json:"serverNames"`
		} `json:"realitySettings"`
	}
	if err := json.Unmarshal([]byte(ib.StreamSettings), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Reality.Target != "camouflage.example.org:8443" ||
		parsed.Reality.Dest != parsed.Reality.Target ||
		len(parsed.Reality.ServerNames) != 1 ||
		parsed.Reality.ServerNames[0] != "sni.example.org" {
		t.Fatalf("unexpected REALITY SNI or target: %+v", parsed.Reality)
	}
}

func TestManagerEntryCamouflageRequiresReality(t *testing.T) {
	svc := &ManagerService{}
	ib := &model.Inbound{
		Protocol:       model.VLESS,
		Settings:       `{"clients":[]}`,
		StreamSettings: `{"security":"tls","tlsSettings":{"serverName":"example.org"}}`,
	}
	if err := svc.applyEntrySNI(1, ib, model.ManagerEntry{RealityTarget: "example.org"}); err == nil {
		t.Fatal("non-REALITY camouflage target must be rejected")
	}
}
