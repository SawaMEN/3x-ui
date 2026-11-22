package zapret2

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	DefaultPort         = 15900
	MaxVirtualOutbounds = 32
)

type Options struct {
	Port    int    `json:"port"`
	Profile string `json:"profile"`
	TCP     bool   `json:"tcp"`
	UDP     bool   `json:"udp"`
	// Target domains for automatic strategy diagnostics; not hostlist filters.
	TestDomains []string     `json:"testDomains,omitempty"`
	Calibration *Calibration `json:"calibration,omitempty"`
}

type Plan struct {
	Tag     string  `json:"tag"`
	Options Options `json:"settings"`
	Queue   uint16  `json:"-"`
	Mark    uint32  `json:"-"`
}

func Parse(tag string, raw any) (Plan, error) {
	var opts Options
	data, err := json.Marshal(raw)
	if err != nil {
		return Plan{}, err
	}
	if err := json.Unmarshal(data, &opts); err != nil {
		return Plan{}, fmt.Errorf("zapret2 %q: %w", tag, err)
	}
	if opts.Port == 0 {
		opts.Port = DefaultPort
	}
	if opts.Profile == "" {
		opts.Profile = "auto"
	}
	if !opts.TCP && !opts.UDP {
		opts.TCP = true
	}
	if opts.Port < 1025 || opts.Port > 65535 {
		return Plan{}, fmt.Errorf("zapret2 %q: localhost SOCKS port must be 1025..65535", tag)
	}
	switch opts.Profile {
	case "auto", "safe", "balanced", "aggressive":
	default:
		return Plan{}, fmt.Errorf("zapret2 %q: unsupported strategy %q", tag, opts.Profile)
	}
	if len(opts.TestDomains) > 32 {
		return Plan{}, fmt.Errorf("zapret2 %q: too many diagnostic hosts", tag)
	}
	if err := ValidateCalibration(opts.Calibration); err != nil {
		return Plan{}, fmt.Errorf("zapret2 %q: invalid calibrated strategy: %w", tag, err)
	}
	for _, domain := range opts.TestDomains {
		if len(domain) > 253 || strings.ContainsAny(domain, " /\\\r\n\t;&|") || strings.HasPrefix(domain, "-") {
			return Plan{}, fmt.Errorf("zapret2 %q: invalid test hostname", tag)
		}
	}
	if strings.TrimSpace(tag) == "" {
		return Plan{}, fmt.Errorf("zapret2 outbound tag is required")
	}
	return Plan{Tag: tag, Options: opts}, nil
}

func AssignQueues(plans []Plan) ([]Plan, error) {
	if len(plans) > MaxVirtualOutbounds {
		return nil, fmt.Errorf("at most %d zapret2 outbounds are supported", MaxVirtualOutbounds)
	}
	out := append([]Plan(nil), plans...)
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	ports, tags := map[int]bool{}, map[string]bool{}
	for i := range out {
		if tags[out[i].Tag] {
			return nil, fmt.Errorf("duplicate zapret2 outbound %q", out[i].Tag)
		}
		if ports[out[i].Options.Port] {
			return nil, fmt.Errorf("duplicate zapret2 SOCKS port %d", out[i].Options.Port)
		}
		ports[out[i].Options.Port] = true
		tags[out[i].Tag] = true
		out[i].Queue = uint16(2400 + i)
		// nfqws2 adds its default 0x40000000 mark to its own synthesized
		// packets. Keep that bit CLEAR in source route marks to prevent loops.
		out[i].Mark = 0x22000000 | uint32(i+1)
	}
	return out, nil
}
