package model

import "testing"

func TestHealHysteriaVersionLeavesNonObjects(t *testing.T) {
	for _, raw := range []string{"null", " null ", "[]", "123", `"settings"`} {
		t.Run(raw, func(t *testing.T) {
			got, changed := HealHysteriaVersion(raw)
			if changed || got != raw {
				t.Fatalf("non-object settings must be unchanged: changed=%v", changed)
			}
			in := Inbound{Protocol: Hysteria, Settings: raw, StreamSettings: "null"}
			if cfg := in.GenInboundConfig(); string(cfg.Settings) != raw {
				t.Fatal("generating an inbound changed non-object settings")
			}
		})
	}
}
