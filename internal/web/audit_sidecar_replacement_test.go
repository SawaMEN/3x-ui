//go:build !windows

package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/web/job"
)

func TestMieruLifecycleJournalsConsumedTail(t *testing.T) {
	for _, operation := range []string{"remove", "pause", "restart", "reload", "reconcile", "disk_failure"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_BIN_FOLDER", dir)
			t.Setenv("XUI_DB_FOLDER", dir)
			dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
			db := database.GetDB()
			stats := filepath.Join(dir, "stats")
			t.Setenv("XUI_AUDIT_REPLACEMENT_STATS", stats)
			if err := os.WriteFile(filepath.Join(dir, "mita"), []byte("#!/bin/sh\ncase \"$1\" in\nget) cat \"$XUI_AUDIT_REPLACEMENT_STATS\";;\nreload) exit 0;;\n*) exec sleep 60;;\nesac\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			ib := model.Inbound{Id: 990, Tag: "lifecycle", Protocol: model.Mieru, Settings: `{"clients":[]}`}
			if err := db.Create(&ib).Error; err != nil {
				t.Fatal(err)
			}
			row := coremodel.ClientTraffic{Email: "original@example.test", InboundId: ib.Id, Enable: true}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			inst := mieru.Instance{Id: ib.Id, Tag: ib.Tag, Users: []mieru.User{{Name: row.Email, Password: "test"}}, PortBindings: []mieru.PortBinding{{Port: 34576, Protocol: "TCP"}}}
			mgr := mieru.GetManager()
			mgr.StopAll()
			defer mgr.StopAll()
			if err := mgr.Ensure(inst); err != nil {
				t.Fatal(err)
			}
			write := func(up, down int) {
				t.Helper()
				if err := os.WriteFile(stats, []byte(fmt.Sprintf("original@example.test - %dB %dB\n", down, up)), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(10, 20)
			mgr.CollectTraffic([]mieru.Instance{inst})
			write(110, 220)
			// Old runtime names can be reassigned before its stop is applied. The old
			// owner's pinned ID must be used without a fresh lookup by email.
			if err := db.Model(&row).Update("email", "renamed@example.test").Error; err != nil {
				t.Fatal(err)
			}
			other := coremodel.ClientTraffic{Email: "original@example.test", InboundId: ib.Id, Enable: true}
			if err := db.Create(&other).Error; err != nil {
				t.Fatal(err)
			}
			if operation == "disk_failure" {
				if err := os.WriteFile(filepath.Join(dir, "mieru-traffic-journal"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			switch operation {
			case "disk_failure":
				mgr.Remove(ib.Id)
				if mgr.HasRunning() {
					t.Fatal("accounting outage kept removed user running")
				}
				if err := os.Remove(filepath.Join(dir, "mieru-traffic-journal")); err != nil {
					t.Fatal(err)
				}
			case "remove":
				mgr.Remove(ib.Id)
			case "pause":
				if err := mgr.Pause(ib.Id); err != nil {
					t.Fatal(err)
				}
			case "restart":
				inst.PortBindings[0].Port++
				if err := mgr.Ensure(inst); err != nil {
					t.Fatal(err)
				}
			case "reload":
				inst.Users[0].Password = "changed"
				if err := mgr.Ensure(inst); err != nil {
					t.Fatal(err)
				}
			case "reconcile":
				mgr.Reconcile(nil)
			}
			paths, err := filepath.Glob(filepath.Join(dir, "mieru-traffic-journal", "*.json"))
			if err != nil || (operation != "disk_failure" && len(paths) != 1) {
				t.Fatalf("old tail was not handed to durable journal: %v %v", paths, err)
			}
			job.NewMieruJob().Run()
			if err := db.First(&row, row.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&other, other.Id).Error; err != nil {
				t.Fatal(err)
			}
			if row.Up != 100 || row.Down != 200 || other.Up != 0 || other.Down != 0 {
				t.Fatalf("replayed owner totals original=%d/%d replacement=%d/%d", row.Up, row.Down, other.Up, other.Down)
			}
		})
	}
}

func TestMtprotoLifecycleJournalsConsumedTail(t *testing.T) {
	for _, routed := range []bool{false, true} {
		for _, operation := range []string{"remove", "restart", "reload", "reconcile"} {
			t.Run(fmt.Sprintf("%v/%s", routed, operation), func(t *testing.T) {
				dir := t.TempDir()
				t.Setenv("XUI_BIN_FOLDER", dir)
				t.Setenv("XUI_DB_FOLDER", dir)
				dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
				db := database.GetDB()
				if err := os.WriteFile(filepath.Join(dir, mtproto.GetBinaryName()), []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
				ib := model.Inbound{Id: 991, Tag: "mtproto-lifecycle", Protocol: model.MTProto, Settings: `{"clients":[]}`}
				if err := db.Create(&ib).Error; err != nil {
					t.Fatal(err)
				}
				row := coremodel.ClientTraffic{Email: "original@example.test", InboundId: ib.Id, Enable: true}
				if err := db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
				inst := mtproto.Instance{Id: ib.Id, Tag: ib.Tag, Listen: "127.0.0.1", Port: 34577, RouteThroughXray: routed, XrayRoutePort: 34578, Secrets: []mtproto.SecretEntry{{Name: row.Email, Secret: "0123456789abcdef0123456789abcdef"}}}
				mgr := mtproto.GetManager()
				mgr.StopAll()
				defer mgr.StopAll()
				if err := mgr.Ensure(inst); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(dir, "mtproto", "telemt-991.toml"))
				if err != nil {
					t.Fatal(err)
				}
				var cfg struct {
					Server struct{ API struct{ Listen string } }
					Access struct{ Users map[string]string }
				}
				if err := toml.Unmarshal(data, &cfg); err != nil {
					t.Fatal(err)
				}
				var username string
				for name := range cfg.Access.Users {
					username = name
				}
				var total atomic.Int64
				total.Store(10)
				srv := serveShutdownMetrics(t, cfg.Server.API.Listen, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost {
						_, _ = w.Write([]byte(`{"ok":true}`))
						return
					}
					_, _ = fmt.Fprintf(w, `{"ok":true,"data":[{"username":%q,"total_octets":%d}]}`, username, total.Load())
				}))
				defer srv.Close()
				mgr.CollectTrafficConsistent()
				total.Store(110)
				if err := db.Model(&row).Update("email", "renamed@example.test").Error; err != nil {
					t.Fatal(err)
				}
				other := coremodel.ClientTraffic{Email: "original@example.test", InboundId: ib.Id, Enable: true}
				if err := db.Create(&other).Error; err != nil {
					t.Fatal(err)
				}
				switch operation {
				case "remove":
					mgr.Remove(ib.Id)
				case "restart":
					inst.Port++
					if err := mgr.Ensure(inst); err != nil {
						t.Fatal(err)
					}
				case "reload":
					inst.Secrets[0].Secret = "abcdef0123456789abcdef0123456789"
					if err := mgr.Ensure(inst); err != nil {
						t.Fatal(err)
					}
				case "reconcile":
					mgr.Reconcile(nil)
				}
				paths, err := filepath.Glob(filepath.Join(dir, "mtproto-traffic-journal", "*.json"))
				if err != nil || len(paths) != 1 {
					t.Fatalf("missing durable MTProto tail: %v %v", paths, err)
				}
				job.NewMtprotoJob().Run()
				job.NewMtprotoJob().Run()
				if err := db.First(&row, row.Id).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.First(&other, other.Id).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.First(&ib, ib.Id).Error; err != nil {
					t.Fatal(err)
				}
				inbound := int64(100)
				if routed {
					inbound = 0
				}
				if row.Down != 100 || other.Down != 0 || ib.Down != inbound {
					t.Fatalf("MTProto replay original=%d replacement=%d inbound=%d", row.Down, other.Down, ib.Down)
				}
			})
		}
	}
}

func TestExternalLifecycleJournalsConsumedTail(t *testing.T) {
	for _, operation := range []string{"remove", "restart", "reconcile", "rollback"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_BIN_FOLDER", dir)
			t.Setenv("XUI_DB_FOLDER", dir)
			dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
			db := database.GetDB()
			script := "#!/bin/sh\nexec sleep 60\n"
			if operation == "rollback" {
				script = "#!/bin/sh\nif grep -q '^listen_address = \"127.0.0.1:34580\"$' \"$1\"; then exit 1; fi\nexec sleep 60\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "trusttunnel_endpoint"), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			ib := model.Inbound{Id: 992, Tag: "external-lifecycle", Protocol: model.TrustTunnel, Settings: `{"clients":[]}`}
			if err := db.Create(&ib).Error; err != nil {
				t.Fatal(err)
			}
			row := coremodel.ClientTraffic{Email: "original@example.test", InboundId: ib.Id, Enable: true}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			inst := externalvpn.Instance{ID: ib.Id, Tag: ib.Tag, Protocol: ib.Protocol, Listen: "127.0.0.1", Port: 34579, Settings: externalvpn.Settings{Hostname: "fixture.example.test", Clients: []externalvpn.Client{{Email: row.Email, Password: "synthetic", Enable: true}}}}
			mgr := externalvpn.GetManager()
			mgr.StopAll()
			defer mgr.StopAll()
			if err := mgr.Ensure(inst); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "externalvpn", "992", "vpn.toml"))
			if err != nil {
				t.Fatal(err)
			}
			var cfg struct{ Metrics struct{ Address string } }
			if err := toml.Unmarshal(data, &cfg); err != nil {
				t.Fatal(err)
			}
			var total atomic.Uint64
			total.Store(10)
			srv := serveShutdownMetrics(t, cfg.Metrics.Address, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := total.Load()
				_, _ = fmt.Fprintf(w, `[{"username":"original@example.test","inbound":%d,"outbound":%d}]`, n, n*2)
			}))
			defer srv.Close()
			mgr.CollectTraffic()
			total.Store(110)
			if err := db.Model(&row).Update("email", "renamed@example.test").Error; err != nil {
				t.Fatal(err)
			}
			other := coremodel.ClientTraffic{Email: "original@example.test", InboundId: ib.Id, Enable: true}
			if err := db.Create(&other).Error; err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "remove":
				mgr.Remove(ib.Id)
			case "restart":
				inst.Port++
				if err := mgr.Ensure(inst); err != nil {
					t.Fatal(err)
				}
			case "reconcile":
				mgr.Reconcile(nil)
			case "rollback":
				inst.Port++
				if err := mgr.Ensure(inst); err == nil {
					t.Fatal("replacement unexpectedly started")
				}
				data, err := os.ReadFile(filepath.Join(dir, "externalvpn", "992", "vpn.toml"))
				if err != nil {
					t.Fatal(err)
				}
				if err := toml.Unmarshal(data, &cfg); err != nil {
					t.Fatal(err)
				}
				srv.Close()
				total.Store(10)
				restored := serveShutdownMetrics(t, cfg.Metrics.Address, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := total.Load()
					_, _ = fmt.Fprintf(w, `[{"username":"original@example.test","inbound":%d,"outbound":%d}]`, n, n*2)
				}))
				defer restored.Close()
				mgr.Remove(ib.Id)
			}
			paths, err := filepath.Glob(filepath.Join(dir, "externalvpn-traffic-journal", "*.json"))
			expectedFiles := 1
			if operation == "rollback" {
				expectedFiles = 2
			}
			if err != nil || len(paths) != expectedFiles {
				t.Fatalf("missing durable external tail: %v %v", paths, err)
			}
			job.NewExternalVPNJob().Run()
			job.NewExternalVPNJob().Run()
			if err := db.First(&row, row.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&other, other.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&ib, ib.Id).Error; err != nil {
				t.Fatal(err)
			}
			up, down := int64(100), int64(200)
			if operation == "rollback" {
				up, down = 110, 220
			}
			if row.Up != up || row.Down != down || other.Up != 0 || other.Down != 0 || ib.Up != up || ib.Down != down {
				t.Fatalf("external replay original=%d/%d replacement=%d/%d inbound=%d/%d", row.Up, row.Down, other.Up, other.Down, ib.Up, ib.Down)
			}
		})
	}
}

func TestMieruReloadHandoffRejectsOlderConcurrentPoll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	stats, arm, entered, release := filepath.Join(dir, "stats"), filepath.Join(dir, "arm"), filepath.Join(dir, "entered"), filepath.Join(dir, "release")
	for key, value := range map[string]string{"XUI_STATS": stats, "XUI_ARM": arm, "XUI_ENTERED": entered, "XUI_RELEASE": release} {
		t.Setenv(key, value)
	}
	script := []byte("#!/bin/sh\ncase \"$1\" in\nget) data=$(cat \"$XUI_STATS\"); if [ -f \"$XUI_ARM\" ] && [ ! -f \"$XUI_ENTERED\" ]; then : > \"$XUI_ENTERED\"; while [ ! -f \"$XUI_RELEASE\" ]; do sleep 0.01; done; fi; echo \"$data\";;\nreload) exit 0;;\n*) exec sleep 60;;\nesac\n")
	if err := os.WriteFile(filepath.Join(dir, "mita"), script, 0o700); err != nil {
		t.Fatal(err)
	}
	ib := model.Inbound{Id: 993, Tag: "concurrent-reload", Protocol: model.Mieru, Settings: `{"clients":[]}`}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	row := coremodel.ClientTraffic{Email: "concurrent@example.test", InboundId: ib.Id, Enable: true}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	inst := mieru.Instance{Id: ib.Id, Tag: ib.Tag, Users: []mieru.User{{Name: row.Email, Password: "old"}}, PortBindings: []mieru.PortBinding{{Port: 34580, Protocol: "TCP"}}}
	mgr := mieru.GetManager()
	mgr.StopAll()
	defer mgr.StopAll()
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	write := func(up, down int) {
		t.Helper()
		if err := os.WriteFile(stats, []byte(fmt.Sprintf("%s - %dB %dB\n", row.Email, down, up)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(10, 20)
	mgr.CollectTraffic([]mieru.Instance{inst})
	write(110, 220)
	if err := os.WriteFile(arm, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	type result struct {
		rows []mieru.TrafficDelta
		err  error
	}
	done := make(chan result, 1)
	snapshot := inst
	snapshot.Users = append([]mieru.User(nil), inst.Users...)
	go func(snapshot mieru.Instance) {
		rows, _, err := mgr.CollectTrafficChecked([]mieru.Instance{snapshot})
		done <- result{rows: rows, err: err}
	}(snapshot)
	defer os.WriteFile(release, nil, 0o600)
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(entered); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old poll did not reach fixture gate")
		}
		time.Sleep(time.Millisecond)
	}
	write(210, 420)
	inst.Users = append([]mieru.User(nil), inst.Users...)
	inst.Users[0].Password = "new"
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err == nil || len(got.rows) != 0 {
			t.Fatalf("older poll accepted after handoff: %+v %v", got.rows, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("old poll not joined")
	}
	write(220, 440)
	rows, _, err := mgr.CollectTrafficChecked([]mieru.Instance{inst})
	if err != nil || len(rows) != 1 || rows[0].Up != 10 || rows[0].Down != 20 {
		t.Fatalf("handoff cursor overwritten by old poll: %+v %v", rows, err)
	}
	mgr.Remove(ib.Id)
	job.NewMieruJob().Run()
	if err := db.First(&row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if row.Up != 200 || row.Down != 400 {
		t.Fatalf("handoff charged twice: %d/%d", row.Up, row.Down)
	}
}

func TestMtprotoReloadHandoffRejectsOlderConcurrentPoll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	if err := os.WriteFile(filepath.Join(dir, mtproto.GetBinaryName()), []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ib := model.Inbound{Id: 994, Tag: "mtproto-concurrent", Protocol: model.MTProto, Settings: `{"clients":[]}`}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	row := coremodel.ClientTraffic{Email: "concurrent@example.test", InboundId: ib.Id, Enable: true}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	inst := mtproto.Instance{Id: ib.Id, Tag: ib.Tag, Listen: "127.0.0.1", Port: 34581, Secrets: []mtproto.SecretEntry{{Name: row.Email, Secret: "0123456789abcdef0123456789abcdef"}}}
	mgr := mtproto.GetManager()
	mgr.StopAll()
	defer mgr.StopAll()
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "mtproto", "telemt-994.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Server struct{ API struct{ Listen string } }
		Access struct{ Users map[string]string }
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	var username string
	for name := range cfg.Access.Users {
		username = name
	}
	var total atomic.Int64
	total.Store(10)
	var block atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	srv := serveShutdownMetrics(t, cfg.Server.API.Listen, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		n := total.Load()
		if block.CompareAndSwap(true, false) {
			close(entered)
			<-release
		}
		_, _ = fmt.Fprintf(w, `{"ok":true,"data":[{"username":%q,"total_octets":%d}]}`, username, n)
	}))
	defer srv.Close()
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	mgr.CollectTrafficConsistent()
	total.Store(110)
	block.Store(true)
	type result struct {
		rows []mtproto.Traffic
		err  error
	}
	done := make(chan result, 1)
	go func() { rows, _, err := mgr.CollectTrafficConsistentChecked(); done <- result{rows: rows, err: err} }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("old Telemt poll not entered")
	}
	total.Store(210)
	inst.Secrets[0].Secret = "abcdef0123456789abcdef0123456789"
	if err := mgr.Ensure(inst); err != nil {
		unblock()
		t.Fatal(err)
	}
	unblock()
	select {
	case got := <-done:
		if got.err == nil || len(got.rows) != 0 {
			t.Fatalf("old Telemt poll accepted after handoff: %+v %v", got.rows, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("old Telemt poll not joined")
	}
	total.Store(220)
	rows, _, err := mgr.CollectTrafficConsistentChecked()
	if err != nil || len(rows) != 1 || rows[0].Down != 10 {
		t.Fatalf("Telemt handoff cursor overwritten: %+v %v", rows, err)
	}
	mgr.Remove(ib.Id)
	job.NewMtprotoJob().Run()
	if err := db.First(&row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if row.Down != 200 {
		t.Fatalf("Telemt handoff charged twice: %d", row.Down)
	}
}
