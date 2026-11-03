package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestStage7NftReplacementFailurePreservesFirewall(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "deleted")
	t.Setenv("STAGE7_NFT_EVENTS", events)
	fakeFirewallBinary(t, dir, "nft", `case "$1" in
-j) echo '{"nftables":[]}' ;;
list) echo 'table inet xui_firewall' ;;
delete) echo deleted > "$STAGE7_NFT_EVENTS" ;;
-c|-f) cat > /dev/null; exit 1 ;;
esac`)
	if err := applyManagedNftables(context.Background(), filepath.Join(dir, "nft"), nil); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	if _, err := os.Stat(events); !os.IsNotExist(err) {
		t.Fatal("old firewall deleted before failed replacement", err)
	}
}

func TestStage7DeletedInboundCannotBroadenPolicy(t *testing.T) {
	for _, matcher := range []map[string]any{{"domain": []string{"example.com"}}, {"source": []string{"10.0.0.0/8"}}, {"routingListId": 1}} {
		rule := map[string]any{"inboundTag": []string{"deleted"}, "outboundTag": "direct"}
		for k, v := range matcher {
			rule[k] = v
		}
		rules, changed := removeInboundTagFromRules([]map[string]any{rule}, "deleted")
		if !changed || len(rules) != 0 {
			t.Fatalf("removed scope became a policy for other inbounds: %v", rules)
		}
	}
}

func TestStage7FirewallInterval(t *testing.T) {
	setupSettingTestDB(t)
	if got := firewallSyncInterval(); got != time.Minute {
		t.Fatal("unsafe default reconciliation interval", got)
	}
	settings := &SettingService{}
	for _, value := range []int{5, 60, 120, 3600} {
		if err := ValidateFirewallSyncInterval(value); err != nil {
			t.Fatal(err)
		}
		if err := settings.setInt(firewallSyncIntervalKey, value); err != nil {
			t.Fatal(err)
		}
		if got := firewallSyncInterval(); got != time.Duration(value)*time.Second {
			t.Fatal("interval not persisted", got)
		}
	}
	for _, value := range []int{-1, 0, 4, 3601} {
		if _, err := (&FirewallService{}).SetManagedAutoSyncPreferenceSafe(context.Background(), true, 0, value); err == nil {
			t.Fatal("invalid interval accepted", value)
		}
		if firewallSyncInterval() != time.Hour {
			t.Fatal("invalid edit changed the stored interval")
		}
	}
	if err := settings.saveSetting(firewallSyncIntervalKey, "broken"); err != nil {
		t.Fatal(err)
	}
	if firewallSyncInterval() != time.Minute {
		t.Fatal("invalid stored interval can create a busy loop")
	}
}

func TestStage7CatalogDoesNotEraseInvalidRules(t *testing.T) {
	template := `{"route":{"rules":[null,{"routingListId":1,"action":"route","outbound":"vpn"}]}}`
	if _, err := materializeNativeRoutingLists(template, []model.RoutingList{{Id: 1, Entries: `{"ips":["198.18.0.0/15"]}`}}); err == nil {
		t.Fatal("catalog expansion silently erased malformed routing policy")
	}
}
