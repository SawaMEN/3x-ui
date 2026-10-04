package gateway

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestGatewayDatabasePoolAndInactiveXrayTemplate(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "gateway.db")); err != nil {
		t.Fatal(err)
	}
	pool := database.GetDB()
	for i := 0; i < 3; i++ {
		if err := ensureDatabase(); err != nil {
			t.Fatal(err)
		}
		if database.GetDB() != pool {
			t.Fatal("Gateway replaced the running panel database pool")
		}
	}
	settings := &service.SettingService{}
	if err := settings.SetCoreType(service.CoreTypeSingBox); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "inbounds": []any{}}
	if err := saveTemplate(cfg); err != nil {
		t.Fatal("saving inactive Xray template used sing-box validation:", err)
	}
}
func TestGatewayMigrationAndRollback(t *testing.T) {
	states := map[string]State{"old": {Enabled: true, Configured: true}, "new": {}}
	state := func(core string) (State, error) { return states[core], nil }
	enable := func(core string) error { states[core] = State{Enabled: true, Configured: true}; return nil }
	disable := func(core string) error { states[core] = State{}; return nil }
	undo, err := migrateCore("old", "new", state, enable, disable, enable)
	if err != nil {
		t.Fatal(err)
	}
	if states["old"].Enabled || !states["new"].Configured {
		t.Fatal("migration left incorrect ownership")
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	if !states["old"].Configured || states["new"].Enabled {
		t.Fatal("rollback did not restore ownership")
	}
	failDisable := func(core string) error {
		if core == "old" {
			states[core] = State{}
			return errors.New("old save failed after removing listener")
		}
		return disable(core)
	}
	if _, err := migrateCore("old", "new", state, enable, failDisable, enable); err == nil {
		t.Fatal("migration ignored save failure")
	}
	if !states["old"].Configured || states["new"].Enabled {
		t.Fatal("failed migration leaked target listener")
	}
	states["old"] = State{Enabled: true}
	if _, err := migrateCore("old", "new", state, enable, disable, enable); err == nil {
		t.Fatal("partial Gateway migrated")
	}
}
func TestGatewayMigrationSkipsInactiveTargetInspection(t *testing.T) {
	state := func(core string) (State, error) {
		if core == "new" {
			return State{}, errors.New("unused template malformed")
		}
		return State{}, nil
	}
	if _, err := migrateCore("old", "new", state, nil, nil, nil); err != nil {
		t.Fatal("disabled Gateway prevented normal core switch:", err)
	}
}
func TestGatewayPortRangesAndLegacyRecovery(t *testing.T) {
	for _, port := range []any{52345, "52345", "52340-52350", "1080,52345", "443,52300-52400"} {
		cfg := map[string]any{"inbounds": []any{map[string]any{"tag": "user", "port": port}}}
		if err := validateXrayGatewayConflicts(cfg); err == nil || !strings.Contains(err.Error(), "port") {
			t.Fatalf("port conflict ignored: %v", port)
		}
	}
	inbound := gatewayInbound()
	inbound["listen"] = "127.0.0.1"
	cfg := map[string]any{"inbounds": []any{inbound}}
	if hasUsableXrayGatewayInbound(cfg) || !hasGatewayArtifacts(cfg) {
		t.Fatal("loopback Gateway was presented as usable or lost cleanup ownership")
	}
}
func TestSingBoxEmptyBackupDoesNotEraseOperatorChanges(t *testing.T) {
	for _, cfg := range []map[string]any{{"outbounds": []any{map[string]any{"tag": "added", "type": "direct"}}}, {"route": map[string]any{"final": "added"}}, {"log": map[string]any{"level": "debug"}}} {
		if isEmptySingBoxGatewayRemainder(cfg) {
			t.Fatalf("operator changes would be replaced by empty backup: %#v", cfg)
		}
	}
	if !isEmptySingBoxGatewayRemainder(map[string]any{"inbounds": []any{}, "route": map[string]any{}}) {
		t.Fatal("empty generated sections not recognized")
	}
}
