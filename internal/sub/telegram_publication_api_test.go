package sub_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sub"
	"github.com/SawaMEN/3x-ui/v3/internal/web/controller"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestPublishedTelegramAPIUsesOnlyAllowedLocalInbounds(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB(); service.RegisterSubLinkProvider(nil) })
	service.RegisterSubLinkProvider(sub.NewLinkProvider())
	db := database.GetDB()
	client := &model.ClientRecord{Email: "current@example.com", SubID: "owned-sub", Enable: true, Secret: "dd0123456789abcdef0123456789abcdef"}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	disabled := &model.ClientRecord{Email: "disabled@example.com", SubID: "disabled-sub", Enable: true, Secret: "ddffffffffffffffffffffffffffffffff"}
	if err := db.Create(disabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(disabled).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	for index, settings := range []string{`{"showOnParent":true,"showInSubscription":false}`, `{"showOnParent":false}`, `{}`} {
		inbound := &model.Inbound{Tag: "publication-" + strconv.Itoa(index), Remark: "Published", Protocol: model.MTProto, Enable: true, Port: 8443, Settings: settings, ShareAddrStrategy: "custom", ShareAddr: "public.example"}
		if index == 2 {
			id := 42
			inbound.NodeID = &id
		}
		if err := db.Create(inbound).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			if err := db.Create(&model.ClientInbound{ClientId: disabled.Id, InboundId: inbound.Id}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller.NewTelemtController(router.Group("/telemt"), service.SettingService{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://control.internal/telemt/published", nil))
	var payload struct {
		Success bool                             `json:"success"`
		Obj     []coremodel.TelegramProxyProfile `json:"obj"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Success || len(payload.Obj) != 1 {
		t.Fatalf("published inventory=%s", response.Body.String())
	}
	u, err := url.Parse(payload.Obj[0].Link)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("server") != "public.example" || u.Query().Get("secret") != client.Secret {
		t.Fatalf("wrong normalized client endpoint: %v", u)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("published credentials must not be cached")
	}
}

func TestParentTelegramInventoryWorksWithoutLocalTelemt(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/panel/api/telemt/published" || r.Header.Get("Authorization") != "Bearer node-token" {
			t.Errorf("unexpected node RPC %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"obj":[{"name":"MTProto","link":"tg://proxy?server=public.example&port=443&secret=dd1234"},{"name":"invalid","link":"javascript:alert(1)"}]}`))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	host, portText, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portText)
	node := &model.Node{Name: "Published node", Scheme: "http", Address: host, Port: port, ApiToken: "node-token", Enable: true, AllowPrivateAddress: true}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	controller.NewTelemtController(router.Group("/telemt"), service.SettingService{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://parent.example/telemt/remote-proxies", nil))
	var payload struct {
		Success bool                            `json:"success"`
		Obj     []coremodel.RemoteTelegramProxy `json:"obj"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Success || len(payload.Obj) != 1 || payload.Obj[0].NodeID != node.Id || payload.Obj[0].Profile.Host != "public.example" {
		t.Fatalf("remote inventory=%s", response.Body.String())
	}
}
