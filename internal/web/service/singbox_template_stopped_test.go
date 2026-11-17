package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
)

func TestStoppedNativeTemplateRejectsAndRollsBack(t *testing.T) {
	svc, _, _ := stage5ProcessFixture(t)
	if err := svc.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	old, err := singBoxSettingService.GetSingBoxConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveTemplate(context.Background(), `{"log":{"level":"stage-audit-reject"}}`); err == nil || !strings.Contains(err.Error(), "candidate rejected") {
		t.Fatalf("unvalidated stopped template: %v", err)
	}
	after, err := singBoxSettingService.GetSingBoxConfigTemplate()
	if err != nil || after != old {
		t.Fatalf("template rollback failed: %v", err)
	}
	data, err := os.ReadFile(hiddify.GetConfigPath())
	if err != nil || strings.Contains(string(data), "stage-audit-reject") {
		t.Fatalf("candidate rollback failed: %v", err)
	}
	if svc.IsRunning() {
		t.Fatal("save unexpectedly started a stopped core")
	}
}

func TestStoppedSharedTemplateChecksCandidateBeforeStoring(t *testing.T) {
	native, _, _ := stage5ProcessFixture(t)
	if err := native.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	old, err := singBoxSettingService.GetXrayConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := os.ReadFile(hiddify.GetConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	svc := &XraySettingService{}
	raw := `{"outbounds":[{"protocol":"freedom","tag":"direct"},{"protocol":"blackhole","tag":"blocked"},{"type":"direct","tag":"stage-audit-reject"}]}`
	if err := svc.SaveValidatedXraySetting(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "candidate rejected") {
		t.Fatalf("unchecked shared template: %v", err)
	}
	stored, err := singBoxSettingService.GetXrayConfigTemplate()
	if err != nil || stored != old {
		t.Fatalf("invalid shared template stored: %v", err)
	}
	current, err := os.ReadFile(hiddify.GetConfigPath())
	if err != nil || string(current) != string(cfg) {
		t.Fatalf("active config modified: %v", err)
	}
	if native.IsRunning() {
		t.Fatal("saving template started stopped core")
	}
}
