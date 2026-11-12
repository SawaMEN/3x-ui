//go:build !windows

package web

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func TestSidecarRejectsRevokedDesired(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.MTProto, model.OpenFlux} {
		for _, revoke := range []string{"delete", "disable", "move-node", "change-protocol"} {
			for _, operation := range []string{"ensure", "reconcile"} {
				t.Run(string(protocol)+"/"+revoke+"/"+operation, func(t *testing.T) {
					dir := t.TempDir()
					t.Setenv("XUI_BIN_FOLDER", dir)
					t.Setenv("XUI_DB_FOLDER", dir)
					dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
					db := database.GetDB()
					ib := model.Inbound{Id: 995, Enable: true, Tag: "revocation", Protocol: protocol, Settings: `{"clients":[]}`}
					if err := db.Create(&ib).Error; err != nil {
						t.Fatal(err)
					}
					marker := filepath.Join(dir, "started")
					t.Setenv("XUI_AUDIT_REVOCATION_MARKER", marker)
					script := []byte("#!/bin/sh\ncase \"$1\" in\nget) exit 0;;\n*) echo started > \"$XUI_AUDIT_REVOCATION_MARKER\"; exec sleep 60;;\nesac\n")
					var ensure func() error
					var reconcile, stop func()
					switch protocol {
					case model.Mieru:
						if err := os.WriteFile(filepath.Join(dir, "mita"), script, 0o700); err != nil {
							t.Fatal(err)
						}
						mgr := mieru.GetManager()
						mgr.StopAll()
						stop = mgr.StopAll
						inst := mieru.Instance{Id: ib.Id, Tag: ib.Tag, Users: []mieru.User{{Name: "test", Password: "test"}}, PortBindings: []mieru.PortBinding{{Port: 34582, Protocol: "TCP"}}}
						ensure = func() error { return mgr.Ensure(inst) }
						reconcile = func() { mgr.Reconcile([]mieru.Instance{inst}) }
					case model.MTProto:
						if err := os.WriteFile(filepath.Join(dir, mtproto.GetBinaryName()), script, 0o700); err != nil {
							t.Fatal(err)
						}
						mgr := mtproto.GetManager()
						mgr.StopAll()
						stop = mgr.StopAll
						inst := mtproto.Instance{Id: ib.Id, Tag: ib.Tag, Listen: "127.0.0.1", Port: 34582, Secrets: []mtproto.SecretEntry{{Name: "test", Secret: "0123456789abcdef0123456789abcdef"}}}
						ensure = func() error { return mgr.Ensure(inst) }
						reconcile = func() { mgr.Reconcile([]mtproto.Instance{inst}) }
					default:
						if err := os.WriteFile(filepath.Join(dir, "openflux"), script, 0o700); err != nil {
							t.Fatal(err)
						}
						mgr := externalvpn.GetManager()
						mgr.StopAll()
						stop = mgr.StopAll
						inst := externalvpn.Instance{ID: ib.Id, Tag: ib.Tag, Protocol: protocol, Listen: "127.0.0.1", Port: 34582, Settings: externalvpn.Settings{Context: "stable", Transports: []externalvpn.OpenFluxTransport{{Type: "direct", Priority: 100}}, Clients: []externalvpn.Client{{Email: "test", Password: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Enable: true}}}}
						ensure = func() error { return mgr.Ensure(inst) }
						reconcile = func() { mgr.Reconcile([]externalvpn.Instance{inst}) }
					}
					defer stop()
					if err := ensure(); err != nil {
						t.Fatal(err)
					}
					deadline := time.Now().Add(time.Second)
					for {
						if _, err := os.Stat(marker); err == nil {
							break
						}
						if time.Now().After(deadline) {
							t.Fatal("fixture child did not start")
						}
						time.Sleep(time.Millisecond)
					}
					var err error
					switch revoke {
					case "delete":
						err = db.Delete(&ib).Error
					case "disable":
						err = db.Model(&ib).Update("enable", false).Error
					case "move-node":
						err = db.Model(&ib).Update("node_id", 123).Error
					case "change-protocol":
						err = db.Model(&ib).Update("protocol", model.TrustTunnel).Error
					}
					if err != nil {
						t.Fatal(err)
					}
					// Cached/no-op paths must revalidate too, before any lifecycle work.
					if err := ensure(); !errors.Is(err, sidecartraffic.ErrRevoked) {
						t.Fatalf("cached Ensure accepted revoked inbound: %v", err)
					}
					stop()
					if err := os.Remove(marker); err != nil {
						t.Fatal(err)
					}
					if operation == "ensure" {
						if err := ensure(); err == nil {
							t.Error("stale Ensure accepted revoked inbound")
						}
					} else {
						reconcile()
					}
					// Startup is asynchronous for Mieru/Telemt. Give the owned child a chance
					// to write its marker; no network, system services or real VPN is involved.
					deadline = time.Now().Add(50 * time.Millisecond)
					for time.Now().Before(deadline) {
						if _, err := os.Stat(marker); err == nil {
							t.Fatal("stale desired restarted revoked inbound")
						} else if !errors.Is(err, os.ErrNotExist) {
							t.Fatal(err)
						}
						time.Sleep(time.Millisecond)
					}
				})
			}
		}
	}
}
