package masque

import "testing"

func TestAdvancedOptionsCannotOverrideManagedFields(t *testing.T) {
	for _, data := range []string{
		`{"advanced":{"users":[]}}`,
		`{"advanced":{"tls":{"enabled":false}}}`,
		`{"advanced":{"udp_timeout":"never"}}`,
		`{"advanced":{"initial_packet_size":100}}`,
		`{"advanced":{"stream_receive_window":"bad-size"}}`,
	} {
		if _, err := Parse(data); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	settings, err := Parse(`{"advanced":{"udp_timeout":"5m","bind_interface":"","idle_timeout":"30s","initial_packet_size":1200,"disable_path_mtu_discovery":true}}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := settings.Advanced["bind_interface"]; exists {
		t.Fatal("empty interface was retained")
	}
	if settings.Advanced["initial_packet_size"] != float64(1200) {
		t.Fatal(settings.Advanced)
	}
}

func TestMemoryWindowsMatchKernelFormat(t *testing.T) {
	for _, value := range []any{"64 MB", "32kb", "1024 B", float64(1048576), float64(0)} {
		if !validMemorySize(value) {
			t.Errorf("rejected supported window %#v", value)
		}
	}
	for _, value := range []any{"1.5 MB", "64 MiB", "64", "18446744073709551616 B", "16 EB", float64(-1), float64(1.5)} {
		if validMemorySize(value) {
			t.Errorf("accepted invalid window %#v", value)
		}
	}
}
