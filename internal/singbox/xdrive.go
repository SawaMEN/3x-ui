package singbox

import (
	"encoding/json"
	"fmt"
	"strings"
)

// translateXDrive maps the panel transport to the native storage stream. TLS is
// supplied by each HTTPS storage request, never wrapped around the VLESS stream.
func translateXDrive(out map[string]any, protocol string, stream map[string]any, inbound bool) error {
	if protocol != "vless" {
		return fmt.Errorf("XDrive requires VLESS")
	}
	if security := rawString(stream, "security"); security != "" && security != "none" {
		return fmt.Errorf("XDrive requires security none and VLESS encryption")
	}
	for _, value := range rawObject(stream, "finalmask") {
		if isMeaningfulCompatValue(value) {
			return fmt.Errorf("XDrive transport finalmask has no native storage mapping")
		}
	}
	key := "encryption"
	if inbound {
		key = "decryption"
	}
	if value := rawString(out, key); value == "" || value == "none" {
		return fmt.Errorf("XDrive requires VLESS %s", key)
	}
	if rawString(out, "flow") != "" {
		return fmt.Errorf("XDrive does not support VLESS flow")
	}
	if users, ok := out["users"].([]map[string]any); ok {
		for _, user := range users {
			if rawString(user, "flow") != "" {
				return fmt.Errorf("XDrive does not support VLESS flow")
			}
		}
	}
	xd := rawObject(stream, "xdriveSettings")
	service := rawString(xd, "service")
	if service == "Google Drive" {
		service = "google-drive"
	}
	folder := strings.TrimSpace(rawString(xd, "remoteFolder"))
	if folder == "" {
		return fmt.Errorf("XDrive remoteFolder is required")
	}
	transport := map[string]any{"type": "xdrive", "service": service, "remote_folder": folder}
	if front := rawString(stream, "address"); front != "" {
		transport["front_address"] = front
	}
	if port := rawInt(stream, "port"); port > 0 {
		transport["front_port"] = port
	}
	secrets := rawStrings(xd, "secrets")
	switch service {
	case "local":
	case "google-drive":
		if len(secrets) != 3 {
			return fmt.Errorf("XDrive Google Drive needs three secrets")
		}
		for _, value := range secrets {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("XDrive Google Drive secrets must not be empty")
			}
		}
	case "template":
		template := rawObject(xd, "template")
		if len(template) == 0 {
			return fmt.Errorf("XDrive needs an HTTP storage template")
		}
		encoded, err := json.Marshal(template)
		if err != nil {
			return err
		}
		transport["template"] = string(encoded)
	case "s3":
		s3 := rawObject(xd, "s3")
		if rawString(s3, "endpoint") == "" || rawString(s3, "bucket") == "" || len(secrets) < 2 || len(secrets) > 3 {
			return fmt.Errorf("XDrive S3 needs endpoint, bucket and access credentials")
		}
		transport["s3"] = map[string]any{"endpoint": s3["endpoint"], "region": s3["region"], "bucket": s3["bucket"], "path_style": s3["pathStyle"]}
	default:
		return fmt.Errorf("unsupported XDrive service %q", service)
	}
	if len(secrets) > 0 {
		transport["secrets"] = secrets
	}
	for from, to := range map[string]string{"segmentBytes": "segment_bytes", "flushIntervalMs": "flush_interval_ms", "pollIntervalMs": "poll_interval_ms", "maxPollIntervalMs": "max_poll_interval_ms", "sessionTtlSeconds": "session_ttl_seconds", "concurrency": "concurrency", "eagerWindowMs": "eager_window_ms", "holeTimeoutMs": "hole_timeout_ms"} {
		if value, exists := xd[from]; exists {
			n, ok := value.(float64)
			if !ok || n < 0 || n > 4294967295 || n != float64(uint32(n)) {
				return fmt.Errorf("invalid XDrive %s", from)
			}
			if from == "segmentBytes" && n > 16<<20 || from == "concurrency" && n > 64 {
				return fmt.Errorf("XDrive %s exceeds core limit", from)
			}
			transport[to] = value
		}
	}
	if xmux := rawObject(xd, "xmux"); len(xmux) > 0 {
		normalized := map[string]any{}
		for from, to := range map[string]string{"maxConcurrency": "max_concurrency", "maxConnections": "max_connections", "cMaxReuseTimes": "c_max_reuse_times", "hMaxRequestTimes": "h_max_request_times", "hMaxReusableSecs": "h_max_reusable_secs", "hKeepAlivePeriod": "h_keep_alive_period"} {
			if value, ok := xmux[from]; ok {
				normalized[to] = value
			}
		}
		transport["xmux"] = normalized
	}
	if inbound {
		delete(out, "listen")
		delete(out, "listen_port")
	}
	out["transport"] = transport
	return nil
}
