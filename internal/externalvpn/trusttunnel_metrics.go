package externalvpn

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
)

type trustTunnelMetric struct {
	Username string `json:"username"`
	Sessions int    `json:"sessions"`
	Inbound  uint64 `json:"inbound"`
	Outbound uint64 `json:"outbound"`
}

func readTrustTunnelMetrics(address string) ([]trustTunnelMetric, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+address+"/clients", nil)
	if err != nil {
		return nil, errors.New("client metrics unavailable")
	}
	response, err := metricsHTTPClient.Do(request)
	if err != nil {
		return nil, errors.New("client metrics unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("client metrics response rejected")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	var clients []trustTunnelMetric
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, &clients) != nil || clients == nil {
		return nil, errors.New("invalid client metrics response")
	}
	seen := map[string]bool{}
	for _, row := range clients {
		if row.Username == "" {
			continue
		}
		if strings.ContainsRune(row.Username, 0) || seen[row.Username] {
			return nil, errors.New("invalid or duplicate client metrics identity")
		}
		seen[row.Username] = true
	}
	return clients, nil
}

// Keep per-process baselines in the same map that the existing accounting
// transaction snapshots and restores. A NUL prefix cannot collide with a
// validated TrustTunnel username. Compute deltas before merging, not after.
func collectTrustTunnelMetrics(proc *running, primary, secondary []trustTunnelMetric, includeIdle bool) []TrafficDelta {
	if proc.counters == nil {
		proc.counters = map[string]trafficCounters{}
	}
	byEmail := map[string]TrafficDelta{}
	for i, rows := range [][]trustTunnelMetric{primary, secondary} {
		for _, row := range rows {
			if row.Username == "" {
				continue
			}
			key := row.Username
			if i == 1 {
				key = "\x00quic:" + key
			}
			old := proc.counters[key]
			delta := byEmail[row.Username]
			delta.Tag, delta.Email = proc.tag, row.Username
			delta.Up = saturatedTrafficSum(delta.Up, cumulativeTrafficDelta(row.Inbound, old.up))
			delta.Down = saturatedTrafficSum(delta.Down, cumulativeTrafficDelta(row.Outbound, old.down))
			delta.Active = delta.Active || row.Sessions > 0
			byEmail[row.Username] = delta
			proc.counters[key] = trafficCounters{up: row.Inbound, down: row.Outbound}
		}
	}
	out := make([]TrafficDelta, 0, len(byEmail))
	for _, delta := range byEmail {
		if includeIdle || delta.Up > 0 || delta.Down > 0 || delta.Active {
			out = append(out, delta)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out
}
