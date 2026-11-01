package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"
)

func TestSNIScanControllerModes(t *testing.T) {
	logger.InitLogger(logging.ERROR)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	controller := &ServerController{}
	engine.POST("/panel/api/server/scanRealityTargets", controller.scanRealityTargets)
	for _, tc := range []struct {
		mode, targets, reason string
		canceled              bool
		success               bool
	}{
		{mode: "nearby", targets: "127.0.0.1", reason: "must be public"},
		{mode: "nearby", targets: "8.8.8.0/24", reason: "invalid target host"},
		{mode: "unknown", targets: "8.8.8.8", reason: "unknown SNI scan mode"},
		{mode: "nearby", targets: "8.8.8.8", canceled: true, reason: "context canceled"},
		{mode: "candidates", targets: "8.8.8.8", canceled: true, reason: "context canceled"},
		{mode: "", targets: "127.0.0.1", success: true},
		{mode: "candidates", targets: "127.0.0.1", success: true},
	} {
		t.Run(tc.mode+"/"+tc.reason, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/panel/api/server/scanRealityTargets", strings.NewReader(url.Values{"mode": {tc.mode}, "targets": {tc.targets}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.canceled {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			env := doGeodataReq(t, engine, req)
			if env.Success != tc.success || !strings.Contains(env.Msg, tc.reason) {
				t.Fatalf("%+v", env)
			}
			if tc.success && string(env.Obj) == "null" {
				t.Fatal("candidate mode lost legacy result array")
			}
		})
	}
}
