package externalvpn

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestOpenFluxAutomaticContextAndSecret(t *testing.T) {
	ib := &model.Inbound{Id: 91, Protocol: model.OpenFlux, Port: 8443, Settings: `{"clients":[{"email":"alice","enable":true}]}`}
	if err := PrepareAdditional(ib, ""); err != nil {
		t.Fatal(err)
	}
	first, err := FromInbound(ib)
	if err != nil {
		t.Fatal(err)
	}
	if first.Settings.Context == "" || len(first.Settings.Clients[0].Password) < 32 {
		t.Fatal("automatic credentials were not generated")
	}
	previous := ib.Settings
	ib.Settings = `{"context":"","clients":[{"email":"alice","enable":true}],"transports":[{"type":"direct","priority":100}]}`
	if err := PrepareAdditional(ib, previous); err != nil {
		t.Fatal(err)
	}
	second, _ := FromInbound(ib)
	if second.Settings.Context != first.Settings.Context || second.Settings.Clients[0].Password != first.Settings.Clients[0].Password {
		t.Fatal("editing rotated encryption identity")
	}
}

func TestOpenFluxLegacyContextSurvivesTransportEdit(t *testing.T) {
	folder := t.TempDir()
	for _, context := range []string{"http://#", "https://docs.yandex.ru/edit/d/channel"} {
		t.Run(context, func(t *testing.T) {
			previous := Settings{Clients: []Client{{Email: "alice", Password: strings.Repeat("a", 64), Enable: true}}, Transports: []OpenFluxTransport{{Type: "direct", Priority: 100}}}
			if context != "http://#" {
				previous.Transports = append(previous.Transports, OpenFluxTransport{Type: "vyandex", URL: context, Priority: 50})
			}
			raw, _ := json.Marshal(previous)
			ib := &model.Inbound{Id: 92, Protocol: model.OpenFlux, Port: 8443, Settings: `{"context":"","clients":[{"email":"alice","enable":true}],"transports":[{"type":"direct","priority":100}]}`}
			if err := PrepareAdditional(ib, string(raw)); err != nil {
				t.Fatal(err)
			}
			inst, err := FromInbound(ib)
			if err != nil || inst.Settings.Context != context {
				t.Fatalf("legacy context changed: %q, %v", inst.Settings.Context, err)
			}
			t.Setenv("XUI_BIN_FOLDER", folder)
			cmd, cancel, err := GetManager().additionalCommand(inst, "")
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()
			found := false
			for i, arg := range cmd.Args {
				if arg == "--session-context" && i+1 < len(cmd.Args) && cmd.Args[i+1] == context {
					found = true
				}
			}
			if !found {
				t.Fatal("exact context was not passed to OpenFlux")
			}
		})
	}
}

func TestOpenFluxOfficialSecretLengthAndCopiedLinks(t *testing.T) {
	inst := Instance{Protocol: model.OpenFlux, Port: 443, Settings: Settings{Context: "test", Clients: []Client{{Email: "alice", Password: strings.Repeat("я", 16), Enable: true}}, Transports: []OpenFluxTransport{{Type: "direct", Priority: 100}}}}
	link, err := ExportAdditionalLink(inst, "alice", "example.org", 443, "test")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimPrefix(link, "openflux://v1/")
	body = strings.NewReplacer("-", "+", "_", "/").Replace(body)
	copied := " \nopenflux://v1/" + body[:10] + "\n\t" + body[10:] + "==\n "
	decoded, err := DecodeOpenFluxLink(copied)
	if err != nil || decoded.Secret != inst.Settings.Clients[0].Password {
		t.Fatalf("copied official link failed: %v", err)
	}
	inst.Settings.Clients[0].Password = strings.Repeat("я", 10)
	if err := inst.Validate(); err == nil {
		t.Fatal("short multibyte secret accepted")
	}
	inst.Settings.Clients[0].Password = strings.Repeat("a", 32)
	inst.Settings.Context = " trailing "
	if err := inst.Validate(); err == nil {
		t.Fatal("INI whitespace mismatch accepted")
	}
}

func TestOpenFluxInstallerDeduplicatesAndReportsFailure(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	job := &openFluxInstaller{install: func(ctx context.Context) error {
		calls.Add(1)
		select {
		case <-release:
			return errors.New("test installation failed")
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	first, second := job.start(), job.start()
	if first.RunID == "" || first.RunID != second.RunID || second.State != "running" {
		t.Fatal("duplicate installation was scheduled")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for job.read().State == "running" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	result := job.read()
	if calls.Load() != 1 || result.State != "failed" || result.Error != "test installation failed" {
		t.Fatalf("job result: %+v, calls=%d", result, calls.Load())
	}
}

func TestOpenFluxStatusNeedsVerifiedReceipt(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	path := binary(model.OpenFlux)
	if err := os.WriteFile(path, []byte("3x-ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeOpenFluxReceipt("0.3.0", "sha256:1343462835cbb1f7631c393c1090169aee14f90129e0922c7ec1d58fd40b0647"); err != nil {
		t.Fatal(err)
	}
	status := OpenFluxBinaryStatus(context.Background(), true)
	if !status.Supported {
		t.Skip("installer platform")
	}
	if !status.Installed || status.Version != "0.3.0" || status.UpdateAvailable {
		t.Fatalf("verified install not recognized: %+v", status)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "openflux"), []byte("replaced"), 0o755); err != nil {
		t.Fatal(err)
	}
	status = OpenFluxBinaryStatus(context.Background(), true)
	if !status.Installed || status.Version != "" || status.UpdateAvailable {
		t.Fatalf("stale receipt trusted: %+v", status)
	}
}

func TestOpenFluxLatestShareableCarriers(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	for _, typ := range []string{"direct", "yandex", "vyandex", "boards", "mailru", "cupsonline", "avito", "telemost"} {
		t.Run(typ, func(t *testing.T) {
			inst := Instance{ID: 95, Protocol: model.OpenFlux, Port: 443, Settings: Settings{
				Context: "stable-context", MaxPacketSize: 1400, RefusePeerLogin: true, Debug: 2,
				Clients:    []Client{{Email: "alice", Password: strings.Repeat("a", 32), Enable: true}},
				Transports: []OpenFluxTransport{{Type: typ, URL: "https://example.org/room", Priority: 50}},
			}}
			if err := inst.Validate(); err != nil {
				t.Fatal(err)
			}
			link, err := ExportAdditionalLink(inst, "alice", "example.org", 443, typ)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeOpenFluxLink(link)
			if err != nil || len(decoded.Transports) != 1 || decoded.Transports[0].Type != typ {
				t.Fatalf("link mismatch: %+v, %v", decoded, err)
			}
			cmd, cancel, err := GetManager().additionalCommand(inst, "")
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()
			args := strings.Join(cmd.Args, " ")
			for _, want := range []string{"--negotiate", "--max-packet-size 1400", "--refuse-peer-login", "--debug 2"} {
				if !strings.Contains(args, want) {
					t.Fatalf("missing %q in %s", want, args)
				}
			}
			data, err := os.ReadFile(filepath.Join(directory(), "95", "openflux.conf"))
			if err != nil || !strings.Contains(string(data), "Type = "+typ) {
				t.Fatalf("missing carrier config: %v", err)
			}
			inst.Settings.MaxPacketSize = 1279
			if err := inst.Validate(); err == nil {
				t.Fatal("bad packet limit accepted")
			}
			inst.Settings.MaxPacketSize, inst.Settings.Debug = 0, 3
			if err := inst.Validate(); err == nil {
				t.Fatal("sensitive debug level accepted")
			}
		})
	}
}
