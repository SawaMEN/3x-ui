package sub

import "testing"

func TestIsIncyClient(t *testing.T) {
	for _, ua := range []string{"INCY/1.0.0/ios", "incy/1.0/android", "Mozilla/5.0 INCY/2.0"} {
		if !IsIncyClient(ua) {
			t.Errorf("INCY client not detected: %q", ua)
		}
	}
	for _, ua := range []string{"", "Happ/1.2.0", "Hiddify/3.0", "Mihomo", "Coincy/1.0"} {
		if IsIncyClient(ua) {
			t.Errorf("non-INCY client detected: %q", ua)
		}
	}
}
