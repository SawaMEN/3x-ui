package runtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func panelUpdateTestRemote(t *testing.T, handler http.HandlerFunc) *Remote {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return NewRemote(&model.Node{
		Id: 999, Name: "test-node", Scheme: "http", Address: host,
		Port: port, BasePath: "/", ApiToken: "test-token",
		AllowPrivateAddress: true, TlsVerifyMode: "verify",
	}, nil)
}

func TestStartPanelUpdateAlwaysSendsSelectedChannel(t *testing.T) {
	for _, dev := range []bool{false, true} {
		t.Run(strconv.FormatBool(dev), func(t *testing.T) {
			remote := panelUpdateTestRemote(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/panel/api/server/updatePanel" || r.Method != http.MethodPost {
					t.Errorf("unexpected route: %s %s", r.Method, r.URL.Path)
				}
				if got := r.PostFormValue("dev"); got != strconv.FormatBool(dev) {
					t.Errorf("dev=%q, want %t", got, dev)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"success":true,"obj":{"runId":"1735689600123456789"}}`)
			})
			runID, err := remote.StartPanelUpdate(context.Background(), dev)
			if err != nil || runID != "1735689600123456789" {
				t.Fatalf("StartPanelUpdate = %q, %v", runID, err)
			}
		})
	}
}

func TestStartPanelUpdateRejectsMissingRunID(t *testing.T) {
	remote := panelUpdateTestRemote(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"success":true,"obj":{}}`)
	})
	if _, err := remote.StartPanelUpdate(context.Background(), false); err == nil {
		t.Fatal("accepted launch without run ID")
	}
}

func TestGetPanelUpdateStatusUsesRemoteResult(t *testing.T) {
	remote := panelUpdateTestRemote(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/panel/api/server/getUpdateStatus" || r.Method != http.MethodGet {
			t.Errorf("unexpected route: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"success":true,"obj":{"runId":"1735689600123456789","state":"failed","exitCode":2,"message":"insufficient disk space","logFile":"/var/log/x-ui/update.log"}}`)
	})
	status, err := remote.GetPanelUpdateStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.RunID != "1735689600123456789" || status.State != "failed" ||
		status.ExitCode != 2 || status.Message != "insufficient disk space" {
		t.Fatalf("unexpected status: %+v", status)
	}
}
