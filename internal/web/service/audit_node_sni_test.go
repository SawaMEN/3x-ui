package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func TestAuditNodeSNIDiscoveryKeepsPathlessSummaries(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	previous := runtime.GetManager()
	runtime.SetManager(runtime.NewManager(runtime.LocalDeps{}))
	t.Cleanup(func() { runtime.SetManager(previous) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/panel/api/domains/sni" || req.URL.Query().Get("local") != "1" || req.URL.Query().Get("summary") != "1" || req.Header.Get("Authorization") != "Bearer audit-node" {
			t.Errorf("unexpected discovery request: %s", req.URL)
		}
		_, _ = w.Write([]byte(`{"success":true,"obj":[{"name":"cover.example.org","source":"gateway","expiresAt":2000000000,"issuer":"Example CA","certFile":"/private/node.crt","keyFile":"/private/node.key"}]}`))
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Name: "SNI node", Scheme: "http", Address: u.Hostname(), Port: port, BasePath: "/", ApiToken: "audit-node", Enable: true, AllowPrivateAddress: true}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	sites := discoverNodeSNI(context.Background())
	if len(sites) != 1 || sites[0].CertFile != "" || sites[0].KeyFile != "" {
		t.Fatalf("discovery must discard node paths: %+v", sites)
	}
	merged := mergeNodeManagedSNIDomains(nil, sites, map[string]bool{})
	if len(merged) != 1 || merged[0].NodeID != node.Id || merged[0].Name != "cover.example.org" || merged[0].ExpiresAt != 2000000000 || merged[0].Issuer != "Example CA" {
		t.Fatalf("valid pathless node summary disappeared: %+v", merged)
	}
}

func TestAuditNodeCertificateSummaryUsesDNSAndDropsPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { StopDomainFront(); _ = database.CloseDB() })
	certPath, keyPath := writeTestCertificate(t)
	settings := &SettingService{}
	for key, value := range map[string]string{"webCertFile": certPath, "webKeyFile": keyPath} {
		if err := settings.setString(key, value); err != nil {
			t.Fatal(err)
		}
	}
	summaries, err := LocalDomainCertificateSummaries()
	if err != nil || len(summaries) != 1 {
		t.Fatalf("local certificate summary: %+v, err=%v", summaries, err)
	}
	if summaries[0].Name != "golden-fixture.test" || summaries[0].ExpiresAt == 0 {
		t.Fatalf("summary lost certificate DNS identity or expiry: %+v", summaries[0])
	}
	if _, ok := nodeCertificateInventoryItem(7, "Node", summaries[0]); !ok {
		t.Fatal("parent rejected the local certificate summary")
	}
	raw, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{certPath, keyPath, "certFile", "keyFile", "warning"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("node metadata disclosed %q: %s", forbidden, raw)
		}
	}
}
