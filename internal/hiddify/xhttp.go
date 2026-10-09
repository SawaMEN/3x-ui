package hiddify

import (
	"fmt"
	"maps"
	"reflect"
)

var xhttpFields = map[string]string{
	"host": "host", "path": "path", "mode": "mode", "headers": "headers",
	"domainStrategy": "domain_strategy", "xPaddingBytes": "x_padding_bytes",
	"noGRPCHeader": "no_grpc_header", "noSSEHeader": "no_sse_header",
	"scMaxEachPostBytes": "sc_max_each_post_bytes", "scMinPostsIntervalMs": "sc_min_posts_interval_ms",
	"scMaxBufferedPosts": "sc_max_buffered_posts", "scStreamUpServerSecs": "sc_stream_up_server_secs",
	"xPaddingObfsMode": "x_padding_obfs_mode", "xPaddingKey": "x_padding_key",
	"xPaddingHeader": "x_padding_header", "xPaddingPlacement": "x_padding_placement", "xPaddingMethod": "x_padding_method",
	"uplinkHTTPMethod": "uplink_http_method", "sessionPlacement": "session_placement", "sessionKey": "session_key",
	"sessionIDPlacement": "session_placement", "sessionIDKey": "session_key",
	"seqPlacement": "seq_placement", "seqKey": "seq_key", "uplinkDataPlacement": "uplink_data_placement",
	"uplinkDataKey": "uplink_data_key", "uplinkChunkSize": "uplink_chunk_size",
}

var xmuxFields = map[string]string{
	"maxConcurrency": "max_concurrency", "maxConnections": "max_connections",
	"cMaxReuseTimes": "c_max_reuse_times", "hMaxRequestTimes": "h_max_request_times",
	"hMaxReusableSecs": "h_max_reusable_secs", "hKeepAlivePeriod": "h_keep_alive_period",
}

func effectiveXHTTPOptions(raw map[string]any, label string) (map[string]any, error) {
	options := maps.Clone(raw)
	if extra, exists := options["extra"]; exists && extra != nil {
		object, ok := extra.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: xhttpSettings.extra must be an object", label)
		}
		options = maps.Clone(object)
		for _, key := range []string{"host", "path", "mode"} {
			delete(options, key)
			if value, exists := raw[key]; exists {
				options[key] = value
			}
		}
	}
	return options, nil
}

func translateXHTTP(raw map[string]any, inbound bool, label string) (map[string]any, error) {
	options, err := effectiveXHTTPOptions(raw, label)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"type": "xhttp"}
	for key, value := range options {
		switch key {
		case "xmux":
			if value == nil {
				continue
			}
			fields, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s: xhttpSettings.xmux must be an object", label)
			}
			xmux := map[string]any{}
			for name, setting := range fields {
				native, ok := xmuxFields[name]
				if !ok {
					return nil, fmt.Errorf("%s: unsupported xhttpSettings.xmux.%s", label, name)
				}
				xmux[native] = setting
			}
			out["xmux"] = xmux
		case "downloadSettings":
			if meaningful(value) {
				if inbound {
					return nil, fmt.Errorf("%s: downloadSettings is outbound-only", label)
				}
				download, ok := value.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%s: downloadSettings must be an object", label)
				}
				translated, err := translateDownload(download, label)
				if err != nil {
					return nil, err
				}
				out["download"] = translated
			}
		case "enableXmux":
			// This is editor state; the actual xmux object carries runtime settings.
		case "extra":
		default:
			native, ok := xhttpFields[key]
			if !ok {
				if !meaningful(value) {
					continue
				}
				return nil, fmt.Errorf("%s: unsupported xhttpSettings.%s", label, key)
			}
			if value == "" {
				continue
			}
			if previous, exists := out[native]; exists && !reflect.DeepEqual(previous, value) {
				return nil, fmt.Errorf("%s: conflicting XHTTP aliases for %s", label, native)
			}
			out[native] = value
		}
	}
	if mode := out["mode"]; mode != nil && mode != "" && mode != "auto" && mode != "packet-up" && mode != "stream-up" && mode != "stream-one" {
		return nil, fmt.Errorf("%s: invalid XHTTP mode %v", label, mode)
	}
	return out, nil
}

func translateDownload(raw map[string]any, label string) (map[string]any, error) {
	for key, value := range raw {
		switch key {
		case "address", "port", "network", "security", "tlsSettings", "realitySettings", "xhttpSettings", "sockopt":
		default:
			if meaningful(value) {
				return nil, fmt.Errorf("%s: unsupported downloadSettings.%s", label, key)
			}
		}
	}
	if raw["network"] != "xhttp" {
		return nil, fmt.Errorf("%s: downloadSettings.network must be xhttp", label)
	}
	if value := raw["xhttpSettings"]; value != nil {
		if _, ok := value.(map[string]any); !ok {
			return nil, fmt.Errorf("%s: download xhttpSettings must be an object", label)
		}
	}
	xhttp, err := effectiveXHTTPOptions(object(raw, "xhttpSettings"), label)
	if err != nil {
		return nil, err
	}
	if mode := xhttp["mode"]; meaningful(mode) && mode != "auto" {
		return nil, fmt.Errorf("%s: explicit download XHTTP mode cannot be preserved", label)
	}
	if meaningful(xhttp["downloadSettings"]) {
		return nil, fmt.Errorf("%s: nested downloadSettings cannot be preserved", label)
	}
	out, err := TranslateOutbound(map[string]any{
		"protocol": "vless", "tag": label + " download",
		"settings": map[string]any{
			"address": raw["address"], "port": raw["port"],
			"id": "11111111-1111-4111-8111-111111111111",
		},
		"streamSettings": raw,
	})
	if err != nil {
		return nil, err
	}
	download := object(out, "transport")
	delete(download, "type")
	delete(download, "mode")
	for key, value := range out {
		switch key {
		case "server", "server_port", "tls", "detour":
			download[key] = value
		case "type", "tag", "uuid", "transport":
		default:
			return nil, fmt.Errorf("%s: downloadSettings cannot preserve %s", label, key)
		}
	}
	return download, nil
}
