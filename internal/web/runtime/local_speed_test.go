package runtime

import (
	"context"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestNativeSidecarLimitsRequestOneBatchedCoreApply(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.Sudoku} {
		t.Run(string(protocol), func(t *testing.T) {
			applies := 0
			local := NewLocal(LocalDeps{RequestConfigApply: func(context.Context) error { applies++; return nil }})
			ib := &model.Inbound{Id: 200, Protocol: protocol, Enable: true, Settings: `{"clients":[{"email":"alice","enable":true,"speedLimit":{"enabled":true,"uploadMbps":1}}]}`}
			ctx := context.Background()
			finish := local.BeginConfigBatch()
			if err := local.AddInbound(ctx, ib); err != nil {
				t.Fatal(err)
			}
			if err := local.AddClient(ctx, ib, model.Client{Enable: true, SpeedLimit: &model.ClientSpeedLimit{Enabled: true, UploadMbps: 1}}); err != nil {
				t.Fatal(err)
			}
			if err := local.UpdateUser(ctx, ib, "alice", model.Client{}); err != nil {
				t.Fatal(err)
			}
			if err := local.RemoveUser(ctx, ib, "alice"); err != nil {
				t.Fatal(err)
			}
			if err := local.DelInbound(ctx, ib); err != nil {
				t.Fatal(err)
			}
			if applies != 0 {
				t.Fatal("native speed plan applied before the DB batch completed")
			}
			if err := finish(); err != nil {
				t.Fatal(err)
			}
			if applies != 1 {
				t.Fatalf("sidecar speed plan applies=%d, want 1", applies)
			}
		})
	}
}

func TestNativeSidecarFirstLimitUsesClientPayload(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.Sudoku} {
		t.Run(string(protocol), func(t *testing.T) {
			applies := 0
			local := NewLocal(LocalDeps{RequestConfigApply: func(context.Context) error { applies++; return nil }})
			ib := &model.Inbound{Protocol: protocol, Enable: true}
			c := model.Client{Enable: true, SpeedLimit: &model.ClientSpeedLimit{Enabled: true, UploadMbps: 1}}
			if err := local.AddClient(context.Background(), ib, c); err != nil {
				t.Fatal(err)
			}
			if err := local.UpdateUser(context.Background(), ib, "alice", c); err != nil {
				t.Fatal(err)
			}
			if applies != 2 {
				t.Fatalf("first sidecar limit was saved without enforcement: applies=%d", applies)
			}
		})
	}
}
