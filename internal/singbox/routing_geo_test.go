package singbox

import (
	"encoding/json"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	xraygeodata "github.com/xtls/xray-core/common/geodata"
	"google.golang.org/protobuf/proto"

	"github.com/SawaMEN/3x-ui/v3/internal/xray/geodata"
)

func TestTranslateXrayRoutingExternalGeoData(t *testing.T) {
	dir := t.TempDir()
	prefix := netip.MustParsePrefix("203.0.113.0/24")
	ipData, err := proto.Marshal(&xraygeodata.GeoIPList{Entry: []*xraygeodata.GeoIP{{Code: "RU", Cidr: []*xraygeodata.CIDR{{Ip: prefix.Addr().AsSlice(), Prefix: 24}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geoip_RU.dat"), ipData, 0o600); err != nil {
		t.Fatal(err)
	}
	siteData, err := proto.Marshal(&xraygeodata.GeoSiteList{Entry: []*xraygeodata.GeoSite{{Code: "RU", Domain: []*xraygeodata.Domain{{Type: xraygeodata.Domain_Domain, Value: "example.ru"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geosite_RU.dat"), siteData, 0o600); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"rules": []any{map[string]any{
		"ip":          []any{"ext:geoip_RU.dat:ru", "geoip:private"},
		"domain":      []any{"ext:geosite_RU.dat:ru"},
		"outboundTag": "direct",
	}}}
	got, err := TranslateXrayRoutingWithGeoData(raw, geodata.NewStore(dir))
	if err != nil {
		t.Fatal(err)
	}
	rules := got["rules"].([]map[string]any)
	if len(rules) != 1 || rules[0]["type"] != "logical" {
		t.Fatalf("rules = %#v", rules)
	}
	children := rules[0]["rules"].([]map[string]any)
	if !reflect.DeepEqual(children[0]["domain_suffix"], []string{"example.ru"}) ||
		!reflect.DeepEqual(children[1]["ip_cidr"], []string{"203.0.113.0/24"}) || children[1]["ip_is_private"] != true {
		t.Fatalf("converted geodata = %#v", children)
	}
}

func TestTranslateXrayRoutingMissingGeoDataFailsClosed(t *testing.T) {
	raw := map[string]any{"rules": []any{map[string]any{"ip": []any{"ext:geoip_RU.dat:ru"}, "outboundTag": "direct"}}}
	_, err := TranslateXrayRoutingWithGeoData(raw, geodata.NewStore(t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "geoip_RU.dat") {
		t.Fatalf("error = %v, want missing file diagnostic", err)
	}
}

func TestRoutingGeoAttributeFilters(t *testing.T) {
	dir := t.TempDir()
	data, err := proto.Marshal(&xraygeodata.GeoSiteList{Entry: []*xraygeodata.GeoSite{{Code: "test", Domain: []*xraygeodata.Domain{
		{Type: xraygeodata.Domain_Domain, Value: "selected.example", Attribute: []*xraygeodata.Domain_Attribute{{Key: "ads"}, {Key: "cn"}}},
		{Type: xraygeodata.Domain_Domain, Value: "excluded.example", Attribute: []*xraygeodata.Domain_Attribute{{Key: "ads"}}},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geosite.dat"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"geosite:test@ads@cn", "ext-domain:geosite.dat:test@ads@cn"} {
		entries, err := routingGeoEntries(geodata.NewStore(dir), token, geodata.KindSite)
		if err != nil || len(entries) != 1 || entries[0].Value != "selected.example" {
			t.Fatalf("filter %s: %+v %v", token, entries, err)
		}
	}
}

func TestGeoRoutingModifiedSingBoxCheck(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY")
	}
	dir := t.TempDir()
	data, err := proto.Marshal(&xraygeodata.GeoIPList{Entry: []*xraygeodata.GeoIP{{Code: "test", Cidr: []*xraygeodata.CIDR{{Ip: []byte{203, 0, 113, 0}, Prefix: 24}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geoip.dat"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	route, err := TranslateXrayRoutingWithGeoData(map[string]any{"rules": []any{map[string]any{"ip": []any{"geoip:test"}, "outboundTag": "direct"}}}, geodata.NewStore(dir))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"route": route, "outbounds": []map[string]any{{"type": "direct", "tag": "direct"}}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
		t.Fatalf("modified Sing-box rejected converted geodata: %v: %s", err, output)
	}
}
