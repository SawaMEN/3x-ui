package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	xraygeodata "github.com/xtls/xray-core/common/geodata"
	"google.golang.org/protobuf/proto"
)

func TestNativeGeoDataReferencesSurviveTemplateAndExpandAtRuntime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XRAY_LOCATION_ASSET", dir)
	site, err := proto.Marshal(&xraygeodata.GeoSiteList{Entry: []*xraygeodata.GeoSite{{Code: "ru-available-only-inside", Domain: []*xraygeodata.Domain{{Type: xraygeodata.Domain_Domain, Value: "rzd.ru"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	ip, err := proto.Marshal(&xraygeodata.GeoIPList{Entry: []*xraygeodata.GeoIP{{Code: "ru", Cidr: []*xraygeodata.CIDR{{Ip: []byte{203, 0, 113, 0}, Prefix: 24}}}}})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"geosite_RU.dat": site, "geoip.dat": ip} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	template := `{"route":{"rules":[{"type":"logical","mode":"and","action":"route","outbound":"vpn","rules":[{"routingGeoDomains":["ext:geosite_RU.dat:ru-available-only-inside"],"domain_suffix":["manual.example"]},{"routingGeoIPs":["geoip:ru"],"routingGeoSourceIPs":["geoip:private"],"ip_cidr":["198.51.100.0/24"]}]}]}}`
	generated, err := materializeNativeRoutingLists(template, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(generated, "routingGeo") || !strings.Contains(generated, "rzd.ru") || !strings.Contains(generated, "manual.example") || !strings.Contains(generated, "203.0.113.0/24") || !strings.Contains(generated, "198.51.100.0/24") || !strings.Contains(generated, `"source_ip_is_private":true`) {
		t.Fatalf("matchers lost: %s", generated)
	}

	setupSettingTestDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	if err := (&SettingService{}).SetSingBoxConfigTemplate(template); err != nil {
		t.Fatal(err)
	}
	seedXrayTemplate(t, `{"outbounds":[{"protocol":"freedom","tag":"vpn"}],"routing":{"rules":[{"domain":["ext:missing.dat:unused-legacy"],"outboundTag":"vpn"}]}}`)
	snapshot, err := loadNativeConfigSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	native := NativeCore(CoreTypeHiddify)
	runtime, err := native.generateNativeConfig(snapshot)
	if err != nil {
		t.Fatalf("unused legacy route blocked native config: %v", err)
	}
	runtimeJSON, err := runtime.Marshal()
	if err != nil || strings.Contains(string(runtimeJSON), "routingGeo") || !strings.Contains(string(runtimeJSON), "rzd.ru") {
		t.Fatalf("invalid native runtime: %s %v", runtimeJSON, err)
	}
	editor, err := native.generateNativeConfig(snapshot, true)
	if err != nil {
		t.Fatal(err)
	}
	source, err := json.Marshal(editor.Route)
	if err != nil || !strings.Contains(string(source), "routingGeoDomains") {
		t.Fatalf("editor froze references: %s %v", source, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(generated), &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(template, "routingGeoDomains") {
		t.Fatal("template references were changed")
	}
	for _, broken := range []string{
		`{"route":{"rules":[{"routingGeoDomains":["   "],"outbound":"vpn"}]}}`,
		`{"route":{"rules":[{"routingGeoDomains":["ext:missing.dat:ru"],"outbound":"vpn"}]}}`,
		`{"route":{"rules":[{"routingGeoIPs":[42],"outbound":"vpn"}]}}`,
		`{"route":{"rules":[{"type":"logical","routingGeoDomains":["ext:geosite_RU.dat:ru-available-only-inside"]}]}}`,
	} {
		if _, err := materializeNativeRoutingLists(broken, nil); err == nil {
			t.Fatalf("invalid reference became catch-all: %s", broken)
		}
	}
	disabled := `{"route":{"rules":[{"enabled":false,"routingGeoDomains":["ext:missing.dat:ru"]}]}}`
	if _, err := materializeNativeRoutingLists(disabled, nil); err != nil {
		t.Fatalf("disabled reference loaded: %v", err)
	}
}
