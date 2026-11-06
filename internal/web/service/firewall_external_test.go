package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStage7ExternalFirewallRules(t *testing.T) {
	t.Run("nft stable fingerprint and exact deletion", func(t *testing.T) {
		dir := t.TempDir()
		state := filepath.Join(dir, "state")
		deleted := filepath.Join(dir, "deleted")
		doc := `{"nftables":[{"rule":{"family":"inet","table":"admin","chain":"input","handle":8,"expr":[{"counter":{"packets":1,"bytes":32}},{"drop":null}]}},{"rule":{"family":"inet","table":"xui_firewall","chain":"input","handle":2,"expr":[{"accept":null}]}}]}`
		if err := os.WriteFile(state, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("STAGE7_EXTERNAL_STATE", state)
		t.Setenv("STAGE7_EXTERNAL_DELETED", deleted)
		fakeFirewallBinary(t, dir, "nft", `if [ "$1" = -j ]; then cat "$STAGE7_EXTERNAL_STATE"; else printf '%s\n' "$@" > "$STAGE7_EXTERNAL_DELETED"; fi`)
		b := firewallBackend{name: "nftables", binary: filepath.Join(dir, "nft")}
		rules, err := listExternalFirewallRules(context.Background(), b)
		if err != nil || len(rules) != 1 {
			t.Fatal(rules, err)
		}
		id := rules[0].ID
		changed := strings.ReplaceAll(doc, `"packets":1`, `"packets":100`)
		if err := os.WriteFile(state, []byte(changed), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := deleteExternalFirewallRule(context.Background(), b, id); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(deleted)
		if string(got) != "delete\nrule\ninet\nadmin\ninput\nhandle\n8\n" {
			t.Fatal(string(got))
		}
		_ = os.Remove(deleted)
		changed = strings.ReplaceAll(changed, `"drop":null`, `"accept":null`)
		if err := os.WriteFile(state, []byte(changed), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := deleteExternalFirewallRule(context.Background(), b, id); err == nil {
			t.Fatal("deleted changed rule")
		}
		if _, err := os.Stat(deleted); !os.IsNotExist(err) {
			t.Fatal("stale selection mutated kernel")
		}
	})
	t.Run("iptables preserves quoted literal and ignores owned rules", func(t *testing.T) {
		dir := t.TempDir()
		deleted := filepath.Join(dir, "deleted")
		t.Setenv("STAGE7_EXTERNAL_DELETED", deleted)
		t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
		ipv6Probe := filepath.Join(dir, "ipv6-probed")
		t.Setenv("STAGE7_IPV6_PROBED", ipv6Probe)
		fakeFirewallBinary(t, dir, "ip6tables", `echo probed > "$STAGE7_IPV6_PROBED"`)
		fakeFirewallBinary(t, dir, "iptables", `if [ "$1" = -S ]; then
cat <<'RULES'
-P INPUT ACCEPT
-A INPUT -p tcp --dport 99 -m comment --comment "literal $(whoami); spaces" -j DROP
-A INPUT -j XUI-FIREWALL
-A XUI-FIREWALL -p tcp --dport 80 -j ACCEPT
RULES
else printf '%s\n' "$@" > "$STAGE7_EXTERNAL_DELETED"; fi`)
		b := firewallBackend{name: "iptables", binary: filepath.Join(dir, "iptables")}
		rules, err := listExternalFirewallRules(context.Background(), b)
		if err != nil || len(rules) != 1 {
			t.Fatal(rules, err)
		}
		if _, err := os.Stat(ipv6Probe); err != nil {
			t.Fatal("IPv6 inventory was not inspected", err)
		}
		if err := deleteExternalFirewallRule(context.Background(), b, rules[0].ID); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(deleted)
		if !strings.Contains(string(got), "--comment\nliteral $(whoami); spaces\n") || !strings.HasPrefix(string(got), "-w\n5\n-D\nINPUT\n") {
			t.Fatal(string(got))
		}
	})
	t.Run("ufw keeps restricted rule intact", func(t *testing.T) {
		dir := t.TempDir()
		fakeFirewallBinary(t, dir, "ufw", `cat <<'RULES'
ufw allow from 10.0.0.0/8 to any port 443 proto tcp
ufw allow 80/tcp comment '3x-ui managed'
RULES`)
		rules, err := listExternalFirewallRules(context.Background(), firewallBackend{name: "ufw", binary: filepath.Join(dir, "ufw")})
		if err != nil || len(rules) != 1 {
			t.Fatal(rules, err)
		}
		want := []string{"--force", "delete", "allow", "from", "10.0.0.0/8", "to", "any", "port", "443", "proto", "tcp"}
		if !reflect.DeepEqual(rules[0].args, want) {
			t.Fatal(rules[0].args)
		}
	})
	t.Run("firewalld rich rule stays one argument", func(t *testing.T) {
		dir := t.TempDir()
		fakeFirewallBinary(t, dir, "firewall-cmd", `echo 'rule family="ipv4" source address="10.0.0.1" reject'`)
		rules, err := listExternalFirewallRules(context.Background(), firewallBackend{name: "firewalld", binary: filepath.Join(dir, "firewall-cmd"), zone: "public"})
		if err != nil || len(rules) != 1 || len(rules[0].args) != 2 {
			t.Fatal(rules, err)
		}
		other, err := listExternalFirewallRules(context.Background(), firewallBackend{name: "firewalld", binary: filepath.Join(dir, "firewall-cmd"), zone: "private"})
		if err != nil || len(other) != 1 || other[0].ID == rules[0].ID {
			t.Fatal("selection survived zone change", other, err)
		}
	})
}

func TestStage7FirewallRuleLexer(t *testing.T) {
	got, err := firewallRuleWords(`-A INPUT --comment "a b" --empty '' --other 'c"d'`)
	if err != nil || !reflect.DeepEqual(got, []string{"-A", "INPUT", "--comment", "a b", "--empty", "", "--other", `c"d`}) {
		t.Fatal(got, err)
	}
	for _, s := range []string{`"unterminated`, `end\`} {
		if _, err := firewallRuleWords(s); err == nil {
			t.Fatal(s)
		}
	}
}
