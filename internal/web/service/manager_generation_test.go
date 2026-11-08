package service

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func TestManagerGeneratesWithoutSavedInbound(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote=%v", remote), func(t *testing.T) {
			s, template, _ := managerFixture(t)
			if err := database.GetDB().Delete(template).Error; err != nil {
				t.Fatal(err)
			}
			var request model.ManagerSubscription
			if err := json.Unmarshal([]byte(`{"name":"Automatic","enabled":true,"intervalMinutes":60,"entries":[{"mode":"generate","port":"random","config":{"protocol":"vless","settings":"{\"clients\":[],\"decryption\":\"none\"}","streamSettings":"{\"network\":\"tcp\",\"security\":\"none\"}","sniffing":"{}"}}]}`), &request); err != nil {
				t.Fatal(err)
			}
			if remote {
				node := &model.Node{Name: "generation-node", Address: "node.example.com", Port: 2053, ApiToken: "test-only", Enable: true, Status: "online"}
				if err := database.GetDB().Create(node).Error; err != nil {
					t.Fatal(err)
				}
				runtime.GetManager().SetRuntimeOverride(node.Id, &fakeNodeRuntime{})
				request.Entries[0].NodeID = &node.Id
			}
			sub, err := s.Save(1, request)
			if err != nil {
				t.Fatalf("automatic recipe rejected: %v", err)
			}
			if op := managerRun(t, s, sub); op.State != "success" {
				t.Fatal(op.Message)
			}
			if len(sub.Published) != 1 || sub.Published[0].ActiveInboundID == 0 || sub.ClientID == 0 {
				t.Fatalf("generation: %+v", sub)
			}
			if op := managerRun(t, s, sub); op.State != "success" {
				t.Fatalf("second automatic generation: %s", op.Message)
			}
		})
	}
}

type managerRemoteRestartFailure struct{ fakeNodeRuntime }

func (*managerRemoteRestartFailure) RestartXray(context.Context) error {
	return fmt.Errorf("remote core rejected generation")
}

func TestManagerConfirmsRemoteCoreBeforePublishing(t *testing.T) {
	s, _, sub := managerFixture(t)
	node := &model.Node{Name: "restart-node", Address: "node.example.com", Port: 2053, ApiToken: "test-only", Enable: true, Status: "online"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	runtime.GetManager().SetRuntimeOverride(node.Id, &fakeNodeRuntime{})
	sub.Entries[0].NodeID = &node.Id
	var err error
	sub, err = s.Save(1, *sub)
	if err != nil {
		t.Fatal(err)
	}
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	old := sub.Published[0].ActiveInboundID
	runtime.GetManager().SetRuntimeOverride(node.Id, &managerRemoteRestartFailure{})
	op := managerRun(t, s, sub)
	if op.State != "failed" {
		t.Fatalf("unconfirmed remote generation published: %+v", op)
	}
	if sub.Published[0].ActiveInboundID != old {
		t.Fatal("published generation changed after remote restart rejection")
	}
	if err := database.GetDB().First(&model.Inbound{}, old).Error; err != nil {
		t.Fatalf("working inbound removed: %v", err)
	}
}

func TestManagerRealityGenerationCreatesFreshConsistentKeys(t *testing.T) {
	s, template, _ := managerFixture(t)
	if err := database.GetDB().Delete(template).Error; err != nil {
		t.Fatal(err)
	}
	request := model.ManagerSubscription{Name: "Reality recipe", Enabled: true, IntervalMinutes: 60, Entries: []model.ManagerEntry{{Mode: "generate", Port: "random", Config: &model.ManagerInboundConfig{Protocol: model.VLESS, Settings: `{"clients":[],"decryption":"none"}`, StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"target":"www.example.com:443","serverNames":["www.example.com"],"privateKey":"","shortIds":[],"settings":{"publicKey":"","fingerprint":"firefox"}}}`, Sniffing: `{}`}}}}
	sub, err := s.Save(1, request)
	if err != nil {
		t.Fatal(err)
	}
	previous := ""
	for range 2 {
		if op := managerRun(t, s, sub); op.State != "success" {
			t.Fatal(op.Message)
		}
		ib, err := (&InboundService{}).GetInbound(sub.Published[0].ActiveInboundID)
		if err != nil {
			t.Fatal(err)
		}
		var stream struct {
			Reality struct {
				Private  string   `json:"privateKey"`
				ShortIDs []string `json:"shortIds"`
				Client   struct {
					Public string `json:"publicKey"`
				} `json:"settings"`
			} `json:"realitySettings"`
		}
		if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil {
			t.Fatal(err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(stream.Reality.Private)
		if err != nil {
			t.Fatal(err)
		}
		key, err := ecdh.X25519().NewPrivateKey(raw)
		if err != nil {
			t.Fatal(err)
		}
		if stream.Reality.Private == previous || len(stream.Reality.ShortIDs) != 1 || base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()) != stream.Reality.Client.Public {
			t.Fatalf("invalid or reused Reality identity: %+v", stream.Reality)
		}
		previous = stream.Reality.Private
	}
}

func TestManagerGeneratesVetroffOnNode(t *testing.T) {
	s, template, _ := managerFixture(t)
	if err := database.GetDB().Delete(template).Error; err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Name: "vetroff-node", Address: "node.example.com", Port: 2053, ApiToken: "test-only", Enable: true, Status: "online"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	runtime.GetManager().SetRuntimeOverride(node.Id, &fakeNodeRuntime{})
	sub, err := s.Save(1, model.ManagerSubscription{Name: "Vetroff node recipe", Enabled: true, IntervalMinutes: 60, Entries: []model.ManagerEntry{{Mode: "generate", Port: "random", NodeID: &node.Id, Config: &model.ManagerInboundConfig{Protocol: model.VetroffProxy, Settings: `{"hostname":"vpn.example.com","certificate":"/etc/cert/fullchain.pem","privateKey":"/etc/cert/privkey.pem","clients":[]}`, StreamSettings: `{}`, Sniffing: `{}`}}}})
	if err != nil {
		t.Fatal(err)
	}
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	ib, err := (&InboundService{}).GetInbound(sub.Published[0].ActiveInboundID)
	if err != nil {
		t.Fatal(err)
	}
	if ib.Protocol != model.VetroffProxy || ib.NodeID == nil || *ib.NodeID != node.Id {
		t.Fatalf("wrong deployment: %+v", ib)
	}
}

func TestManagerOneBundleGeneratesDifferentProtocolsAndSettings(t *testing.T) {
	s, template, _ := managerFixture(t)
	if err := database.GetDB().Delete(template).Error; err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Name: "mixed-node", Address: "node.example.com", Port: 2053, ApiToken: "test-only", Enable: true, Status: "online"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	runtime.GetManager().SetRuntimeOverride(node.Id, &fakeNodeRuntime{})
	sub, err := s.Save(1, model.ManagerSubscription{Name: "Mixed automatic", Enabled: true, IntervalMinutes: 60, Entries: []model.ManagerEntry{
		{Mode: "generate", Port: "random", Config: &model.ManagerInboundConfig{Protocol: model.VLESS, Settings: `{"clients":[],"decryption":"none"}`, StreamSettings: `{"network":"tcp","security":"none"}`, Sniffing: `{}`}},
		{Mode: "generate", Port: "random", NodeID: &node.Id, Config: &model.ManagerInboundConfig{Protocol: model.VMESS, ShareAddrStrategy: "custom", ShareAddr: "public-node.example.com", SubSortIndex: 7, Settings: `{"clients":[]}`, StreamSettings: `{"network":"ws","security":"none","wsSettings":{"path":"/independent-path","host":"edge.example.com"}}`, Sniffing: `{}`}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if op := managerRun(t, s, sub); op.State != "success" {
			t.Fatal(op.Message)
		}
		if len(sub.Published) != 2 {
			t.Fatalf("published entries: %+v", sub.Published)
		}
		local, err := (&InboundService{}).GetInbound(sub.Published[0].ActiveInboundID)
		if err != nil {
			t.Fatal(err)
		}
		remote, err := (&InboundService{}).GetInbound(sub.Published[1].ActiveInboundID)
		if err != nil {
			t.Fatal(err)
		}
		if local.Protocol != model.VLESS || local.NodeID != nil || remote.Protocol != model.VMESS || remote.NodeID == nil || *remote.NodeID != node.Id {
			t.Fatalf("wrong protocol or deployment: %+v %+v", local, remote)
		}
		var stream struct {
			Network string `json:"network"`
			WS      struct {
				Path string `json:"path"`
				Host string `json:"host"`
			} `json:"wsSettings"`
		}
		if err := json.Unmarshal([]byte(remote.StreamSettings), &stream); err != nil {
			t.Fatal(err)
		}
		if remote.ShareAddrStrategy != "custom" || remote.ShareAddr != "public-node.example.com" || remote.SubSortIndex != 7 {
			t.Fatalf("endpoint settings lost: %+v", remote)
		}
		if stream.Network != "ws" || stream.WS.Path != "/independent-path" || stream.WS.Host != "edge.example.com" {
			t.Fatalf("entry settings lost: %+v", stream)
		}
	}
}
