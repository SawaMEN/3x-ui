package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteSNIJobsUseAuthenticatedBoundedNodeTransport(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing node credential")
		}
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["nodeId"] != nil {
				t.Error("master-local node id forwarded")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"obj":{"id":"job","state":"queued","results":[]}}`))
	}))
	defer server.Close()
	remote := remoteForTestServer(t, server)
	for _, op := range []struct{ method, path string }{{"POST", "jobs"}, {"GET", "jobs/job"}, {"DELETE", "jobs/job"}, {"GET", "history"}} {
		var body any
		if op.method == "POST" {
			body = map[string]string{"mode": "combined"}
		}
		if _, err := remote.RequestSNIScan(context.Background(), op.method, op.path, body); err != nil {
			t.Fatal(err)
		}
	}
	for _, resource := range []string{"jobs/..", "jobs/.", "jobs/"} {
		if _, err := remote.RequestSNIScan(context.Background(), http.MethodGet, resource, nil); err == nil {
			t.Fatalf("accepted invalid resource %q", resource)
		}
	}

	if len(paths) != 4 {
		t.Fatal(paths)
	}
	if _, err := remote.RequestSNIScan(context.Background(), "POST", "../../updatePanel", nil); err == nil {
		t.Fatal("arbitrary node endpoint accepted")
	}
}
