package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/outbound"
)

func subscriptionCompatibilityCategory(issue string) string {
	lower := strings.ToLower(issue)
	switch {
	case strings.Contains(lower, "unsupported vless encryption"):
		return "VLESS encryption"
	case strings.Contains(lower, "unsupported xray transport \"xhttp\""):
		return "XHTTP transport"
	case strings.Contains(lower, "grpc authority"):
		return "gRPC authority"
	case strings.Contains(lower, "grpc user_agent"):
		return "gRPC user_agent"
	case strings.Contains(lower, "grpc multimode"):
		return "gRPC multiMode"
	case strings.Contains(lower, "grpc initial_windows_size"):
		return "gRPC window settings"
	case strings.Contains(lower, "xray quic transport"):
		return "Xray QUIC transport"
	default:
		return "other"
	}
}

func logSubscriptionCompatibilitySummary(label, coreName string, issues []string) {
	if len(issues) == 0 {
		return
	}
	counts := make(map[string]int)
	for _, issue := range issues {
		counts[subscriptionCompatibilityCategory(issue)]++
	}
	categories := make([]string, 0, len(counts))
	for category := range counts {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	parts := make([]string, 0, len(categories))
	for _, category := range categories {
		parts = append(parts, fmt.Sprintf("%s=%d", category, counts[category]))
	}
	logger.Warningf(
		"%s: %d outbound(s) are incompatible with current %s runtime and were skipped (%s); profiles remain stored for core switching/upgrades",
		label, len(issues), coreName, strings.Join(parts, ", "),
	)
}

// filterSubscriptionOutboundsWithIssues returns every incompatibility so the
// subscription UI can explain excluded profiles without failing the refresh.
// Original source profiles remain stored for future kernel upgrades.
func filterSubscriptionOutboundsWithIssues(label string, members []any) ([]any, []string, string) {
	core, _ := (&SettingService{}).GetCoreType()
	return filterSubscriptionOutboundsForCore(label, core, members)
}

func filterSubscriptionOutboundsForCore(label, core string, members []any) ([]any, []string, string) {
	if !IsNativeCore(core) {
		kept, compatibilityIssues := filterOutboundsRejectedByCore(label, members)
		logSubscriptionCompatibilitySummary(label, "xray", compatibilityIssues)
		return kept, compatibilityIssues, "xray"
	}

	kept := make([]any, 0, len(members))
	statusIssues := make([]string, 0)
	compatibilityIssues := make([]string, 0)
	for index, raw := range members {
		ob, ok := raw.(map[string]any)
		if !ok {
			statusIssues = append(statusIssues, fmt.Sprintf("profile %d: invalid outbound JSON object", index+1))
			continue
		}
		tag, _ := ob["tag"].(string)
		if strings.TrimSpace(tag) == "" {
			tag = fmt.Sprintf("profile %d", index+1)
		}
		protocol, _ := ob["protocol"].(string)
		var err error
		kind := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(protocol)), "singbox:")
		switch kind {
		case "masque-server", "openvpn-server", "tunnel_server", "hinvalid":
			compatibilityIssues = append(compatibilityIssues, fmt.Sprintf("%s: %s не поддерживается как исходящий клиент", tag, kind))
			continue
		}
		if externalvpn.IsAdditionalOutbound(ob) {
			if err := externalvpn.ValidateAdditionalOutbound(ob); err != nil {
				statusIssues = append(statusIssues, fmt.Sprintf("%s: %v", tag, err))
			} else {
				kept = append(kept, raw)
			}
			continue
		}
		if _, handled, endpointErr := singbox.TranslatePanelEndpoint(ob); handled {
			err = endpointErr
		} else {
			switch strings.ToLower(strings.TrimSpace(protocol)) {
			case "loopback":
				err = fmt.Errorf("Xray loopback members cannot be translated to sing-box")
			case "amneziawg":
				// The runtime and probe builders supply a SOCKS bridge.
			default:
				_, err = NativeCore(core).translateOutbound(ob)
			}
		}
		if err != nil {
			compatibilityIssues = append(compatibilityIssues, fmt.Sprintf("%s: %v", tag, err))
			continue
		}
		if kind != "amneziawg" {
			if err := validateSubscriptionNativeMember(ob, core); err != nil {
				compatibilityIssues = append(compatibilityIssues, fmt.Sprintf("%s: %v", tag, err))
				continue
			}
		}
		kept = append(kept, raw)
	}
	kept, dependencyIssues := pruneSubscriptionDependencies(members, kept)
	compatibilityIssues = append(compatibilityIssues, dependencyIssues...)
	logSubscriptionCompatibilitySummary(label, core, compatibilityIssues)
	return kept, append(statusIssues, compatibilityIssues...), core
}

// Unsupported subscription members must not prevent the active core starting.
// Keep the original stored subscription so switching cores can restore them.
func filterSubscriptionOutbounds(label string, members []any) []any {
	kept, _, _ := filterSubscriptionOutboundsWithIssues(label, members)
	return kept
}

// Keep references to manual template tags, but drop chains whose imported hop
// was rejected by the active core. Repeat to remove transitive dependents.
func pruneSubscriptionDependencies(source, kept []any) ([]any, []string) {
	original, available := map[string]bool{}, map[string]bool{}
	for _, raw := range source {
		if row, ok := raw.(map[string]any); ok {
			if tag, _ := row["tag"].(string); tag != "" {
				original[tag] = true
			}
		}
	}
	for _, raw := range kept {
		if row, ok := raw.(map[string]any); ok {
			if tag, _ := row["tag"].(string); tag != "" {
				available[tag] = true
			}
		}
	}
	// A cycle can look locally valid when each member is checked in isolation.
	// Reject the cycle and let the ordinary pass prune all dependent chains.
	graph := map[string][]string{}
	for _, raw := range kept {
		if row, ok := raw.(map[string]any); ok {
			tag, _ := row["tag"].(string)
			graph[tag] = outbound.OutboundDependencies(row)
		}
	}
	state, cyclic := map[string]int{}, map[string]bool{}
	var stack []string
	var visit func(string)
	visit = func(tag string) {
		if state[tag] == 2 {
			return
		}
		if state[tag] == 1 {
			for i, node := range stack {
				if node == tag {
					for _, member := range stack[i:] {
						cyclic[member] = true
					}
					break
				}
			}
			return
		}
		state[tag] = 1
		stack = append(stack, tag)
		for _, dependency := range graph[tag] {
			if _, known := graph[dependency]; known {
				visit(dependency)
			}
		}
		stack = stack[:len(stack)-1]
		state[tag] = 2
	}
	for tag := range graph {
		visit(tag)
	}
	var issues []string
	remaining := make([]any, 0, len(kept))
	for _, raw := range kept {
		row, _ := raw.(map[string]any)
		tag, _ := row["tag"].(string)
		if cyclic[tag] {
			delete(available, tag)
			issues = append(issues, fmt.Sprintf("%s: циклическая цепочка подключений, пропущена", tag))
		} else {
			remaining = append(remaining, raw)
		}
	}
	kept = remaining
	for {
		changed := false
		remaining := make([]any, 0, len(kept))
		for _, raw := range kept {
			row, _ := raw.(map[string]any)
			missing := ""
			for _, dependency := range outbound.OutboundDependencies(row) {
				if original[dependency] && !available[dependency] {
					missing = dependency
					break
				}
			}
			if missing != "" {
				tag, _ := row["tag"].(string)
				delete(available, tag)
				issues = append(issues, fmt.Sprintf("%s: incompatible cascade dependency %q", tag, missing))
				changed = true
			} else {
				remaining = append(remaining, raw)
			}
		}
		kept = remaining
		if !changed {
			return kept, issues
		}
	}
}

// A chain is a single candidate. Never select its intermediate transport or a
// direct/block utility profile instead of the full proxy connection.
func subscriptionCascadeCandidates(members []any) []any {
	dependencies := map[string]bool{}
	for _, raw := range members {
		if row, ok := raw.(map[string]any); ok {
			for _, tag := range outbound.OutboundDependencies(row) {
				dependencies[tag] = true
			}
		}
	}
	var roots []any
	for _, raw := range members {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := row["tag"].(string)
		protocol, _ := row["protocol"].(string)
		kind := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(protocol)), "singbox:")
		switch kind {
		case "shadowtls", "block", "blackhole", "direct", "freedom", "dns", "loopback", "bridge":
			continue
		}
		if tag != "" && !dependencies[tag] {
			roots = append(roots, raw)
		}
	}
	return roots
}
