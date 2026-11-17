package hiddify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// takeProtectionExtensions extracts only fields implemented by the pinned modified core.
// Stock sing-box conversion remains strict and never silently accepts these options.
func takeProtectionExtensions(settings, stream map[string]any, protocol string, inbound bool) (map[string]any, map[string]any, error) {
	vision := map[string]any{}
	reality := map[string]any{}
	source := settings
	if !inbound {
		source = vlessUser(settings)
	}
	if protocol == "vless" {
		if value, exists := source["testseed"]; exists && meaningful(value) {
			data, _ := json.Marshal(value)
			var seed []uint32
			if json.Unmarshal(data, &seed) != nil || len(seed) != 4 || seed[0] == 0 || seed[1] == 0 || seed[3] == 0 || seed[2] < seed[0]-1 || uint64(seed[1])+uint64(seed[2])-1 > 65535 || seed[3] > 65536 {
				return nil, nil, fmt.Errorf("invalid Vision testseed: four integers producing uint16 padding required")
			}
			vision["testseed"] = seed
		}
		delete(source, "testseed")
		if !inbound {
			if value, exists := source["testpre"]; exists && meaningful(value) {
				data, _ := json.Marshal(value)
				var count uint32
				if json.Unmarshal(data, &count) != nil || count > 64 {
					return nil, nil, fmt.Errorf("Vision testpre must be an integer from 0 to 64")
				}
				if source["flow"] != "xtls-rprx-vision" {
					return nil, nil, fmt.Errorf("testpre requires Vision flow")
				}
				vision["testpre"] = count
			}
			delete(source, "testpre")
		}
	}
	r := object(stream, "realitySettings")
	if stream["security"] == "reality" {
		key, native, size := "mldsa65Verify", "mldsa65_verify", 1952
		if inbound {
			key, native, size = "mldsa65Seed", "mldsa65_seed", 32
		}
		if value, exists := r[key]; exists && meaningful(value) {
			encoded, ok := value.(string)
			decoded, err := base64.RawURLEncoding.DecodeString(encoded)
			if !ok || err != nil || len(decoded) != size {
				return nil, nil, fmt.Errorf("invalid REALITY %s", key)
			}
			if inbound && encoded == r["privateKey"] {
				return nil, nil, fmt.Errorf("REALITY ML-DSA seed must differ from private key")
			}
			reality[native] = encoded
		}
		delete(r, key)
		if inbound {
			for key, native := range map[string]string{"minClientVer": "min_client_version", "maxClientVer": "max_client_version", "xver": "xver"} {
				if value, exists := r[key]; exists && meaningful(value) {
					reality[native] = value
				}
				delete(r, key)
			}
			if names, exists := r["serverNames"]; exists {
				reality["server_names"] = names
			}
			for key, native := range map[string]string{"limitFallbackUpload": "limit_fallback_upload", "limitFallbackDownload": "limit_fallback_download"} {
				if value, exists := r[key]; exists && meaningful(value) {
					fields, ok := value.(map[string]any)
					if !ok {
						return nil, nil, fmt.Errorf("REALITY %s must be an object", key)
					}
					mapped := map[string]any{}
					for field, nativeField := range map[string]string{"afterBytes": "after_bytes", "bytesPerSec": "bytes_per_sec", "burstBytesPerSec": "burst_bytes_per_sec"} {
						if value, exists := fields[field]; exists {
							data, _ := json.Marshal(value)
							var number int64
							if json.Unmarshal(data, &number) != nil || number < 0 {
								return nil, nil, fmt.Errorf("invalid REALITY %s.%s", key, field)
							}
							mapped[nativeField] = number
						}
					}
					for field, value := range fields {
						if field != "afterBytes" && field != "bytesPerSec" && field != "burstBytesPerSec" && meaningful(value) {
							return nil, nil, fmt.Errorf("unknown REALITY %s.%s", key, field)
						}
					}
					reality[native] = mapped
				}
				delete(r, key)
			}
		}
	}
	return vision, reality, nil
}
