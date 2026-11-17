package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestNewNodeAddAndReconnect(t *testing.T) {
	for _, tls := range []bool{false, true} {
		name := "http"
		if tls {
			name = "https"
		}
		t.Run(name, func(t *testing.T) {
			engine := newNodeCredentialTestEngine(t)
			requests := 0
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/private-panel/panel/api/server/status" {
					http.NotFound(w, r)
					return
				}
				if r.Header.Get("Authorization") != "Bearer new-node-token" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"obj":{"core":{"type":"singbox","state":"running","version":"1.12"},"panelGuid":"new-guid"}}`))
			})
			var remote *httptest.Server
			if tls {
				remote = httptest.NewTLSServer(handler)
			} else {
				remote = httptest.NewServer(handler)
			}
			defer remote.Close()
			u, _ := url.Parse(remote.URL)
			port, _ := strconv.Atoi(u.Port())
			payload := map[string]any{"name": "new-node", "scheme": u.Scheme, "address": u.Hostname(), "port": port, "basePath": "private-panel", "apiToken": "new-node-token", "enable": true, "allowPrivateAddress": true, "tlsVerifyMode": "skip"}
			raw, _ := json.Marshal(payload)
			call := func(path string, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				return w
			}
			for _, path := range []string{"/panel/api/nodes/test", "/panel/api/nodes/add"} {
				w := call(path, string(raw))
				if !strings.Contains(w.Body.String(), `"success":true`) {
					t.Fatalf("%s: %s", path, w.Body.String())
				}
			}
			var node model.Node
			if err := database.GetDB().Where("name = ?", "new-node").First(&node).Error; err != nil {
				t.Fatal(err)
			}
			if node.Status != "online" || node.BasePath != "/private-panel/" {
				t.Fatalf("new node not connected: %+v", node)
			}
			w := call("/panel/api/nodes/probe/"+strconv.Itoa(node.Id), "")
			if !strings.Contains(w.Body.String(), `"status":"online"`) {
				t.Fatal(w.Body.String())
			}
			if requests != 3 {
				t.Fatalf("remote requests = %d, want test + add + probe", requests)
			}
			payload["name"] = "rejected-node"
			payload["apiToken"] = "wrong-token"
			raw, _ = json.Marshal(payload)
			denied := call("/panel/api/nodes/add", string(raw))
			if !strings.Contains(denied.Body.String(), `"success":false`) || !strings.Contains(denied.Body.String(), "403") {
				t.Fatal(denied.Body.String())
			}
			var count int64
			if err := database.GetDB().Model(&model.Node{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("rejected token added a node: rows=%d", count)
			}
		})
	}
}

func TestNewNodeAddRollsBackOnHeartbeatFailure(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"obj":{"panelGuid":"new-guid"}}`))
	}))
	defer remote.Close()
	u, _ := url.Parse(remote.URL)
	port, _ := strconv.Atoi(u.Port())
	injected := errors.New("heartbeat write failed")
	db := database.GetDB()
	const callback = "test:new_node_heartbeat_failure"
	if err := db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "nodes" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Update().Remove(callback)
	raw, _ := json.Marshal(map[string]any{"name": "new-node", "scheme": "http", "address": u.Hostname(), "port": port, "apiToken": "test-token", "enable": true, "allowPrivateAddress": true})
	req := httptest.NewRequest(http.MethodPost, "/panel/api/nodes/add", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), injected.Error()) || !strings.Contains(w.Body.String(), `"success":false`) {
		t.Fatal(w.Body.String())
	}
	var count int64
	if err := db.Model(&model.Node{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed add left %d hidden nodes", count)
	}
}

func TestNewNodeRoutedAddRollsBackWhenCoreCannotApply(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	settings := &service.SettingService{}
	if err := settings.SetCoreType(service.CoreTypeSingBox); err != nil {
		t.Fatal(err)
	}
	core := service.NativeCore(service.CoreTypeSingBox)
	t.Cleanup(func() { core.IsNeedRestartAndSetFalse() })
	// A routed node must never be probed directly if creating its bridge fails.
	requests := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; w.WriteHeader(http.StatusOK) }))
	defer remote.Close()
	u, _ := url.Parse(remote.URL)
	port, _ := strconv.Atoi(u.Port())
	raw, _ := json.Marshal(map[string]any{"name": "routed-node", "scheme": "http", "address": u.Hostname(), "port": port, "apiToken": "test-token", "enable": true, "allowPrivateAddress": true, "outboundTag": "via-vpn"})
	req := httptest.NewRequest(http.MethodPost, "/panel/api/nodes/add", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `"success":false`) || !strings.Contains(w.Body.String(), "not installed") {
		t.Fatal(w.Body.String())
	}
	var count int64
	if err := database.GetDB().Model(&model.Node{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 || requests != 0 {
		t.Fatalf("failed routed add: rows=%d, direct requests=%d", count, requests)
	}
}

type newNodeTestEgress struct{ url string }

func (e newNodeTestEgress) NodeEgressProxyURL(int) string { return e.url }

func TestNewNodeSavedRouteReconnect(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)
	requests := 0
	// This is a local stand-in for the applied core bridge. The deliberately
	// unresolvable node hostname makes a direct-connection fallback fail.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Host != "node.invalid:2053" || r.URL.Path != "/private/panel/api/server/status" || r.Header.Get("Authorization") != "Bearer route-token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":{"panelGuid":"route-guid","core":{"type":"singbox","state":"running"}}}`))
	}))
	defer proxy.Close()
	old := webruntime.GetManager()
	manager := webruntime.NewManager(webruntime.LocalDeps{})
	manager.SetNodeEgressResolver(newNodeTestEgress{url: proxy.URL})
	webruntime.SetManager(manager)
	t.Cleanup(func() { webruntime.SetManager(old) })
	node := &model.Node{Name: "routed", Scheme: "http", Address: "node.invalid", Port: 2053, BasePath: "/private/", ApiToken: "route-token", Enable: true, AllowPrivateAddress: true, OutboundTag: "via-vpn"}
	if err := (&service.NodeService{}).Create(node); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"id": node.Id, "name": node.Name, "scheme": node.Scheme, "address": node.Address, "port": node.Port, "basePath": node.BasePath, "enable": true, "allowPrivateAddress": true, "outboundTag": node.OutboundTag}
	raw, _ := json.Marshal(payload)
	for _, path := range []string{"/panel/api/nodes/test", "/panel/api/nodes/probe/" + strconv.Itoa(node.Id)} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if !strings.Contains(w.Body.String(), `"status":"online"`) {
			t.Fatalf("%s: %s", path, w.Body.String())
		}
	}
	if requests != 2 {
		t.Fatalf("proxy requests=%d, want 2", requests)
	}
}
