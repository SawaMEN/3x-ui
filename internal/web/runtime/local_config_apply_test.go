package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestLocalConfigApplyPreservesCoreAndProtocolDispatch(t *testing.T) {
	for _, core := range []string{"sing-box", "hiddify-core", "xray", "sing-box-upstream"} {
		for _, protocol := range []model.Protocol{model.VLESS, model.TUIC} {
			t.Run(fmt.Sprintf("%s/%s", core, protocol), func(t *testing.T) {
				calls := 0
				local := NewLocal(LocalDeps{
					CoreType:           func() string { return core },
					APIPort:            func() int { t.Error("config apply used the removed direct API"); return -1 },
					RequestConfigApply: func(context.Context) error { calls++; return nil },
				})
				ctx := context.Background()
				ib := &model.Inbound{Id: 950001, Protocol: protocol, Tag: "apply-test", Enable: true, Settings: `{"clients":[]}`}
				if !local.UsesNativeConfig() {
					t.Fatal("local mutations must use config apply")
				}
				if err := local.AddInbound(ctx, ib); err != nil {
					t.Fatal(err)
				}
				if err := local.AddClient(ctx, ib, model.Client{Email: "alice", ID: "test-id", Enable: true}); err != nil {
					t.Fatal(err)
				}
				if err := local.UpdateUser(ctx, ib, "alice", model.Client{Email: "renamed", Enable: false}); err != nil {
					t.Fatal(err)
				}
				if err := local.UpdateInbound(ctx, ib, ib); err != nil {
					t.Fatal(err)
				}
				if err := local.DelInbound(ctx, ib); err != nil {
					t.Fatal(err)
				}
				if calls != 5 {
					t.Fatalf("got %d config applies, want 5", calls)
				}
			})
		}
	}
}

func BenchmarkLocalAddClient(b *testing.B) {
	local := NewLocal(LocalDeps{RequestConfigApply: func(context.Context) error { return nil }})
	ib := &model.Inbound{Protocol: model.VLESS, Enable: true}
	client := model.Client{Email: "bench@example.invalid", ID: "test-id", Enable: true}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := local.AddClient(ctx, ib, client); err != nil {
			b.Fatal(err)
		}
	}
}
