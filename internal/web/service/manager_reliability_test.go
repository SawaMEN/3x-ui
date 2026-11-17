package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func TestManagerSubscriptionEnforcesDeviceLimitAcrossFormats(t *testing.T) {
	s, _, sub := managerFixture(t)
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("id = ?", sub.ClientID).Update("limit_hwid", 1).Error; err != nil {
		t.Fatal(err)
	}
	RegisterSubLinkProvider(managerProvider{})
	t.Cleanup(func() { RegisterSubLinkProvider(nil) })
	request := func(format, hwid string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "https://panel.example/bus/"+sub.Token+"?format="+format, nil)
		r.Header.Set("X-HWID", hwid)
		w := httptest.NewRecorder()
		ServeManagerSubscription(w, r, sub.Token)
		return w
	}
	for _, format := range []string{"raw", "base64", "xray", "singbox", "hiddify"} {
		if w := request(format, ""); w.Code != http.StatusNotFound || w.Header().Get("X-Hwid-Not-Supported") != "true" {
			t.Fatalf("%s without HWID: %d %v", format, w.Code, w.Header())
		}
	}
	if w := request("raw", "device-first"); w.Code != http.StatusOK {
		t.Fatalf("first device: %d %s", w.Code, w.Body.String())
	}
	if w := request("raw", "device-second"); w.Code != http.StatusNotFound || w.Header().Get("X-Hwid-Max-Devices-Reached") != "true" {
		t.Fatalf("second device: %d %v", w.Code, w.Header())
	}
	if w := request("raw", "device-first"); w.Code != http.StatusOK {
		t.Fatalf("returning device: %d", w.Code)
	}
}

func TestManagerFailedGenerationRemovesOnlyStagedListeners(t *testing.T) {
	s, _, sub := managerFixture(t)
	node := &model.Node{Name: "rollback", Address: "node.example.com", Port: 2053, Enable: true, Status: "online"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	mgr := runtime.GetManager()
	mgr.SetRuntimeOverride(node.Id, &fakeNodeRuntime{})
	sub.Entries[0].NodeID = &node.Id
	var err error
	sub, err = s.Save(1, *sub)
	if err != nil {
		t.Fatal(err)
	}
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	old := sub.Published[0].ActiveInboundID
	mgr.SetRuntimeOverride(node.Id, &managerRemoteRestartFailure{})
	for range 2 {
		if op := managerRun(t, s, sub); op.State != "failed" {
			t.Fatalf("failed rotation state: %s %s", op.State, op.Message)
		}
	}
	var listeners []model.Inbound
	if err := database.GetDB().Where("tag LIKE ?", "%manager-"+sub.Token+"-%").Find(&listeners).Error; err != nil {
		t.Fatal(err)
	}
	if len(listeners) != 1 || listeners[0].Id != old {
		t.Fatalf("staged listeners left after rollback: %+v", listeners)
	}
}
