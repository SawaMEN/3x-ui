//go:build !windows

package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
	"github.com/SawaMEN/3x-ui/v3/internal/web/job"
)

func TestPeriodicSidecarTrafficKeepsRuntimeOwner(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.MTProto, model.TrustTunnel} {
		for _, variant := range []string{"normal", "disk-retry", "routed"} {
			if variant == "routed" && protocol != model.MTProto {
				continue
			}
			t.Run(string(protocol)+"/"+variant, func(t *testing.T) {
				dir := t.TempDir()
				t.Setenv("XUI_BIN_FOLDER", dir)
				t.Setenv("XUI_DB_FOLDER", dir)
				sidecaregress.SetRuntimeReader(nil)
				t.Cleanup(func() { sidecaregress.SetRuntimeReader(nil) })
				dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
				db := database.GetDB()
				ib := model.Inbound{Id: 1002, Tag: "periodic-owner", Enable: true, Protocol: protocol, Listen: "127.0.0.1", Port: 34589, Settings: `{"hostname":"fixture.example.test","clients":[{"email":"accounting","enable":true,"password":"synthetic","secret":"0123456789abcdef0123456789abcdef"}]}`}
				if variant == "routed" {
					ib.Settings = strings.Replace(ib.Settings, `{"hostname"`, `{"routeThroughXray":true,"routeXrayPort":34590,"hostname"`, 1)
				}
				source := string(protocol)
				if protocol == model.TrustTunnel {
					source = "externalvpn"
				}
				blockPath := filepath.Join(dir, source+"-traffic-journal")
				t.Setenv("XUI_AUDIT_OWNER_BLOCK", "")
				var block atomic.Bool
				blockFS := func() {
					if block.Swap(false) {
						if err := os.WriteFile(blockPath, nil, 0o600); err != nil {
							t.Error(err)
						}
					}
				}
				if err := db.Create(&ib).Error; err != nil {
					t.Fatal(err)
				}
				original := coremodel.ClientTraffic{InboundId: ib.Id, Email: "accounting", Enable: true}
				if err := db.Create(&original).Error; err != nil {
					t.Fatal(err)
				}
				var run func()
				var total atomic.Int64
				total.Store(10)
				switch protocol {
				case model.Mieru:
					stats := filepath.Join(dir, "stats")
					t.Setenv("XUI_AUDIT_OWNER_STATS", stats)
					script := "#!/bin/sh\ncase \"$1\" in\nget) if [ -n \"$XUI_AUDIT_OWNER_BLOCK\" ]; then : > \"$XUI_AUDIT_OWNER_BLOCK\"; fi; cat \"$XUI_AUDIT_OWNER_STATS\";;\nreload) exit 0;;\n*) exec sleep 60;;\nesac\n"
					if err := os.WriteFile(filepath.Join(dir, "mita"), []byte(script), 0o700); err != nil {
						t.Fatal(err)
					}
					mgr := mieru.GetManager()
					mgr.StopAll()
					t.Cleanup(mgr.StopAll)
					inst, ok := mieru.InstanceFromInbound(&ib)
					if !ok {
						t.Fatal("invalid Mieru fixture")
					}
					if err := mgr.Ensure(inst); err != nil {
						t.Fatal(err)
					}
					write := func(n int) {
						if err := os.WriteFile(stats, []byte(fmt.Sprintf("accounting - %dB %dB\n", n*2, n)), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					write(10)
					mgr.CollectTraffic([]mieru.Instance{inst})
					write(110)
					run = func() { job.NewMieruJob().Run() }
				case model.MTProto:
					if err := os.WriteFile(filepath.Join(dir, mtproto.GetBinaryName()), []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
						t.Fatal(err)
					}
					mgr := mtproto.GetManager()
					mgr.StopAll()
					t.Cleanup(mgr.StopAll)
					inst, ok := mtproto.InstanceFromInbound(&ib)
					if !ok {
						t.Fatal("invalid Telemt fixture")
					}
					if err := mgr.Ensure(inst); err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(filepath.Join(dir, "mtproto", "telemt-1002.toml"))
					if err != nil {
						t.Fatal(err)
					}
					var cfg struct {
						Server struct {
							API struct {
								Listen     string
								AuthHeader string `toml:"auth_header"`
							}
						}
					}
					if err := toml.Unmarshal(data, &cfg); err != nil {
						t.Fatal(err)
					}
					srv := serveShutdownMetrics(t, cfg.Server.API.Listen, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Header.Get("Authorization") != cfg.Server.API.AuthHeader {
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
						blockFS()
						_, _ = fmt.Fprintf(w, `{"ok":true,"data":[{"username":"accounting","total_octets":%d}]}`, total.Load())
					}))
					defer srv.Close()
					mgr.CollectTrafficConsistent()
					total.Store(110)
					run = func() { job.NewMtprotoJob().Run() }
				default:
					if err := os.WriteFile(filepath.Join(dir, "trusttunnel_endpoint"), []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
						t.Fatal(err)
					}
					mgr := externalvpn.GetManager()
					mgr.StopAll()
					t.Cleanup(mgr.StopAll)
					inst, err := externalvpn.FromInbound(&ib)
					if err != nil {
						t.Fatal(err)
					}
					if err := mgr.Ensure(inst); err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(filepath.Join(dir, "externalvpn", "1002", "vpn.toml"))
					if err != nil {
						t.Fatal(err)
					}
					var cfg struct{ Metrics struct{ Address string } }
					if err := toml.Unmarshal(data, &cfg); err != nil {
						t.Fatal(err)
					}
					srv := serveShutdownMetrics(t, cfg.Metrics.Address, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						blockFS()
						n := total.Load()
						_, _ = fmt.Fprintf(w, `[{"username":"accounting","inbound":%d,"outbound":%d}]`, n, n*2)
					}))
					defer srv.Close()
					mgr.CollectTraffic()
					total.Store(110)
					run = func() { job.NewExternalVPNJob().Run() }
				}
				if err := db.Model(&original).Update("email", "retired-accounting").Error; err != nil {
					t.Fatal(err)
				}
				replacement := coremodel.ClientTraffic{InboundId: ib.Id, Email: "accounting", Enable: true}
				if err := db.Create(&replacement).Error; err != nil {
					t.Fatal(err)
				}
				if variant == "disk-retry" {
					block.Store(true)
					t.Setenv("XUI_AUDIT_OWNER_BLOCK", blockPath)
				}
				run()
				if variant == "disk-retry" {
					var current coremodel.ClientTraffic
					if err := db.First(&current, original.Id).Error; err != nil {
						t.Fatal(err)
					}
					if current.Up != 0 || current.Down != 0 {
						t.Fatal("disk failure committed counters without a journal")
					}
					if err := os.Remove(blockPath); err != nil {
						t.Fatal(err)
					}
					t.Setenv("XUI_AUDIT_OWNER_BLOCK", "")
					block.Store(false)
					run() // A replacement job recovers the pending batch exactly once.
				}

				if err := db.First(&original, original.Id).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.First(&replacement, replacement.Id).Error; err != nil {
					t.Fatal(err)
				}
				wantUp, wantDown := int64(100), int64(200)
				if protocol == model.MTProto {
					wantUp = 0
					wantDown = 100
				}
				if err := db.First(&ib, ib.Id).Error; err != nil {
					t.Fatal(err)
				}
				inboundUp, inboundDown := wantUp, wantDown
				if variant == "routed" {
					inboundUp = 0
					inboundDown = 0
				}
				if ib.Up != inboundUp || ib.Down != inboundDown {
					t.Fatalf("wrong inbound totals %d/%d, want %d/%d", ib.Up, ib.Down, inboundUp, inboundDown)
				}
				if original.Up != wantUp || original.Down != wantDown || replacement.Up != 0 || replacement.Down != 0 {
					t.Fatalf("runtime bytes charged by reused email: original=%d/%d replacement=%d/%d", original.Up, original.Down, replacement.Up, replacement.Down)
				}
			})
		}
	}
}
