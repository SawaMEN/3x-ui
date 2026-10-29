package sub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSubscriptionOutputFormatRouting(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "formats", "vless", 4591, 1, dnsTestStream)
	unsupported := seedSubInbound(t, "formats", "mtproto", 4592, 2, dnsTestStream)
	if err := database.GetDB().Model(unsupported).Update("protocol", model.MTProto).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode, fallback, query, agent, want string
		code                               int
	}{
		{"both", "auto", "?format=xray", "Hiddify", "\"protocol\": \"vless\"", 200},
		{"both", "auto", "?format=singbox", "Xray", "\"type\": \"vless\"", 200},
		{"both", "sing-box", "", "Xray", "\"type\": \"vless\"", 200},
		{"both", "xray", "?format=links", "sing-box", "vless://", 200},
		{"xray", "xray", "?format=sing-box", "Xray", "", 404},
		{"sing-box", "sing-box", "?format=xray", "Hiddify", "", 404},
		{"xray", "xray", "", "sing-box", "\"protocol\": \"vless\"", 200},
	} {
		t.Run(tc.mode+tc.fallback+tc.query, func(t *testing.T) {
			router := gin.New()
			NewSUBController(router.Group("/"), WithSUBFormats(tc.mode, tc.fallback), WithSUBEncryption(false))
			req := httptest.NewRequest(http.MethodGet, "/sub/formats"+tc.query, nil)
			req.Header.Set("User-Agent", tc.agent)
			if tc.query == "?format=xray" {
				req.Header.Set("Accept", "text/html")
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != tc.code || tc.want != "" && !strings.Contains(resp.Body.String(), tc.want) {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestSubscriptionFormatsApplyWithoutServerRestart(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "live-formats", "vless", 4593, 1, dnsTestStream)
	router := gin.New()
	NewSUBController(router.Group("/"), WithSUBLiveFormats())
	for _, format := range []string{"sing-box", "xray"} {
		if err := database.GetDB().Where("key = ?", "subDefaultFormat").Assign(model.Setting{Key: "subDefaultFormat", Value: format}).FirstOrCreate(&model.Setting{}).Error; err != nil {
			t.Fatal(err)
		}
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/sub/live-formats", nil))
		if resp.Code != 200 {
			t.Fatalf("%s: %d %s", format, resp.Code, resp.Body.String())
		}
		want := "\"protocol\": \"vless\""
		if format == "sing-box" {
			want = "\"type\": \"vless\""
		}
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatal(resp.Body.String())
		}
	}
}

func TestSubscriptionOutputFormatPreservesNativeDNS(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "native-dns", "vless", 4594, 1, dnsTestStream)
	router := gin.New()
	NewSUBController(router.Group("/"), WithSUBFormats("both", "sing-box"), WithSUBJsonDns(`["9.9.9.9"]`))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/sub/native-dns", nil))
	if resp.Code != 200 || !strings.Contains(resp.Body.String(), "9.9.9.9") {
		t.Fatalf("%d %s", resp.Code, resp.Body.String())
	}
}
