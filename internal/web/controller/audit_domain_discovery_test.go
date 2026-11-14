package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestAuditNodeDomainDiscoveryScopesAndPrivacy(t *testing.T) {
	engine, api := newAPIAuthTestEngine(t)
	NewDomainController(engine.Group("/panel/api", api.checkAPIAuth, api.enforceTokenScope))
	t.Cleanup(service.StopDomainFront)
	for _, scope := range []string{model.ApiScopeNodeSync, model.ApiScopeNodeAdmin, model.ApiScopeMonitor, model.ApiScopeAdmin} {
		if err := database.GetDB().Create(&model.ApiToken{Name: scope, Token: crypto.HashTokenSHA256(scope), Scope: scope, Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range map[string]string{"webCertFile": "/private/audit-node.crt", "webKeyFile": "/private/audit-node.key"} {
		setting := model.Setting{Key: key}
		if err := database.GetDB().Where("key = ?", key).Assign(model.Setting{Value: value}).FirstOrCreate(&setting).Error; err != nil {
			t.Fatal(err)
		}
	}
	var childRequests atomic.Int32
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		childRequests.Add(1)
		_, _ = w.Write([]byte(`{"success":true,"obj":[]}`))
	}))
	defer child.Close()
	u, err := url.Parse(child.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Node{Name: "child trap", Scheme: "http", Address: u.Hostname(), Port: port, BasePath: "/", ApiToken: "child-token", AllowPrivateAddress: true, Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	previous := runtime.GetManager()
	runtime.SetManager(runtime.NewManager(runtime.LocalDeps{}))
	t.Cleanup(func() { runtime.SetManager(previous) })
	for _, scope := range []string{model.ApiScopeNodeSync, model.ApiScopeNodeAdmin, "mtls", model.ApiScopeMonitor, model.ApiScopeAdmin} {
		for _, route := range []struct {
			method, path string
			discovery    bool
		}{
			{http.MethodGet, "/domains/sni?local=0&summary=0", true},
			{http.MethodGet, "/domains/certificates?local=0", true},
			{http.MethodPost, "/domains/probe/http01", true},
			{http.MethodGet, "/domains/status", false},
			{http.MethodPost, "/domains/apply", false},
			{http.MethodPost, "/domains/certificates/renew", false},
			{http.MethodPost, "/domains/certificates/panel-domain", false},
			{http.MethodPost, "/domains/dns/credential", false},
		} {
			// Admin API keeps its existing general read API. Test its mutation
			// gate only; full local inventory includes deliberate firewall reads.
			if scope == model.ApiScopeAdmin && (route.method == http.MethodGet || route.discovery) {
				continue
			}
			t.Run(scope+route.method+route.path, func(t *testing.T) {
				req := httptest.NewRequest(route.method, "/panel/api"+route.path, strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				if scope == "mtls" {
					req.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}
				} else {
					req.Header.Set("Authorization", "Bearer "+scope)
				}
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				want := http.StatusForbidden
				if route.discovery && scope != model.ApiScopeMonitor {
					want = http.StatusOK
				}
				if w.Code != want {
					t.Fatalf("status=%d, want %d; body=%s", w.Code, want, w.Body.String())
				}
				if w.Code == http.StatusOK && strings.Contains(route.path, "/certificates") {
					var result struct {
						Success bool `json:"success"`
						Obj     struct {
							Items []json.RawMessage `json:"items"`
						} `json:"obj"`
					}
					if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || !result.Success || len(result.Obj.Items) == 0 {
						t.Fatalf("expected nonempty certificate metadata: %s, err=%v", w.Body.String(), err)
					}
					for _, forbidden := range []string{"/private/", "certFile", "keyFile", "warning", "firewall", "usedBy", "ports"} {
						if strings.Contains(w.Body.String(), forbidden) {
							t.Errorf("node summary disclosed %q: %s", forbidden, w.Body.String())
						}
					}
				}
			})
		}
	}
	server := httptest.NewServer(engine)
	defer server.Close()
	parentURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	parentPort, err := strconv.Atoi(parentURL.Port())
	if err != nil {
		t.Fatal(err)
	}
	remote := runtime.NewRemote(&model.Node{
		Id: 9001, Scheme: "http", Address: parentURL.Hostname(), Port: parentPort,
		BasePath: "/", ApiToken: model.ApiScopeNodeSync, AllowPrivateAddress: true,
	}, nil)
	if certs, err := remote.ListCertificateSummaries(context.Background()); err != nil || len(certs) == 0 {
		t.Fatalf("parent-to-node certificate RPC failed: %+v, err=%v", certs, err)
	}
	if _, err := remote.ListManagedSNI(context.Background()); err != nil {
		t.Fatalf("parent-to-node SNI RPC failed: %v", err)
	}
	if childRequests.Load() != 0 {
		t.Fatalf("node discovery recursively contacted %d child endpoints", childRequests.Load())
	}
}
