package singbox

import (
	"fmt"
	"math"
	"strconv"
)

// Panel numeric options must survive translation without rounding or overflow.
// Native JSON is validated by its own schema; string-valued ports from older
// panel exports remain accepted when they encode an exact integer.
func ValidatePanelIntegerOptions(raw map[string]any) error {
	limits := map[string]int64{"port": 65535, "serverPort": 65535, "server_port": 65535, "alterId": 65535, "mtu": 65535, "keepAlive": 65535, "maxEarlyData": math.MaxUint32, "maxTimeDiff": math.MaxInt64 / 1_000_000, "udpIdleTimeout": math.MaxInt64 / 1_000_000_000, "up_mbps": math.MaxInt64 / 1_000_000, "down_mbps": math.MaxInt64 / 1_000_000, "minPacketSize": 65535, "maxPacketSize": 65535}
	var visit func(any, string) error
	visit = func(value any, path string) error {
		switch object := value.(type) {
		case map[string]any:
			for key, value := range object {
				field := path + "." + key
				if max, ok := limits[key]; ok && value != nil {
					var number int64
					switch v := value.(type) {
					case int:
						number = int64(v)
					case int64:
						number = v
					case float64:
						if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v < 0 || v > float64(max) {
							return fmt.Errorf("%s must be an integer between 0 and %d", field, max)
						}
						number = int64(v)
					case string:
						var err error
						number, err = strconv.ParseInt(v, 10, 64)
						if err != nil {
							return fmt.Errorf("%s must be an integer", field)
						}
					default:
						return fmt.Errorf("%s must be an integer", field)
					}
					if number < 0 || number > max {
						return fmt.Errorf("%s must be between 0 and %d", field, max)
					}
				}
				if err := visit(value, field); err != nil {
					return err
				}
			}
		case []any:
			for i, item := range object {
				if err := visit(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(raw, "config")
}
