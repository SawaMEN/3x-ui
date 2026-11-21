package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteTelemtWebUsesNodeInboundID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing node credential")
		}
		if got := r.URL.Query().Get("inboundId"); got != "2" {
			t.Errorf("inboundId = %q, want node ID 2", got)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /panel/api/telemt/webproxy/status":
		case "POST /panel/api/telemt/webproxy/preflight":
			var body struct {
				Domain string `json:"domain"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Domain != "edge.example.com" {
				t.Errorf("preflight body = %+v, err = %v", body, err)
			}
		default:
			t.Errorf("unexpected node request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":{"ready":true}}`))
	}))
	defer server.Close()

	remote := remoteForTestServer(t, server)
	const tag = "in-443-tcp"
	remote.cacheSet(tag, 2)
	if _, err := remote.TelemtWebStatus(context.Background(), tag); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.TelemtWebPreflight(context.Background(), tag, map[string]string{"domain": "edge.example.com"}); err != nil {
		t.Fatal(err)
	}
}
