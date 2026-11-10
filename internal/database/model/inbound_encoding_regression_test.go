package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenInboundConfigQuotesListenAsJSON(t *testing.T) {
	for _, listen := range []string{"127.0.0.1", "::1", `@/run/x-ui/"edge".sock`, "@/run/x-ui/back\\slash.sock", "@/run/x-ui/line\nbreak.sock"} {
		cfg := (&Inbound{Protocol: VLESS, Listen: listen}).GenInboundConfig()
		var decoded string
		if err := json.Unmarshal(cfg.Listen, &decoded); err != nil {
			t.Errorf("listen value is not valid JSON: %v", err)
		} else if decoded != listen {
			t.Error("listen value changed after JSON encoding")
		}
	}
}

func TestHealMtprotoRawSecretsPreserveMarkerLikeKeyBytes(t *testing.T) {
	for _, raw := range []string{"ee112233445566778899aabbccddeeff", "dd112233445566778899aabbccddeeff"} {
		input := `{"fakeTlsDomain":"edge.example","clients":[{"secret":"` + raw + `"}]}`
		out, changed := HealMtprotoClientSecrets(input)
		if !changed {
			t.Fatal("raw legacy secret must gain the FakeTLS wrapper")
		}
		var parsed struct {
			Clients []struct {
				Secret string `json:"secret"`
			} `json:"clients"`
		}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatal(err)
		}
		if len(parsed.Clients) != 1 || !strings.HasPrefix(parsed.Clients[0].Secret, "ee"+raw) {
			t.Error("raw MTProto key rotated because its first byte resembles a wrapper")
		}
		if again, changed := HealMtprotoClientSecrets(out); changed || again != out {
			t.Error("healed key must remain stable")
		}
	}
}
