package service

import (
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
)

// validateNativeSubscriptionMembersConcurrently bounds the number of
// simultaneous kernel check subprocesses to protect small VDS instances.
// Results remain indexed by source order; a failed check never changes the
// imported profile or the ordering of healthy candidates.
func validateNativeSubscriptionMembersConcurrently(members []any, core string) []error {
	results := make([]error, len(members))
	if len(members) == 0 {
		return results
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for i := 0; i < min(4, len(members)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				row, _ := members[index].(map[string]any)
				results[index] = validateSubscriptionNativeMember(row, core)
			}
		}()
	}
	for index, raw := range members {
		row, ok := raw.(map[string]any)
		if !ok || externalvpn.IsAdditionalOutbound(row) {
			continue
		}
		protocol, _ := row["protocol"].(string)
		kind := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(protocol)), "singbox:")
		switch kind {
		case "masque-server", "openvpn-server", "tunnel_server", "hinvalid", "amneziawg", "loopback":
			continue
		}
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return results
}
