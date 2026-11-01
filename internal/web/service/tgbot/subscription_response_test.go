package tgbot

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestBuildSubscriptionURLsIPv6(t *testing.T) {
	for _, test := range []struct{ domain, port, cert, key, prefix string }{
		{"2001:db8::1", "2096", "", "", "http://[2001:db8::1]:2096/"},
		{"[2001:db8::1]", "80", "", "", "http://[2001:db8::1]/"},
		{"2001:db8::1", "443", "/cert.pem", "/key.pem", "https://[2001:db8::1]/"},
	} {
		t.Run(test.prefix, func(t *testing.T) {
			tb, _ := newLinksCallbackTgbot(t, ownerMail)
			for key, value := range map[string]string{
				"subDomain": test.domain, "subPort": test.port,
				"subCertFile": test.cert, "subKeyFile": test.key, "subURI": "", "subJsonURI": "", "subJsonEnable": "true",
			} {
				setting := model.Setting{Key: key, Value: value}
				if err := database.GetDB().Where("key = ?", key).Assign(setting).FirstOrCreate(&setting).Error; err != nil {
					t.Fatal(err)
				}
			}
			sub, jsonSub, err := tb.buildSubscriptionURLs(ownerMail)
			if err != nil {
				t.Fatal(err)
			}
			for _, generated := range []string{sub, jsonSub} {
				if !strings.HasPrefix(generated, test.prefix) {
					t.Errorf("invalid IPv6 subscription URL %q, want prefix %q", generated, test.prefix)
				}
			}
		})
	}
}

func TestReadClientSubscriptionResponse(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"valid", 200, "vless://client@host:443", false},
		{"not found", 404, "error page", true},
		{"server error", 500, "error page", true},
		{"at limit", 200, strings.Repeat("x", maxClientSubscriptionBytes), false},
		{"oversized", 200, strings.Repeat("x", maxClientSubscriptionBytes+1), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}
			raw, err := readClientSubscriptionResponse(resp)
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v, wantError=%v", err, test.wantError)
			}
			if !test.wantError && string(raw) != test.body {
				t.Fatal("subscription response changed")
			}
		})
	}
}
