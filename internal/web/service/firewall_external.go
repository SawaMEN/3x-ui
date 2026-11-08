package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// IDs are fingerprints of freshly inspected rules, never executable client input.
// Reconciliation only changes owned rules; external rules require explicit deletion.
type FirewallExternalRule struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type externalFirewallRule struct {
	FirewallExternalRule
	args   []string
	binary string
}

func externalRule(backend, label string, args []string) externalFirewallRule {
	sum := sha256.Sum256([]byte(backend + "\x00" + label))
	return externalFirewallRule{FirewallExternalRule: FirewallExternalRule{hex.EncodeToString(sum[:]), label}, args: args}
}

// Parse the quoting emitted by iptables -S and ufw show added. This is a lexer,
// not a shell: substitutions, operators and expansions are always literal.
func firewallRuleWords(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, c := range line {
		if escaped {
			word.WriteRune(c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			started = true
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			started = true
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(c)
		started = true
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("malformed firewall rule quoting")
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}

func nftExternalRules(text string) ([]externalFirewallRule, error) {
	var doc struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	var out []externalFirewallRule
	for _, item := range doc.Nftables {
		raw, ok := item["rule"]
		if !ok {
			continue
		}
		var rule struct {
			Family, Table, Chain string
			Comment              string
			Handle               uint64
			Expr                 []map[string]json.RawMessage
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			return nil, err
		}
		if rule.Handle == 0 || (rule.Family != "ip" && rule.Family != "ip6" && rule.Family != "inet") {
			continue
		}
		if rule.Table == managedNftTable || rule.Table == "xui_gateway" || rule.Table == "xui_gateway_nat" {
			continue
		}
		// Anonymous counters change on every packet, and are not rule identity.
		for _, expr := range rule.Expr {
			if _, ok := expr["counter"]; ok {
				var counts map[string]json.RawMessage
				if json.Unmarshal(expr["counter"], &counts) == nil && counts != nil {
					delete(counts, "packets")
					delete(counts, "bytes")
					expr["counter"], _ = json.Marshal(counts)
				}
			}
		}
		stable, err := json.Marshal(rule)
		if err != nil {
			return nil, err
		}
		args := []string{"delete", "rule", rule.Family, rule.Table, rule.Chain, "handle", strconv.FormatUint(rule.Handle, 10)}
		out = append(out, externalRule("nftables", string(stable), args))
	}
	return out, nil
}

func listExternalFirewallRules(ctx context.Context, b firewallBackend) ([]externalFirewallRule, error) {
	rules, err := listExternalFirewallRulesForBackend(ctx, b)
	if err != nil {
		return nil, err
	}
	occurrences := make(map[string]int, len(rules))
	for i := range rules {
		rules[i].binary = b.binary
		identity := rules[i].ID
		ordinal := occurrences[identity]
		occurrences[identity]++
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d", b.name, b.binary, b.zone, identity, ordinal)))
		rules[i].ID = hex.EncodeToString(sum[:])
	}
	if b.name == "iptables" {
		ip6, err := managedIP6TablesPath(b.binary)
		if err != nil {
			return nil, err
		}
		if ip6 != "" {
			v6, err := listExternalFirewallRules(ctx, firewallBackend{name: "ip6tables", binary: ip6})
			if err != nil {
				return nil, err
			}
			rules = append(rules, v6...)
		}
	}
	return rules, nil
}

func listExternalFirewallRulesForBackend(ctx context.Context, b firewallBackend) ([]externalFirewallRule, error) {
	var out []externalFirewallRule
	switch b.name {
	case "nftables":
		text, err := runFirewallCommand(ctx, b.binary, "-j", "list", "ruleset")
		if err != nil {
			return nil, err
		}
		return nftExternalRules(text)
	case "iptables", "ip6tables":
		text, err := runFirewallCommand(ctx, b.binary, "-S")
		if err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(text, "\n") {
			args, err := firewallRuleWords(line)
			if err != nil {
				return nil, err
			}
			if len(args) < 2 || args[0] != "-A" || args[1] == managedIPTablesChain {
				continue
			}
			owned := false
			for i := 2; i+1 < len(args); i++ {
				if (args[i] == "-j" || args[i] == "-g") && args[i+1] == managedIPTablesChain {
					owned = true
				}
			}
			if owned {
				continue
			}
			args[0] = "-D"
			out = append(out, externalRule(b.name, line, append([]string{"-w", "5"}, args...)))
		}
	case "ufw":
		text, err := runFirewallCommand(ctx, b.binary, "show", "added")
		if err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(text, "\n") {
			args, err := firewallRuleWords(line)
			if err != nil {
				return nil, err
			}
			if len(args) < 3 || args[0] != "ufw" {
				continue
			}
			if args[1] != "allow" && args[1] != "deny" && args[1] != "reject" && args[1] != "limit" && args[1] != "route" {
				continue
			}
			// Owned allow rules have this exact generated comment.
			owned := false
			for i := 2; i+1 < len(args); i++ {
				if args[i] == "comment" && args[i+1] == "3x-ui managed" {
					owned = true
				}
			}
			if owned {
				continue
			}
			out = append(out, externalRule(b.name, line, append([]string{"--force", "delete"}, args[1:]...)))
		}
	case "firewalld":
		// Rich rules cannot be represented as a single open port. Preserve their
		// full expression and remove only the exact runtime rule the user selected.
		text, err := runFirewallCommand(ctx, b.binary, "--zone="+b.zone, "--list-rich-rules")
		if err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			out = append(out, externalRule(b.name, line, []string{"--zone=" + b.zone, "--remove-rich-rule=" + line}))
		}
	}
	return out, nil
}

func externalRuleViews(rules []externalFirewallRule) []FirewallExternalRule {
	out := make([]FirewallExternalRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, rule.FirewallExternalRule)
	}
	return out
}

func deleteExternalFirewallRule(ctx context.Context, b firewallBackend, id string) error {
	if len(id) != 64 {
		return fmt.Errorf("invalid firewall rule ID")
	}
	rules, err := listExternalFirewallRules(ctx, b)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.ID != id {
			continue
		}
		_, err := runFirewallCommand(ctx, rule.binary, rule.args...)
		return err
	}
	return fmt.Errorf("firewall rule changed or disappeared; refresh before deleting")
}

func (s *FirewallService) DeleteExternalFirewallRule(ctx context.Context, id string, safetyPort int) (FirewallManagedStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()
	b, err := detectManagedFirewallBackend(ctx)
	if err != nil {
		return FirewallManagedStatus{}, err
	}
	if err = deleteExternalFirewallRule(ctx, b, id); err != nil {
		return FirewallManagedStatus{}, err
	}
	return s.managedStatusSafeLocked(ctx, safetyPort)
}
