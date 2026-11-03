package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Only an unconditional generated port accept proves a desired port is present.
// A source/interface-restricted administrator rule must not become a global allow.
func observedNftPorts(text string) (map[string]bool, error) {
	var doc struct {
		Nftables []struct {
			Rule *struct {
				Family, Table, Chain string
				Expr                 []json.RawMessage
			} `json:"rule"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, item := range doc.Nftables {
		r := item.Rule
		if r == nil || r.Family != "inet" || r.Table != managedNftTable || r.Chain != managedNftChain || len(r.Expr) != 2 {
			continue
		}
		var accept map[string]json.RawMessage
		if json.Unmarshal(r.Expr[1], &accept) != nil || len(accept) != 1 {
			continue
		}
		if _, ok := accept["accept"]; !ok {
			continue
		}
		var match struct {
			Match struct {
				Op   string
				Left struct {
					Payload struct{ Protocol, Field string }
				}
				Right json.RawMessage
			}
		}
		if json.Unmarshal(r.Expr[0], &match) != nil || match.Match.Op != "==" || match.Match.Left.Payload.Field != "dport" {
			continue
		}
		protocol := match.Match.Left.Payload.Protocol
		if protocol != "tcp" && protocol != "udp" {
			continue
		}
		port := ""
		var n int
		if json.Unmarshal(match.Match.Right, &n) == nil {
			port = fmt.Sprint(n)
		} else {
			var span struct{ Range []int }
			if json.Unmarshal(match.Match.Right, &span) == nil && len(span.Range) == 2 {
				port = fmt.Sprintf("%d-%d", span.Range[0], span.Range[1])
			}
		}
		if key, ok := canonicalFirewallSpec(port + "/" + protocol); ok {
			out[key] = true
		}
	}
	return out, nil
}

func observedIPTablesPorts(text string) (map[string]bool, error) {
	out := map[string]bool{}
	for line := range strings.SplitSeq(text, "\n") {
		args, err := firewallRuleWords(line)
		if err != nil {
			return nil, err
		}
		if len(args) < 2 || args[0] != "-A" || args[1] != managedIPTablesChain {
			continue
		}
		protocol, port, verdict := "", "", ""
		valid := true
		for i := 2; i < len(args); i += 2 {
			if i+1 >= len(args) {
				valid = false
				break
			}
			switch args[i] {
			case "-p":
				protocol = args[i+1]
			case "--dport":
				port = args[i+1]
			case "-j":
				verdict = args[i+1]
			case "--comment":
				// Comments do not restrict a port accept.
			case "-m":
				if args[i+1] != "tcp" && args[i+1] != "udp" && args[i+1] != "comment" {
					valid = false
				}
			default:
				valid = false
			}
		}
		if valid && verdict == "ACCEPT" {
			if key, ok := canonicalFirewallSpec(strings.ReplaceAll(port, ":", "-") + "/" + protocol); ok {
				out[key] = true
			}
		}
	}
	return out, nil
}

func observedNativePorts(ctx context.Context, b firewallBackend) (map[string]bool, error) {
	if b.name == "nftables" {
		text, err := runFirewallCommand(ctx, b.binary, "-j", "list", "table", "inet", managedNftTable)
		if err != nil {
			return nil, err
		}
		return observedNftPorts(text)
	}
	text, err := runFirewallCommand(ctx, b.binary, "-S", managedIPTablesChain)
	if err != nil {
		return nil, err
	}
	return observedIPTablesPorts(text)
}
