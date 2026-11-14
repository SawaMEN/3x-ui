package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/wirecodec"
)

func TestCoverCertificateSourcesIncludeTelemtWithoutChangingEditableSites(t *testing.T) {
	sites := []ManagedCoverSiteSummary{{Domain: "local.example.org", SiteCatalog: "builtin"}}
	got := appendTelemtCertificateSources(sites, []string{"telemt.example.org", "alias.example.org", "local.example.org", "https://bad.example.org", "*.example.org"})
	if len(got) != 3 || got[1].SiteCatalog != "telemt" || got[2].Domain != "alias.example.org" {
		t.Fatalf("invalid Telemt source list: %+v", got)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "certFile") || strings.Contains(string(raw), "keyFile") {
		t.Fatal("source list contains private paths")
	}
}

func TestCoverCertificateSourcesDiscoverNodesAndExplainFailures(t *testing.T) {
	setupBulkDB(t)
	useTestRuntimeManager(t)
	var exportCalls atomic.Int32
	addServer := func(label string, status int, body string) int {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/panel/api/domains/cover-sites/local" || r.URL.Query().Get("certificates") != "1" {
				exportCalls.Add(1)
				t.Errorf("unexpected request: %s", r.URL)
			}
			if r.Header.Get(wirecodec.MasterPushHeader) != "1" || r.Header.Get("Authorization") != "Bearer source-test" {
				t.Error("missing master authentication")
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(server.Close)
		host, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
		port, _ := strconv.Atoi(portText)
		sum := sha256.Sum256(server.Certificate().Raw)
		node := model.Node{Name: label, Scheme: "https", Address: host, Port: port, BasePath: "/", ApiToken: "source-test", Enable: true, AllowPrivateAddress: true, TlsVerifyMode: "pin", PinnedCertSha256: base64.StdEncoding.EncodeToString(sum[:])}
		if err := database.GetDB().Create(&node).Error; err != nil {
			t.Fatal(err)
		}
		return node.Id
	}
	good := addServer("Remote website", 200, `{"success":true,"obj":[{"domain":"web.example.org","siteCatalog":"telemt","certFile":"PRIVATE_PATH","siteHtml":"PRIVATE_HTML"}]}`)
	missing := addServer("Old node", 404, `PRIVATE_ERROR_BODY`)
	denied := addServer("No access", 403, `PRIVATE_TOKEN_ERROR`)
	empty := addServer("No website", 200, `{"success":true,"obj":[]}`)
	insecure := model.Node{Name: "HTTP node", Scheme: "http", Enable: true}
	if err := database.GetDB().Create(&insecure).Error; err != nil {
		t.Fatal(err)
	}
	report, err := CoverCertificateSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sites) != 1 || report.Sites[0].NodeID != good || report.Sites[0].Domain != "web.example.org" {
		t.Fatalf("remote source lost: %+v", report)
	}
	states := map[int]string{}
	for _, node := range report.Nodes {
		states[node.NodeID] = node.State
	}
	for id, want := range map[int]string{good: "ready", missing: "update_required", denied: "access_denied", empty: "empty", insecure.Id: "https_required"} {
		if states[id] != want {
			t.Errorf("node %d state=%q, want %q", id, states[id], want)
		}
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatalf("node response leaked: %s", raw)
	}
	// The main server is a valid recipient of the discovered remote identity.
	rows, err := SaveCoverCertificateFleet(context.Background(), good, "web.example.org", []int{0})
	if err != nil || len(rows) != 1 || rows[0].SourceNodeID != good || len(rows[0].Targets) != 1 || rows[0].Targets[0].NodeID != 0 {
		t.Fatalf("remote-to-main enrollment: %+v %v", rows, err)
	}
	if exportCalls.Load() != 0 {
		t.Fatal("metadata discovery requested certificate material")
	}
}
