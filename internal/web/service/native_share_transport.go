package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// The stable kernel implements mKCP but its legacy URI converter does not yet
// recognize type=kcp. Decode the common protocol/security layer as TCP, then
// attach the exact native KCP options. No transport or mask is silently lost.
func prepareNativeShareTransport(raw string) (string, map[string]any, error) {
	if strings.HasPrefix(raw, "vmess://") || strings.HasPrefix(raw, "svmess://") {
		scheme, payload, _ := strings.Cut(raw, "://")
		var data []byte
		var err error
		for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
			data, err = encoding.DecodeString(payload)
			if err == nil {
				break
			}
		}
		if err != nil {
			return raw, nil, nil
		}
		var fields map[string]any
		if json.Unmarshal(data, &fields) != nil {
			return raw, nil, nil
		}
		if fields["net"] != "kcp" && fields["net"] != "mkcp" {
			return raw, nil, nil
		}
		query := url.Values{}
		for _, key := range []string{"seed", "mtu", "tti", "uplinkCapacity", "downlinkCapacity", "cwndMultiplier", "maxSendingWindow"} {
			if value, exists := fields[key]; exists {
				query.Set(key, fmt.Sprint(value))
			}
		}
		if header, ok := fields["type"].(string); ok {
			query.Set("headerType", header)
		}
		transport, err := nativeKCPOptions(query)
		if err != nil {
			return "", nil, err
		}
		fields["net"], fields["type"] = "tcp", "none"
		encoded, err := json.Marshal(fields)
		if err != nil {
			return "", nil, err
		}
		return scheme + "://" + base64.StdEncoding.EncodeToString(encoded), transport, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw, nil, nil
	}
	if u.Scheme != "vless" && u.Scheme != "trojan" && u.Scheme != "svless" && u.Scheme != "strojan" {
		return raw, nil, nil
	}
	q := u.Query()
	if q.Get("flow") == "xtls-rprx-vision-udp443" {
		q.Set("flow", "xtls-rprx-vision")
		q.Set("packet_encoding", "xudp")
		u.RawQuery = q.Encode()
		raw = u.String()
	}
	if q.Get("type") != "kcp" && q.Get("type") != "mkcp" {
		return raw, nil, nil
	}
	transport, err := nativeKCPOptions(q)
	if err != nil {
		return "", nil, err
	}
	q.Set("type", "tcp")
	q.Set("headerType", "none")
	u.RawQuery = q.Encode()
	return u.String(), transport, nil
}

func nativeKCPOptions(q url.Values) (map[string]any, error) {
	out := map[string]any{"type": "kcp", "legacy": true}
	if seed := q.Get("seed"); seed != "" {
		out["seed"] = seed
	}
	if header := q.Get("headerType"); header != "" && header != "none" {
		out["header"] = header
	}
	for key, field := range map[string]string{"mtu": "mtu", "tti": "tti", "uplinkCapacity": "uplink_capacity", "downlinkCapacity": "downlink_capacity", "cwndMultiplier": "cwnd_multiplier", "maxSendingWindow": "max_sending_window"} {
		value := q.Get(key)
		if value == "" {
			value = q.Get(field)
		}
		if value == "" {
			continue
		}
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid mKCP option %s", field)
		}
		out[field] = n
	}
	return out, nil
}
