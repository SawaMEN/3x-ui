//go:build !windows

package web

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func TestSidecarRejectsStaleConfiguration(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.MTProto, model.TrustTunnel} {
		for _, change := range []string{"credentials", "manual-disable", "quota-disable"} {
			for _, operation := range []string{"ensure", "reconcile"} {
				t.Run(string(protocol)+"/"+change+"/"+operation, func(t *testing.T) {
					dir := t.TempDir()
					t.Setenv("XUI_BIN_FOLDER", dir)
					t.Setenv("XUI_DB_FOLDER", dir)
					dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
					db := database.GetDB()
					clients := []map[string]any{{"email": "first", "enable": true, "password": "old-password", "secret": "0123456789abcdef0123456789abcdef"}, {"email": "second", "enable": true, "password": "second-password", "secret": "abcdef0123456789abcdef0123456789"}}
					settings := map[string]any{"clients": clients, "hostname": "fixture.example.test"}
					encode := func() string {
						data, err := json.Marshal(settings)
						if err != nil {
							t.Fatal(err)
						}
						return string(data)
					}
					ib := model.Inbound{Id: 998, Tag: "revision", Enable: true, Protocol: protocol, Listen: "127.0.0.1", Port: 34584, Settings: encode()}
					if err := db.Create(&ib).Error; err != nil {
						t.Fatal(err)
					}
					row := coremodel.ClientTraffic{InboundId: ib.Id, Email: "first", Enable: true}
					if err := db.Create(&row).Error; err != nil {
						t.Fatal(err)
					}
					var start func(*model.Inbound, bool) error
					var replay func()
					var stop func()
					var configPath string
					script := []byte("#!/bin/sh\ncase \"$1\" in\nget|reload) exit 0;;\n*) exec sleep 60;;\nesac\n")
					switch protocol {
					case model.Mieru:
						if err := os.WriteFile(filepath.Join(dir, "mita"), script, 0o700); err != nil {
							t.Fatal(err)
						}
						mgr := mieru.GetManager()
						mgr.StopAll()
						stop = mgr.StopAll
						old, ok := mieru.InstanceFromInbound(&ib)
						if !ok {
							t.Fatal("invalid fixture")
						}
						start = func(current *model.Inbound, quota bool) error {
							inst, ok := mieru.InstanceFromInbound(current)
							if !ok {
								t.Fatal("invalid current fixture")
							}
							if quota {
								inst.ClientRevision = sidecartraffic.DisabledRevision(map[string]bool{"first": true})
								inst.Users = inst.Users[1:]
							}
							return mgr.Ensure(inst)
						}
						replay = func() { mgr.Reconcile([]mieru.Instance{old}) }
						configPath = filepath.Join(dir, "mieru", "mita-998.json")
					case model.MTProto:
						if err := os.WriteFile(filepath.Join(dir, mtproto.GetBinaryName()), script, 0o700); err != nil {
							t.Fatal(err)
						}
						mgr := mtproto.GetManager()
						mgr.StopAll()
						stop = mgr.StopAll
						old, ok := mtproto.InstanceFromInbound(&ib)
						if !ok {
							t.Fatal("invalid fixture")
						}
						start = func(current *model.Inbound, quota bool) error {
							inst, ok := mtproto.InstanceFromInbound(current)
							if !ok {
								t.Fatal("invalid current fixture")
							}
							if quota {
								inst.ClientRevision = sidecartraffic.DisabledRevision(map[string]bool{"first": true})
								inst.Secrets = inst.Secrets[1:]
							}
							return mgr.Ensure(inst)
						}
						replay = func() { mgr.Reconcile([]mtproto.Instance{old}) }
						configPath = filepath.Join(dir, "mtproto", "telemt-998.toml")
					default:
						if err := os.WriteFile(filepath.Join(dir, "trusttunnel_endpoint"), script, 0o700); err != nil {
							t.Fatal(err)
						}
						mgr := externalvpn.GetManager()
						mgr.StopAll()
						stop = mgr.StopAll
						old, err := externalvpn.FromInbound(&ib)
						if err != nil {
							t.Fatal(err)
						}
						start = func(current *model.Inbound, quota bool) error {
							inst, err := externalvpn.FromInbound(current)
							if err != nil {
								t.Fatal(err)
							}
							if quota {
								inst.ClientRevision = sidecartraffic.DisabledRevision(map[string]bool{"first": true})
								inst.Settings.Clients[0].Enable = false
							}
							return mgr.Ensure(inst)
						}
						replay = func() { mgr.Reconcile([]externalvpn.Instance{old}) }
						configPath = filepath.Join(dir, "externalvpn", "998", "credentials.toml")
					}
					defer stop()
					if err := start(&ib, false); err != nil {
						t.Fatal(err)
					}
					oldInbound := ib
					switch change {
					case "credentials":
						clients[0]["password"] = "new-password"
						clients[0]["secret"] = "1123456789abcdef0123456789abcdef"
						ib.Port++
					case "manual-disable":
						clients[0]["enable"] = false
						ib.Port++
					case "quota-disable":
						if err := db.Model(&row).Update("enable", false).Error; err != nil {
							t.Fatal(err)
						}
					}
					ib.Settings = encode()
					if err := db.Save(&ib).Error; err != nil {
						t.Fatal(err)
					}
					if operation == "ensure" {
						if err := start(&oldInbound, false); err == nil {
							t.Fatal("stale Ensure accepted obsolete configuration")
						}
						return
					}
					if err := start(&ib, change == "quota-disable"); err != nil {
						t.Fatal(err)
					}
					before, err := os.ReadFile(configPath)
					if err != nil {
						t.Fatal(err)
					}
					replay()
					after, err := os.ReadFile(configPath)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(before, after) {
						t.Fatal("stale Reconcile replaced current credentials/configuration")
					}
				})
			}
		}
	}
}
