package service

import (
	"context"
	"reflect"
	"testing"
)

func TestStage7ObservedFirewallPorts(t *testing.T) {
	text := `{"nftables":[{"rule":{"family":"inet","table":"xui_firewall","chain":"input","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":2222}},{"accept":null}]}},{"rule":{"family":"inet","table":"xui_firewall","chain":"input","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"udp","field":"dport"}},"right":{"range":[1000,1100]}}},{"accept":null}]}},{"rule":{"family":"inet","table":"admin","chain":"input","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":443}},{"accept":null}]}},{"rule":{"family":"inet","table":"xui_firewall","chain":"input","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":"10.0.0.1"}},{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":8443}},{"accept":null}]}}]}`
	want := map[string]bool{"2222/tcp": true, "1000-1100/udp": true}
	got, err := observedNftPorts(text)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
	got, err = observedIPTablesPorts(`-N XUI-FIREWALL
-A XUI-FIREWALL -p tcp -m tcp --dport 2222 -j ACCEPT
-A XUI-FIREWALL -p udp -m udp --dport 1000:1100 -j ACCEPT
-A XUI-FIREWALL -s 10.0.0.1 -p tcp --dport 8443 -j ACCEPT
-A ADMIN -p tcp --dport 443 -j ACCEPT
-A XUI-FIREWALL -p tcp --dport 1234 -j DROP
`)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
}

func TestStage7NativeStatusDoesNotInventOpenPorts(t *testing.T) {
	setupBulkDB(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	json := `{"nftables":[{"rule":{"family":"inet","table":"xui_firewall","chain":"input","handle":1,"expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":2222}},{"accept":null}]}}]}`
	fakeFirewallBinary(t, dir, "nft", "if [ \"$1\" = -j ]; then echo '"+json+"'; else echo 'table inet xui_firewall'; fi")
	if err := saveFirewallJSON(firewallAutoSyncKey, false); err != nil {
		t.Fatal(err)
	}
	if err := saveFirewallJSON(firewallManualRulesKey, []FirewallManualRule{{Port: 2222, Protocol: "tcp"}, {Port: 443, Protocol: "tcp"}}); err != nil {
		t.Fatal(err)
	}
	s := &FirewallService{}
	status, err := s.GetManagedStatusSafe(context.Background(), 0)
	if err != nil || !status.Enabled || status.InspectionError != "" {
		t.Fatal(status, err)
	}
	seen := map[int]bool{}
	for _, r := range status.Rules {
		if r.Source == "manual" {
			seen[r.Port] = r.Exists
		}
	}
	if !seen[2222] || seen[443] || len(seen) != 2 {
		t.Fatal("invented open port", seen)
	}
}
