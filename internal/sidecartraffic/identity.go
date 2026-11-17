package sidecartraffic

import "maps"

// SameIdentities compares immutable accounting identities, not pointer allocation.
func (o *Owner) SameIdentities(next *Owner) bool {
	if o == nil || next == nil {
		return o == next
	}
	return o.source == next.source && o.dir == next.dir && o.inboundID == next.inboundID && o.routed == next.routed && maps.Equal(o.clients, next.clients)
}

// ChangedClients identifies exactly which counters need a fresh handoff boundary.
func (o *Owner) ChangedClients(next *Owner) []string {
	if o.SameIdentities(next) {
		return nil
	}
	var before, after map[string]int
	if o != nil {
		before = o.clients
	}
	if next != nil {
		after = next.clients
	}
	namespace := o == nil || next == nil
	if !namespace {
		namespace = o.source != next.source || o.dir != next.dir || o.inboundID != next.inboundID || o.routed != next.routed
	}
	changed := map[string]bool{}
	for email, id := range before {
		if namespace || after[email] != id {
			changed[email] = true
		}
	}
	for email, id := range after {
		if namespace || before[email] != id {
			changed[email] = true
		}
	}
	out := make([]string, 0, len(changed))
	for email := range changed {
		out = append(out, email)
	}
	return out
}
