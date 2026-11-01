package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"
)

type sniRemoteStub struct {
	webruntime.Runtime
	calls   int
	request service.SNIScanRequest
}

func (s *sniRemoteStub) Name() string { return "test-node" }
func (s *sniRemoteStub) RequestSNIScan(ctx context.Context, method, resource string, body any) (json.RawMessage, error) {
	s.calls++
	if req, ok := body.(service.SNIScanRequest); ok {
		s.request = req
	}
	return json.RawMessage(`{"id":"remote-job","state":"queued","results":[]}`), nil
}
func TestSNIJobRoutingKeepsNodeExecutionAndScope(t *testing.T) {
	logger.InitLogger(logging.ERROR)
	gin.SetMode(gin.TestMode)
	manager := webruntime.NewManager(webruntime.LocalDeps{})
	previous := webruntime.GetManager()
	webruntime.SetManager(manager)
	defer webruntime.SetManager(previous)
	stub := &sniRemoteStub{}
	manager.SetRuntimeOverride(7, stub)
	for _, scope := range []string{"", model.ApiScopeNodeSync} {
		before := stub.calls
		engine := gin.New()
		engine.Use(func(c *gin.Context) {
			if scope != "" {
				c.Set("api_token_scope", scope)
			}
		})
		(&ServerController{}).initSNIJobRouter(engine.Group("/panel/api/server"))
		req := httptest.NewRequest(http.MethodPost, "/panel/api/server/sni/jobs", strings.NewReader(`{"nodeId":7,"mode":"combined"}`))
		req.Header.Set("Content-Type", "application/json")
		env := doGeodataReq(t, engine, req)
		if scope == "" {
			if !env.Success || stub.calls != before+1 || stub.request.NodeID != nil {
				t.Fatalf("%+v %+v", env, stub)
			}
		} else {
			if env.Success || stub.calls != before {
				t.Fatal("node-sync credential relayed a scan")
			}
		}
		req = httptest.NewRequest(http.MethodGet, "/panel/api/server/sni/jobs/remote-job?nodeId=7", nil)
		env = doGeodataReq(t, engine, req)
		if (scope == "") != env.Success {
			t.Fatalf("scope=%s %+v", scope, env)
		}
	}
}

func TestSNIJobLocalHTTPProgressAndFailure(t *testing.T) {
	logger.InitLogger(logging.ERROR)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	(&ServerController{}).initSNIJobRouter(engine.Group("/panel/api/server"))
	req := httptest.NewRequest(http.MethodPost, "/panel/api/server/sni/jobs", strings.NewReader(`{"mode":"nearby","targets":"127.0.0.1","purpose":"reality"}`))
	req.Header.Set("Content-Type", "application/json")
	envelope := doGeodataReq(t, engine, req)
	if !envelope.Success {
		t.Fatalf("%+v", envelope)
	}
	var view service.SNIJobSnapshot
	if err := json.Unmarshal(envelope.Obj, &view); err != nil || view.ID == "" {
		t.Fatalf("%+v %v", view, err)
	}
	id := view.ID
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		envelope = doGeodataReq(t, engine, httptest.NewRequest(http.MethodGet, "/panel/api/server/sni/jobs/"+id, nil))
		if err := json.Unmarshal(envelope.Obj, &view); err != nil {
			t.Fatal(err)
		}
		if view.State == "failed" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if view.State != "failed" || !strings.Contains(view.Error, "must be public") {
		t.Fatalf("%+v", view)
	}
	envelope = doGeodataReq(t, engine, httptest.NewRequest(http.MethodDelete, "/panel/api/server/sni/jobs/"+id, nil))
	if !envelope.Success {
		t.Fatal(envelope.Msg)
	}
}
