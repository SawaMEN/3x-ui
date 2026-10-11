package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/core"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
)

func TestOwnerSingBoxCanonicalIdentity(t *testing.T) {
	setupBulkDB(t)
	settings := &SettingService{}
	// Read compatibility must also work before an old database is saved again.
	if err := settings.setString("coreType", "hiddify-core"); err != nil {
		t.Fatal(err)
	}
	kind, err := settings.GetCoreType()
	if err != nil || kind != "sing-box" {
		t.Fatalf("type=%q err=%v", kind, err)
	}
	all, err := settings.GetAllSetting()
	if err != nil || all.CoreType != "sing-box" {
		t.Fatalf("settings=%#v err=%v", all, err)
	}
	for _, input := range []string{"sing-box", "hiddify-core"} {
		if err := settings.SetCoreType(input); err != nil {
			t.Fatal(err)
		}
		stored, err := settings.getString("coreType")
		if err != nil || stored != "sing-box" {
			t.Fatalf("persisted=%q err=%v", stored, err)
		}
		runtime := NativeCore(input)
		if runtime.process() != hiddifyProcess || runtime.BinaryPath() != hiddify.GetBinaryPath() || runtime.CoreType() != "sing-box" {
			t.Fatalf("wrong adapter for %q", input)
		}

		if !coreSupportsInboundProtocol(kind, model.Tun) {
			t.Fatal("owner TUN extension lost")
		}
	}
	if err := settings.SetCoreType(CoreTypeXray); err == nil {
		t.Fatal("removed Xray engine remains selectable")
	}
	if hiddify.NewRuntime("", true).Type() != core.SingBox {
		t.Fatal("runtime exposes wrong identity")
	}
}
