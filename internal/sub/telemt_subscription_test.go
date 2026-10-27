package sub

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestTelegramSubscriptionIncludesRemoteProfilesWithoutLocalTelemt(t *testing.T) {
	initSubDB(t)
	const subID = "remote-telegram-sub"
	const personal = "tg://proxy?server=node.example&port=443&secret=ee1234"
	const web = "tg://webproxy?server=web.node.example&secret=dd1234"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/panel/api/telemt/subscription/"+subID || r.Header.Get("Authorization") != "Bearer test-node-token" {
			t.Errorf("unexpected node request: %s, auth=%q", r.URL.Path, r.Header.Get("Authorization"))
			http.Error(w, "wrong request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"obj":{"enabled":true,"personal":{"link":"` + personal + `"},"webProxyEnabled":true,"webProxy":"` + web + `"}}`))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	host, portText, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portText)
	node := &model.Node{Name: "Remote Telegram", Scheme: "http", Address: host, Port: port, ApiToken: "test-node-token", Enable: true, AllowPrivateAddress: true}
	db := database.GetDB()
	if err := db.Create(node).Error; err != nil {
		t.Fatal(err)
	}
	failed := &model.Node{Name: "Unreachable node", Scheme: "http", Address: host, Port: 1, ApiToken: "test-node-token", Enable: true, AllowPrivateAddress: true}
	if err := db.Create(failed).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "telegram@example.com", SubID: subID, Enable: true, Secret: mtprotoTestSecret}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	inbound := &model.Inbound{
		Tag: "remote-mtproto", Enable: true, NodeID: &node.Id, Protocol: model.MTProto, Port: 8443,
		ShareAddrStrategy: "custom", ShareAddr: "mt.node.example",
		Settings: `{"clients":[{"email":"telegram@example.com","subId":"` + subID + `","enable":true,"secret":"` + mtprotoTestSecret + `"}]}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/sub/telemt/:subid", serveTelemtSubscription)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://sub.example/sub/telemt/"+subID, nil))
	var payload struct {
		Enabled bool `json:"enabled"`
		Proxies []struct {
			Name, Host, Link string
			Port             int
		} `json:"proxies"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !payload.Enabled || len(payload.Proxies) != 3 {
		t.Fatalf("subscription lost remote proxies: status=%d body=%s", response.Code, response.Body.String())
	}
	if payload.Proxies[0].Host != "mt.node.example" || payload.Proxies[0].Port != 8443 {
		t.Fatalf("remote inbound endpoint = %+v", payload.Proxies[0])
	}
	if payload.Proxies[1].Link != personal || payload.Proxies[1].Name != node.Name || payload.Proxies[2].Link != web {
		t.Fatalf("remote profiles = %+v", payload.Proxies)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("credentials must not be cached")
	}
	if calls.Load() != 1 {
		t.Fatalf("node requests = %d, want 1", calls.Load())
	}

	unknown := httptest.NewRecorder()
	router.ServeHTTP(unknown, httptest.NewRequest(http.MethodGet, "http://sub.example/sub/telemt/unknown-sub", nil))
	if unknown.Code != http.StatusNotFound || calls.Load() != 1 {
		t.Fatalf("unknown subscription reached nodes: status=%d calls=%d", unknown.Code, calls.Load())
	}
	if err := db.Where("key = ?", "subTelegramRemoteEnable").Assign(model.Setting{Value: "false"}).FirstOrCreate(&model.Setting{Key: "subTelegramRemoteEnable"}).Error; err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://sub.example/sub/telemt/"+subID, nil))
	if calls.Load() != 1 {
		t.Fatal("remote page toggle still queried nodes")
	}
	payload.Proxies = nil
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Enabled || len(payload.Proxies) != 0 {
		t.Fatalf("remote page toggle leaked mirrored links: %s", response.Body.String())
	}
	if err := db.Model(node).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://sub.example/sub/telemt/"+subID, nil))
	if calls.Load() != 1 {
		t.Fatal("disabled node was queried")
	}
}

func TestTelegramSubscriptionNodeDeadline(t *testing.T) {
	initSubDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	host, portText, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portText)
	node := &model.Node{Name: "Slow Telegram", Scheme: "http", Address: host, Port: port, ApiToken: "token", Enable: true, AllowPrivateAddress: true}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	profiles := remoteTelegramSubscriptions(ctx, "slow-sub")
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("node request ignored cancellation: %s", elapsed)
	}
	for _, result := range profiles {
		if len(result.Proxies) != 0 {
			t.Fatalf("slow node returned credentials: %+v", result)
		}
	}
}

func TestTelegramPublicationAndPageVisibility(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	client := &model.ClientRecord{Email: "visibility@example.com", SubID: "visibility-sub", Enable: true, Secret: mtprotoTestSecret}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	nodeID := 99
	inbound := &model.Inbound{Tag: "visible-telegram", Enable: true, NodeID: &nodeID, Protocol: model.MTProto, Port: 8443, ShareAddrStrategy: "custom", ShareAddr: "node.example", Settings: `{}`}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatal(err)
	}
	page := func() []string {
		t.Helper()
		request := NewSubService("").ForRequest("parent.example")
		request.subscriptionBody = false
		links, emails, _, _, err := request.getSubs(client.SubID)
		if err != nil {
			t.Fatal(err)
		}
		if len(emails) != 1 || emails[0] != client.Email {
			t.Fatalf("visibility lost subscription identity: %v", emails)
		}
		return links
	}
	if links := page(); len(links) != 1 {
		t.Fatalf("legacy inbound should remain visible: %v", links)
	}
	if err := db.Model(inbound).Update("settings", `{"showOnParent":false}`).Error; err != nil {
		t.Fatal(err)
	}
	if links := page(); len(links) != 0 {
		t.Fatalf("hidden parent inbound exposed: %v", links)
	}
	request := NewSubService("").ForRequest("parent.example")
	request.subscriptionBody = true
	raw, _, _, _, err := request.getSubs(client.SubID)
	if err != nil || len(raw) != 0 {
		t.Fatalf("parent publication bypass through raw links: %v %v", raw, err)
	}
	if err := db.Model(inbound).Update("settings", `{"showOnParent":true}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("key = ?", "subTelegramRemoteEnable").Assign(model.Setting{Value: "false"}).FirstOrCreate(&model.Setting{Key: "subTelegramRemoteEnable"}).Error; err != nil {
		t.Fatal(err)
	}
	if links := page(); len(links) != 0 {
		t.Fatalf("remote page toggle ignored: %v", links)
	}
	request = NewSubService("").ForRequest("parent.example")
	request.subscriptionBody = true
	raw, _, _, _, err = request.getSubs(client.SubID)
	if err != nil || len(raw) != 1 {
		t.Fatalf("page-only toggle changed raw subscription: %v %v", raw, err)
	}
	if err := db.Model(inbound).Update("node_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if links := page(); len(links) != 1 {
		t.Fatalf("remote toggle hid local proxy: %v", links)
	}
	if err := db.Where("key = ?", "subTelegramLocalEnable").Assign(model.Setting{Value: "false"}).FirstOrCreate(&model.Setting{Key: "subTelegramLocalEnable"}).Error; err != nil {
		t.Fatal(err)
	}
	if links := page(); len(links) != 0 {
		t.Fatalf("local page toggle ignored: %v", links)
	}
	if err := db.Model(&model.Setting{}).Where("key = ?", "subTelegramLocalEnable").Update("value", "true").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(inbound).Update("settings", `{"showInSubscription":false}`).Error; err != nil {
		t.Fatal(err)
	}
	if links := page(); len(links) != 0 {
		t.Fatalf("inbound subscription toggle ignored: %v", links)
	}
	if err := db.Model(inbound).Update("settings", `{"webProxy":{"enabled":true,"domain":"web.example"}}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("key = ?", "subTelegramWebEnable").Assign(model.Setting{Value: "false"}).FirstOrCreate(&model.Setting{Key: "subTelegramWebEnable"}).Error; err != nil {
		t.Fatal(err)
	}
	if links := page(); len(links) != 0 {
		t.Fatalf("WEB toggle ignored: %v", links)
	}
}
