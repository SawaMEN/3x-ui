//go:build !windows

package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
)

func TestShutdownPersistsMieruTrafficBeforeProcessRemoval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	stats := filepath.Join(dir, "stats.txt")
	t.Setenv("XUI_AUDIT_MIERU_STATS", stats)
	script := "#!/bin/sh\ncase \"$1\" in\nrun) exec sleep 60;;\nget) cat \"$XUI_AUDIT_MIERU_STATS\";;\nreload) exit 0;;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "mita"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	ib := model.Inbound{Id: 983, Tag: "audit-tail", Protocol: model.Mieru, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	row := coremodel.ClientTraffic{InboundId: ib.Id, Email: "tail@example.test", Enable: true}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	inst := mieru.Instance{Id: ib.Id, Tag: ib.Tag, PortBindings: []mieru.PortBinding{{Port: 34568, Protocol: "TCP"}}, Users: []mieru.User{{Name: row.Email, Password: "synthetic"}}}
	mgr := mieru.GetManager()
	mgr.StopAll()
	defer mgr.StopAll()
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	writeStats := func(up, down int) {
		t.Helper()
		if err := os.WriteFile(stats, []byte(fmt.Sprintf("%s - %dB %dB\n", row.Email, down, up)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeStats(10, 20)
	mgr.CollectTraffic([]mieru.Instance{inst})
	writeStats(110, 220)
	if err := NewServer(nil).StopPanelOnly(); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if row.Up != 0 || !mgr.HasRunning() {
		t.Fatal("panel-only reload consumed snapshot/stopped runtime")
	}
	if err := NewServer(nil).Stop(); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if row.Up != 100 || row.Down != 200 {
		t.Fatalf("shutdown lost Mieru tail: (%d,%d), want (100,200)", row.Up, row.Down)
	}
	if err := database.GetDB().First(&ib, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	if ib.Up != 100 || ib.Down != 200 {
		t.Fatalf("shutdown inbound tail: (%d,%d)", ib.Up, ib.Down)
	}
	if mgr.HasRunning() {
		t.Fatal("shutdown restarted process")
	}
}

func TestShutdownPersistsExternalTrafficBeforeProcessRemoval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	if err := os.WriteFile(filepath.Join(dir, "trusttunnel_endpoint"), []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ib := model.Inbound{Id: 984, Tag: "external-tail", Protocol: model.TrustTunnel, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	row := coremodel.ClientTraffic{InboundId: ib.Id, Email: "external-tail@example.test", Enable: true}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	inst := externalvpn.Instance{ID: ib.Id, Tag: ib.Tag, Protocol: ib.Protocol, Listen: "127.0.0.1", Port: 34569, Settings: externalvpn.Settings{Hostname: "fixture.example.test", Clients: []externalvpn.Client{{Email: row.Email, Password: "synthetic", Enable: true}}}}
	mgr := externalvpn.GetManager()
	mgr.StopAll()
	defer mgr.StopAll()
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "externalvpn", "984", "vpn.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct{ Metrics struct{ Address string } }
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	email := row.Email
	var total atomic.Uint64
	total.Store(10)
	srv := serveShutdownMetrics(t, cfg.Metrics.Address, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := total.Load()
		_, _ = fmt.Fprintf(w, `[{"username":%q,"inbound":%d,"outbound":%d}]`, email, n, n*2)
	}))
	defer srv.Close()
	if got := mgr.CollectTraffic(); len(got) != 1 {
		t.Fatalf("initial counters: %+v", got)
	}
	total.Store(110)
	if err := NewServer(nil).Stop(); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&ib, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	if row.Up != 100 || row.Down != 200 || ib.Up != 100 || ib.Down != 200 {
		t.Fatalf("external tail client=%d/%d inbound=%d/%d", row.Up, row.Down, ib.Up, ib.Down)
	}
	if got := mgr.CollectTraffic(); len(got) != 0 {
		t.Fatalf("external child restarted: %+v", got)
	}
}

func TestShutdownPersistsMtprotoTrafficAndAvoidsRoutedDoubleCount(t *testing.T) {
	for _, routed := range []bool{false, true} {
		t.Run(fmt.Sprint(routed), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_BIN_FOLDER", dir)
			t.Setenv("XUI_DB_FOLDER", t.TempDir())
			dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
			if err := os.WriteFile(filepath.Join(dir, mtproto.GetBinaryName()), []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			ib := model.Inbound{Id: 985, Tag: "mtproto-tail", Protocol: model.MTProto, Settings: `{"clients":[]}`}
			if err := database.GetDB().Create(&ib).Error; err != nil {
				t.Fatal(err)
			}
			row := coremodel.ClientTraffic{InboundId: ib.Id, Email: "mtproto-tail@example.test", Enable: true}
			if err := database.GetDB().Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			inst := mtproto.Instance{Id: ib.Id, Tag: ib.Tag, Listen: "127.0.0.1", Port: 34570, RouteThroughXray: routed, XrayRoutePort: 34571, Secrets: []mtproto.SecretEntry{{Name: row.Email, Secret: "0123456789abcdef0123456789abcdef"}}}
			mgr := mtproto.GetManager()
			mgr.StopAll()
			defer mgr.StopAll()
			if err := mgr.Ensure(inst); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "mtproto", "telemt-985.toml"))
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
				Access struct{ Users map[string]string }
			}
			if err := toml.Unmarshal(data, &cfg); err != nil {
				t.Fatal(err)
			}
			var username string
			for name := range cfg.Access.Users {
				username = name
			}
			if username == "" {
				t.Fatal("fake Telemt config has no user")
			}
			var total atomic.Int64
			total.Store(10)
			srv := serveShutdownMetrics(t, cfg.Server.API.Listen, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != cfg.Server.API.AuthHeader {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				_, _ = fmt.Fprintf(w, `{"ok":true,"data":[{"username":%q,"total_octets":%d}]}`, username, total.Load())
			}))
			defer srv.Close()
			mgr.CollectTrafficConsistent()
			total.Store(110)
			if err := NewServer(nil).Stop(); err != nil {
				t.Fatal(err)
			}
			if err := database.GetDB().First(&row, row.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.GetDB().First(&ib, ib.Id).Error; err != nil {
				t.Fatal(err)
			}
			expected := int64(100)
			if routed {
				expected = 0
			}
			if row.Up != 0 || row.Down != 100 || ib.Up != 0 || ib.Down != expected {
				t.Fatalf("MTProto tail client=%d/%d inbound=%d/%d routed=%v", row.Up, row.Down, ib.Up, ib.Down, routed)
			}
			if mgr.HasRunning() {
				t.Fatal("Telemt restarted during shutdown")
			}
		})
	}
}

func serveShutdownMetrics(t *testing.T, address string, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(handler)
	_ = srv.Listener.Close()
	srv.Listener = listener
	srv.Start()
	return srv
}
