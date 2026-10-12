package externalvpn

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func openFluxPlan(tag, secret string) map[string]any {
	return map[string]any{"tag": tag, "protocol": "openflux", "settings": map[string]any{"secret": strings.Repeat(secret, 64), "context": "test", "transports": []any{map[string]any{"type": "direct", "dial": "example.com:443", "priority": 100}}}}
}

func TestOutboundPlanPreservesLiveChildAndRollbackRestoresState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := os.WriteFile(filepath.Join(dir, "openflux"), []byte("#!/bin/sh\nif grep -q 'SessionContext = broken' \"$2\"; then exit 3; fi\nexec sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	oldPlans, oldProcesses := outboundPlans, outboundProcesses
	outboundPlans = map[string]*managedOutbound{}
	outboundProcesses = map[string]*managedOutbound{}
	t.Cleanup(func() { KeepOutbounds(nil); outboundPlans, outboundProcesses = oldPlans, oldProcesses })
	raw := openFluxPlan("plan-test", "a")
	oldBridge, err := EnsureOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	original := outboundProcesses["plan-test"]
	if !OutboundsApplied([]map[string]any{raw}) {
		t.Fatal("live sidecar not recognized")
	}
	changed := openFluxPlan("plan-test", "b")
	if OutboundsApplied([]map[string]any{changed}) {
		t.Fatal("stable bridge hid changed sidecar credentials")
	}
	preview, err := PlanOutbound(changed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(preview, oldBridge) || outboundProcesses["plan-test"] != original {
		t.Fatal("planning altered the live child or bridge identity")
	}
	select {
	case <-original.done:
		t.Fatal("preview stopped child")
	default:
	}
	again, err := PlanOutbound(changed)
	if err != nil || !reflect.DeepEqual(again, preview) {
		t.Fatal("preview is unstable")
	}
	invalid := openFluxPlan("invalid-plan", "x")
	invalid["settings"].(map[string]any)["secret"] = "invalid"
	if _, err := ApplyOutbounds([]map[string]any{changed, invalid}); err == nil {
		t.Fatal("invalid desired sidecar accepted")
	}
	restored := outboundProcesses["plan-test"]
	if restored == nil || !reflect.DeepEqual(restored.raw, original.raw) {
		t.Fatal("partial apply did not restore previous sidecar settings")
	}
	select {
	case <-restored.done:
		t.Fatal("rollback left child stopped")
	default:
	}
	finish, err := ApplyOutbounds([]map[string]any{changed})
	if err != nil {
		t.Fatal(err)
	}
	if err := finish(false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(outboundProcesses["plan-test"].raw, original.raw) {
		t.Fatal("core failure did not roll back sidecar")
	}
	broken := openFluxPlan("plan-test", "b")
	broken["settings"].(map[string]any)["context"] = "broken"
	if _, err := PlanOutbound(broken); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyOutbounds([]map[string]any{broken}); err == nil {
		t.Fatal("immediate child exit was reported as a successful apply")
	}
	if !reflect.DeepEqual(outboundProcesses["plan-test"].raw, original.raw) {
		t.Fatal("early startup failure did not restore sidecar")
	}
	// A successful configuration without the old tag retires it at commit only.
	finish, err = ApplyOutbounds(nil)
	if err != nil {
		t.Fatal(err)
	}
	if outboundProcesses["plan-test"] == nil {
		t.Fatal("preparation retired live sidecar")
	}
	if err := finish(true); err != nil {
		t.Fatal(err)
	}
	if outboundProcesses["plan-test"] != nil {
		t.Fatal("commit kept obsolete sidecar")
	}
}
