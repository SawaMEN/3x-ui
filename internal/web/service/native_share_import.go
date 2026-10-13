package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

const maxNativeShareSize = 1 << 20

type NativeShareImport struct {
	Outbounds []map[string]any `json:"outbounds"`
	Endpoints []map[string]any `json:"endpoints"`
}

// ParseNativeOutboundImport uses the installed release's parser, including its
// native panel URI codecs. Panel capability lists must not reject newer core
// features. Raw native JSON is retained verbatim for the full-config validator.
func ParseNativeOutboundImport(raw string) (*NativeShareImport, error) {
	return ParseNativeOutboundImportContext(context.Background(), raw)
}

func ParseNativeOutboundImportContext(ctx context.Context, raw string) (*NativeShareImport, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > maxNativeShareSize {
		return nil, fmt.Errorf("import must contain 1 byte to 1 MiB")
	}
	if native, handled, err := decodeNativeShare(raw); handled {
		return native, err
	}
	if strings.HasPrefix(raw, "ssh://") && !strings.ContainsAny(raw, "\r\n") && !strings.Contains(raw, " -> ") {
		return decodeSSHShare(raw)
	}
	binary := singbox.GetBinaryPath()
	if info, err := os.Stat(binary); err == nil && !info.IsDir() {
		return parseShareWithKernel(ctx, raw, binary)
	}
	out, err := parseLegacyNativeOutboundShareLink(raw)
	if err != nil {
		return nil, err
	}
	return &NativeShareImport{Outbounds: []map[string]any{out}, Endpoints: []map[string]any{}}, nil
}

func ParseNativeOutboundShareLink(raw string) (map[string]any, error) {
	parsed, err := ParseNativeOutboundImport(raw)
	if err != nil {
		return nil, err
	}
	if len(parsed.Outbounds)+len(parsed.Endpoints) != 1 {
		return nil, fmt.Errorf("share contains multiple connections; use batch import")
	}
	if len(parsed.Endpoints) == 1 {
		return parsed.Endpoints[0], nil
	}
	return parsed.Outbounds[0], nil
}

func isNativeImportEndpoint(kind string) bool {
	switch kind {
	case "wireguard", "awg", "warp", "tailscale", "openconnect", "openvpn-client", "openvpn-server", "masque-client", "masque-server", "tunnel_client", "tunnel_server":
		return true
	}
	return false
}

func decodeNativeShare(raw string) (*NativeShareImport, bool, error) {
	if strings.HasPrefix(raw, "singbox://") || strings.HasPrefix(raw, "sing-box://") {
		encoded := strings.SplitN(strings.SplitN(raw, "://", 2)[1], "#", 2)[0]
		var decoded []byte
		var err error
		for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
			decoded, err = encoding.DecodeString(encoded)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, true, fmt.Errorf("invalid native share encoding")
		}
		raw = string(decoded)
	} else if !strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[") {
		return nil, false, nil
	}
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return nil, true, fmt.Errorf("invalid native share JSON")
	}
	result := &NativeShareImport{Outbounds: []map[string]any{}, Endpoints: []map[string]any{}}
	add := func(v any, section string) error {
		item, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("connection must be a JSON object")
		}
		kind, _ := item["type"].(string)
		if strings.TrimSpace(kind) == "" {
			return fmt.Errorf("native connection requires type")
		}
		if kind == "hinvalid" {
			return fmt.Errorf("core returned an invalid connection placeholder")
		}
		if tag, _ := item["tag"].(string); strings.TrimSpace(tag) == "" {
			item["tag"] = kind
		}
		if section == "endpoints" || section == "" && isNativeImportEndpoint(kind) {
			result.Endpoints = append(result.Endpoints, item)
		} else {
			result.Outbounds = append(result.Outbounds, item)
		}
		return nil
	}
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if err := add(item, ""); err != nil {
				return nil, true, err
			}
		}
	case map[string]any:
		_, hasOut := v["outbounds"]
		_, hasEnd := v["endpoints"]
		if kind, _ := v["type"].(string); kind != "" || !hasOut && !hasEnd {
			if err := add(v, ""); err != nil {
				return nil, true, err
			}
		} else {
			for _, section := range []string{"outbounds", "endpoints"} {
				if v[section] == nil {
					continue
				}
				items, ok := v[section].([]any)
				if !ok {
					return nil, true, fmt.Errorf("%s must be an array", section)
				}
				for _, item := range items {
					if err := add(item, section); err != nil {
						return nil, true, err
					}
				}
			}
		}
	default:
		return nil, true, fmt.Errorf("native import requires a connection or configuration object")
	}
	if len(result.Outbounds)+len(result.Endpoints) == 0 {
		return nil, true, fmt.Errorf("no connections in import")
	}
	return result, true, nil
}

func parseShareWithKernel(parent context.Context, raw, binary string) (*NativeShareImport, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	if strings.ContainsAny(raw, "\r\n") || strings.Contains(raw, " -> ") {
		return parseNativeShareBatch(ctx, raw, binary)
	}
	// Only input files are passed to the CLI. A pasted URL cannot become a CLI
	// flag, executable, or path.
	temp, err := os.MkdirTemp("", "xui-share-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	input, output := filepath.Join(temp, "input.txt"), filepath.Join(temp, "output.json")
	kernelInput, nativeTransport, err := prepareNativeShareTransport(raw)
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(input, []byte(kernelInput), 0600); err != nil {
		return nil, err
	}
	if strings.HasPrefix(raw, "ssh://") && !strings.ContainsAny(raw, "\r\n") && !strings.Contains(raw, " -> ") {
		return decodeSSHShare(raw)
	}
	command := exec.CommandContext(ctx, binary, "parse", input, "--output", output)
	command.Dir = filepath.Dir(binary)
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("core share parsing timed out")
		}
		// Core diagnostics can contain credentials. Never relay or log them.
		return nil, fmt.Errorf("ядро не приняло ссылку: проверьте параметры и формат подключения")
	}
	info, err := os.Stat(output)
	if err != nil {
		return nil, err
	}
	if info.Size() > 4*maxNativeShareSize {
		return nil, fmt.Errorf("decoded configuration too large")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return nil, err
	}
	parsed, _, err := decodeNativeShare(string(data))
	if err != nil {
		return nil, err
	}
	// The kernel appends an internal counter to names; it has no UI meaning.
	for _, group := range [][]map[string]any{parsed.Outbounds, parsed.Endpoints} {
		for _, item := range group {
			// Keep counter suffixes for batches because detours reference these tags.
			if len(parsed.Outbounds)+len(parsed.Endpoints) == 1 {
				if tag, ok := item["tag"].(string); ok {
					item["tag"] = strings.TrimSuffix(tag, " § 0")
				}
			}
		}
	}
	if nativeTransport != nil {
		if len(parsed.Outbounds) != 1 || len(parsed.Endpoints) != 0 {
			return nil, fmt.Errorf("mKCP import requires one outbound")
		}
		out := parsed.Outbounds[0]
		out["transport"] = nativeTransport
		if mask, ok := out["final_mask"].(map[string]any); ok {
			if udp, exists := mask["udp"]; exists {
				nativeTransport["final_mask"] = map[string]any{"udp": udp}
				delete(mask, "udp")
				if len(mask) == 0 {
					delete(out, "final_mask")
				}
			}
		}
	}
	// Explicit native overlays retain options newer than the URI converter.
	if len(parsed.Outbounds)+len(parsed.Endpoints) == 1 {
		u, e := url.Parse(raw)
		if e == nil {
			target := parsed.Outbounds
			if len(target) == 0 {
				target = parsed.Endpoints
			}
			if err := applyNativeShareOptions(target[0], u.Query()); err != nil {
				return nil, err
			}
			for _, key := range []string{"options", "native"} {
				if extra := u.Query().Get(key); extra != "" {
					var fields map[string]any
					if json.Unmarshal([]byte(extra), &fields) != nil {
						return nil, fmt.Errorf("%s must be a native JSON object", key)
					}
					target := parsed.Outbounds
					if len(target) == 0 {
						target = parsed.Endpoints
					}
					kind := target[0]["type"]
					tag := target[0]["tag"]
					mergeNativeOptions(target[0], fields)
					target[0]["type"], target[0]["tag"] = kind, tag
				}
			}
		}
	}
	return parsed, nil
}

// Parse each batch node through the same adapters and option overlays as a
// single import. This prevents mKCP/SSH or newer native parameters being lost
// when a link appears inside a chain. Chain order matches the kernel parser.
func parseNativeShareBatch(ctx context.Context, raw, binary string) (*NativeShareImport, error) {
	result := &NativeShareImport{Outbounds: []map[string]any{}, Endpoints: []map[string]any{}}
	count := 0
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		chain := strings.Split(line, " -> ")
		detour := ""
		for i := len(chain) - 1; i >= 0; i-- {
			if count >= 128 {
				return nil, fmt.Errorf("import contains too many connections")
			}
			link := strings.TrimSpace(chain[i])
			parsed, handled, err := decodeNativeShare(link)
			if !handled {
				parsed, err = parseShareWithKernel(ctx, link, binary)
			}
			if err != nil {
				return nil, err
			}
			if len(chain) > 1 && len(parsed.Outbounds)+len(parsed.Endpoints) != 1 {
				return nil, fmt.Errorf("multi-server profiles cannot form a proxy chain")
			}
			names := map[string]string{}
			for _, group := range [][]map[string]any{parsed.Outbounds, parsed.Endpoints} {
				for _, item := range group {
					tag, _ := item["tag"].(string)
					if _, exists := names[tag]; exists {
						return nil, fmt.Errorf("duplicate connection names in profile")
					}
					names[tag] = fmt.Sprintf("%s § %d", tag, count)
					count++
				}
			}
			for _, group := range [][]map[string]any{parsed.Outbounds, parsed.Endpoints} {
				for _, item := range group {
					tag, _ := item["tag"].(string)
					retagNativeShareReferences(item, names)
					item["tag"] = names[tag]
					if detour != "" {
						item["detour"] = detour
					}
					detour = names[tag]
				}
			}
			result.Outbounds = append(result.Outbounds, parsed.Outbounds...)
			result.Endpoints = append(result.Endpoints, parsed.Endpoints...)
		}
	}
	if count == 0 {
		return nil, fmt.Errorf("no connections in import")
	}
	return result, nil
}

func retagNativeShareReferences(value any, names map[string]string) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			retagNativeShareReferences(item, names)
		}
	case map[string]any:
		for key, item := range v {
			switch key {
			case "detour", "default", "outbound", "final", "portal_tag", "bridge_tag":
				if old, ok := item.(string); ok && names[old] != "" {
					v[key] = names[old]
				}
			case "outbounds":
				if list, ok := item.([]any); ok {
					for i, member := range list {
						if old, ok := member.(string); ok && names[old] != "" {
							list[i] = names[old]
						} else {
							retagNativeShareReferences(member, names)
						}
					}
				}
			case "headers":
				// Headers contain arbitrary user data, not connection references.
			default:
				retagNativeShareReferences(item, names)
			}
		}
	}
}

func mergeNativeOptions(dst, src map[string]any) {
	for key, value := range src {
		if child, ok := value.(map[string]any); ok {
			if current, ok := dst[key].(map[string]any); ok {
				mergeNativeOptions(current, child)
				continue
			}
		}
		dst[key] = value
	}
}

// ray2sing currently emits an empty host-key entry for password-only SSH URLs.
// Native SSH supports password authentication with host_key omitted.
func decodeSSHShare(raw string) (*NativeShareImport, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User == nil {
		return nil, fmt.Errorf("invalid SSH connection")
	}
	port := 22
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid SSH port")
		}
	}
	tag := u.Fragment
	if tag == "" {
		tag = u.Hostname()
	}
	out := map[string]any{"type": "ssh", "tag": tag, "server": u.Hostname(), "server_port": port, "user": u.User.Username()}
	password, _ := u.User.Password()
	if password != "" {
		out["password"] = password
	}
	q := u.Query()
	for query, field := range map[string]string{"pk": "private_key", "private_key": "private_key", "private_key_passphrase": "private_key_passphrase", "client_version": "client_version", "detour": "detour"} {
		if q.Get(query) != "" {
			out[field] = q.Get(query)
		}
	}
	for query, field := range map[string]string{"hk": "host_key", "host_key": "host_key", "host_key_algorithms": "host_key_algorithms"} {
		if q.Get(query) != "" {
			out[field] = strings.Split(q.Get(query), ",")
		}
	}
	if mask := q.Get("fm"); mask != "" {
		var fields map[string]any
		if json.Unmarshal([]byte(mask), &fields) != nil {
			return nil, fmt.Errorf("invalid SSH final mask")
		}
		out["final_mask"] = fields
	}
	if err := applyNativeShareOptions(out, q); err != nil {
		return nil, err
	}
	for _, key := range []string{"options", "native"} {
		if q.Get(key) != "" {
			var fields map[string]any
			if json.Unmarshal([]byte(q.Get(key)), &fields) != nil {
				return nil, fmt.Errorf("invalid native SSH options")
			}
			mergeNativeOptions(out, fields)
		}
	}
	if !isMeaningfulSSHOption(out["password"]) && !isMeaningfulSSHOption(out["private_key"]) {
		return nil, fmt.Errorf("SSH requires password or private key")
	}
	out["type"], out["tag"] = "ssh", tag
	return &NativeShareImport{Outbounds: []map[string]any{out}, Endpoints: []map[string]any{}}, nil
}
func isMeaningfulSSHOption(value any) bool { s, ok := value.(string); return ok && s != "" }
