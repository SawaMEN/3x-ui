package sub

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
)

func seedVetroffSubscription(t *testing.T) (*model.Inbound, string, string) {
	t.Helper()
	_, subID := initHwidSubRouter(t, 0)
	cert, key := anyTLSCertificate(t, "vpn.example.com")
	var ib model.Inbound
	if err := database.GetDB().Where("tag = ?", "hwid-sub").First(&ib).Error; err != nil {
		t.Fatal(err)
	}
	options := externalvpn.DefaultVetroffSettings()
	options.AutoTLS = false
	options.Transport = "xhttp-stream-up"
	options.XHTTPPath = "/carrier/"
	options.XHTTPHost = "front.example.com"
	options.ProtocolPolicy = "strict"
	options.ConnectTimeout = "12s"
	options.AttemptTimeout = "3s"
	raw, err := json.Marshal(map[string]any{"hostname": "vpn.example.com", "certificate": cert, "privateKey": key, "vetroff": options, "clients": []model.Client{{Email: "route@example.com", Password: strings.Repeat("v", 48), Enable: true, SubID: subID}}})
	if err != nil {
		t.Fatal(err)
	}
	ib.Protocol, ib.Port, ib.Settings = model.VetroffProxy, 8443, string(raw)
	if err := database.GetDB().Save(&ib).Error; err != nil {
		t.Fatal(err)
	}
	return &ib, subID, cert
}

func decodeVetroffProfile(t *testing.T, link string) map[string]any {
	t.Helper()
	if !strings.HasPrefix(link, "vetroff://") || strings.ContainsAny(link, "#?\r\n") {
		t.Fatalf("invalid native link framing: %s", link)
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(link, "vetroff://"))
	if err != nil {
		t.Fatal(err)
	}
	var profile map[string]any
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatal(err)
	}
	if profile["password"] != strings.Repeat("v", 48) || profile["sni"] != "vpn.example.com" || profile["transport"] != "xhttp-stream-up" || profile["xhttp_path"] != "/carrier/" || profile["xhttp_host"] != "front.example.com" || profile["protocol_policy"] != "strict" || profile["connect_timeout"] != "12s" || !strings.HasPrefix(profile["pin_sha256"].(string), "sha256/") {
		t.Fatalf("lost client settings: %s", raw)
	}
	return profile
}

func TestVetroffHTTPSubscriptionAndNativeBinary(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw", true: "base64"}[encrypted], func(t *testing.T) {
			ib, subID, cert := seedVetroffSubscription(t)
			seedHost(t, &model.Host{InboundId: ib.Id, Remark: "Public", Address: "edge.example.com", Port: 9443, Security: "tls"})
			router := gin.New()
			NewSUBController(router.Group("/"), WithSUBPath("/sub/"), WithSUBEncryption(encrypted))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/sub/"+subID, nil)
			req.Header.Set("User-Agent", "VetroffProxy/0.1.0")
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("subscription: %d %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if encrypted {
				decoded, err := base64.StdEncoding.DecodeString(body)
				if err != nil {
					t.Fatal(err)
				}
				body = string(decoded)
			}
			links := strings.Fields(body)
			if len(links) != 1 {
				t.Fatalf("subscription lost or duplicated native profile: %s", body)
			}
			profile := decodeVetroffProfile(t, links[0])
			if profile["servers"].([]any)[0] != "https://edge.example.com:9443" {
				t.Fatalf("host endpoint lost: %+v", profile["servers"])
			}
			if renamed := applyRemarkToLink(links[0], "Renamed native profile"); renamed != links[0] {
				t.Fatal("remark corrupts the strict native URI")
			}
			if binary := os.Getenv("VETROFF_TEST_BINARY"); binary != "" {
				file := filepath.Join(t.TempDir(), "connection.txt")
				if err := os.WriteFile(file, []byte(links[0]), 0o600); err != nil {
					t.Fatal(err)
				}
				command := exec.Command(binary, "check-config", "--link-file", file, "--ca-cert", cert, "--json")
				if out, err := command.CombinedOutput(); err != nil {
					t.Fatalf("official binary rejected HTTP subscription profile: %v: %s", err, out)
				}
			}
		})
	}
}

func TestVetroffMixedNativeSubscriptionCannotSilentlyLoseProtocol(t *testing.T) {
	_, subID, _ := seedVetroffSubscription(t)
	client := model.Client{Email: "route@example.com", Password: strings.Repeat("a", 48), Enable: true, SubID: subID}
	raw, _ := json.Marshal(map[string]any{"clients": []model.Client{client}, "tls": map[string]any{"serverName": "vpn.example.com"}})
	ib := &model.Inbound{UserId: 1, Tag: "mixed-anytls", Protocol: model.AnyTLS, Enable: true, Port: 9443, Settings: string(raw)}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	var record model.ClientRecord
	if err := database.GetDB().Where("sub_id = ?", subID).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.ClientInbound{ClientId: record.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewSubJsonService("", "", "", "", NewSubService(""))
	if body, _, err := svc.GetHiddifySingBoxJson(subID, "sub.example.com"); !errors.Is(err, errSubscriptionFormatUnsupported) || body != "" {
		t.Fatalf("partial native JSON hides VetroffProxy: %s %v", body, err)
	}
	router := gin.New()
	NewSUBController(router.Group("/"), WithSUBPath("/sub/"), WithSUBEncryption(false), WithSUBJsonEnabled(true), WithSUBJsonAutoDetect(true))
	req := httptest.NewRequest(http.MethodGet, "/sub/"+subID, nil)
	req.Header.Set("User-Agent", "Hiddify/2.0")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "vetroff://") || !strings.Contains(rec.Body.String(), "anytls://") {
		t.Fatalf("auto-detection dropped a mixed connection: %d %s", rec.Code, rec.Body.String())
	}
	for _, format := range []string{"xray", "sing-box", "hiddify"} {
		req := httptest.NewRequest(http.MethodGet, "/sub/"+subID+"?format="+format, nil)
		req.Header.Set("User-Agent", "Hiddify/2.0")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "vetroff://") || !strings.Contains(rec.Body.String(), "anytls://") {
			t.Fatalf("explicit %s format dropped a mixed connection: %d %s", format, rec.Code, rec.Body.String())
		}
	}
}

func TestVetroffSubscriptionExcludesDisabledAndExpiredClients(t *testing.T) {
	for _, mode := range []string{"disabled-client", "disabled-inbound", "expired-client"} {
		t.Run(mode, func(t *testing.T) {
			ib, subID, _ := seedVetroffSubscription(t)
			var raw map[string]any
			if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
				t.Fatal(err)
			}
			clients := raw["clients"].([]any)
			c := clients[0].(map[string]any)
			switch mode {
			case "disabled-client":
				c["enable"] = false
			case "expired-client":
				c["expiryTime"] = time.Now().Add(-time.Hour).UnixMilli()
			case "disabled-inbound":
				ib.Enable = false
			}
			encoded, _ := json.Marshal(raw)
			ib.Settings = string(encoded)
			if err := database.GetDB().Save(ib).Error; err != nil {
				t.Fatal(err)
			}
			links, _, _, _, err := NewSubService("").GetSubs(subID, "vpn.example.com")
			if err != nil {
				t.Fatal(err)
			}
			for _, link := range links {
				if strings.Contains(link, "vetroff://") {
					t.Fatalf("inactive credentials exported: %s", mode)
				}
			}
		})
	}
}

func TestVetroffExplicitNativeFormatKeepsSeparateConnection(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw", true: "base64"}[encrypted], func(t *testing.T) {
			ib, subID, _ := seedVetroffSubscription(t)
			seedHost(t, &model.Host{InboundId: ib.Id, Remark: "Public", Address: "edge.example.com", Port: 9443, Security: "tls"})
			router := gin.New()
			NewSUBController(router.Group("/"), WithSUBPath("/sub/"), WithSUBEncryption(encrypted), WithSUBJsonEnabled(true), WithSUBJsonAutoDetect(true))
			for _, format := range []string{"xray", "sing-box", "hiddify"} {
				t.Run(format, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, "/sub/"+subID+"?format="+format, nil)
					req.Header.Set("User-Agent", "Hiddify/2.0")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("native format discarded VetroffProxy: status %d: %s", rec.Code, rec.Body.String())
					}
					body := rec.Body.String()
					if encrypted {
						raw, err := base64.StdEncoding.DecodeString(body)
						if err != nil {
							t.Fatal(err)
						}
						body = string(raw)
					}
					links := strings.Fields(body)
					if len(links) != 1 {
						t.Fatalf("expected one separate VetroffProxy link, got %d: %q", len(links), body)
					}
					profile := decodeVetroffProfile(t, links[0])
					if profile["servers"].([]any)[0] != "https://edge.example.com:9443" {
						t.Fatalf("fallback lost configured Host endpoint: %+v", profile["servers"])
					}
				})
			}
		})
	}
}
