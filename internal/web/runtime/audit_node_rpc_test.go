package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditRemoteLookupFailuresDoNotMutateOrSucceed(t *testing.T) {
	for _, failure := range []string{"http", "api", "json"} {
		for _, operation := range []string{"update", "delete"} {
			t.Run(failure+"/"+operation, func(t *testing.T) {
				var mutations atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if req.URL.Path != "/panel/api/inbounds/list" {
						mutations.Add(1)
						writeOKEnvelope(w)
						return
					}
					switch failure {
					case "http":
						http.Error(w, "node temporarily unavailable", http.StatusServiceUnavailable)
					case "api":
						_, _ = w.Write([]byte(`{"success":false,"msg":"database read failed"}`))
					case "json":
						_, _ = w.Write([]byte(`{"success":true,"obj":"not a list"}`))
					}
				}))
				defer server.Close()
				remote := remoteForTestServer(t, server)
				ib := &model.Inbound{Tag: "existing", Protocol: model.VLESS}
				var err error
				if operation == "update" {
					err = remote.UpdateInbound(context.Background(), ib, ib)
				} else {
					err = remote.DelInbound(context.Background(), ib)
				}
				if err == nil || mutations.Load() != 0 {
					t.Fatalf("lookup failure: err=%v, mutations=%d; want error and no mutation", err, mutations.Load())
				}
			})
		}
	}
}

func TestAuditRemoteConfirmedAbsenceKeepsUpsertAndIdempotentDelete(t *testing.T) {
	var adds atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/panel/api/inbounds/list":
			_, _ = w.Write([]byte(`{"success":true,"obj":[]}`))
		case "/panel/api/inbounds/add":
			adds.Add(1)
			writeOKEnvelope(w)
		default:
			t.Errorf("unexpected mutation: %s", req.URL.Path)
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	remote := remoteForTestServer(t, server)
	ib := &model.Inbound{Tag: "absent", Protocol: model.VLESS}
	remote.recordPushedInbound(ib)
	remote.adoptedAliases[ib.Tag] = "previously-deployed"
	remote.adoptedAliases["unrelated"] = "different-missing-tag"
	if err := remote.DelInbound(context.Background(), ib); err != nil {
		t.Fatal(err)
	}
	if _, found := remote.pushedFP[ib.Tag]; found {
		t.Error("confirmed deletion retained old fingerprint")
	}
	if _, found := remote.adoptedAliases[ib.Tag]; found {
		t.Error("confirmed deletion retained obsolete adopted association")
	}
	if remote.adoptedAliases["unrelated"] != "different-missing-tag" {
		t.Error("deletion of absent inbound discarded unrelated missing alias")
	}
	if err := remote.UpdateInbound(context.Background(), ib, ib); err != nil {
		t.Fatal(err)
	}
	if adds.Load() != 1 {
		t.Fatalf("adds=%d, want 1", adds.Load())
	}
}

func TestAuditRemoteRestartFallbackUsesHTTPStatus(t *testing.T) {
	for _, failure := range []string{"HTTP 404 in body", "HTTP 405 in API error"} {
		t.Run(failure, func(t *testing.T) {
			var legacy atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if strings.HasSuffix(req.URL.Path, "/restartXrayService") {
					legacy.Add(1)
					writeOKEnvelope(w)
					return
				}
				if strings.HasSuffix(failure, "body") {
					http.Error(w, failure, http.StatusInternalServerError)
				} else {
					_, _ = fmt.Fprintf(w, `{"success":false,"msg":%q}`, failure)
				}
			}))
			defer server.Close()
			err := remoteForTestServer(t, server).RestartXray(context.Background())
			if err == nil || legacy.Load() != 0 {
				t.Fatalf("err=%v legacy restarts=%d; want original failure and no fallback", err, legacy.Load())
			}
		})
	}
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var legacy atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if strings.HasSuffix(req.URL.Path, "/restartCoreService") {
					w.WriteHeader(status)
					return
				}
				legacy.Add(1)
				writeOKEnvelope(w)
			}))
			defer server.Close()
			if err := remoteForTestServer(t, server).RestartXray(context.Background()); err != nil || legacy.Load() != 1 {
				t.Fatalf("legacy status %d: err=%v, fallback calls=%d", status, err, legacy.Load())
			}
		})
	}
}

func TestAuditRemoteHTTPStatusClassification(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		err := fmt.Errorf("wrapped: %w", &remoteHTTPError{status: status})
		if !IsRemoteHTTPStatus(err, status) || IsRemoteHTTPStatus(err, http.StatusInternalServerError) {
			t.Fatalf("wrong classification for %v", err)
		}
	}
	for _, err := range []error{nil, errors.New("HTTP 404"), &remoteAPIError{msg: "HTTP 404"}} {
		if IsRemoteHTTPStatus(err, http.StatusNotFound) {
			t.Fatalf("classified plain error as HTTP response: %v", err)
		}
	}
}

func TestAuditRemoteMutationForgetsOldTagAliases(t *testing.T) {
	for _, operation := range []string{"delete", "rename"} {
		for _, alias := range []string{"prefix", "adopted"} {
			t.Run(operation+"/"+alias, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if req.URL.Path == "/panel/api/inbounds/list" {
						t.Error("mutation should use the seeded remote ID")
					}
					writeOKEnvelope(w)
				}))
				defer server.Close()
				remote := remoteForTestServer(t, server)
				old := &model.Inbound{Tag: "n1-old", Protocol: model.VLESS}
				deployedTag := "old"
				if alias == "adopted" {
					deployedTag = "legacy"
					remote.AdoptInboundAlias(old, RemoteInboundOption{Id: 7, Tag: deployedTag})
				} else {
					remote.cacheSet(deployedTag, 7)
					remote.recordPushedInbound(old)
				}
				remote.cacheSet("unrelated", 99)
				replacement := *old
				replacement.Tag = "n1-new"
				var err error
				if operation == "delete" {
					err = remote.DelInbound(context.Background(), old)
				} else {
					err = remote.UpdateInbound(context.Background(), old, &replacement)
				}
				if err != nil {
					t.Fatal(err)
				}
				for _, tag := range []string{old.Tag, deployedTag} {
					if id, found := remote.cacheGetTag(tag); found {
						t.Errorf("old tag %q still resolves to %d after %s", tag, id, operation)
					}
				}
				if len(remote.AdoptedInboundAliases()) != 0 {
					t.Error("mutation retained obsolete adopted association")
				}
				if _, found := remote.pushedFP[old.Tag]; found {
					t.Error("mutation retained old reconcile fingerprint")
				}
				if id, found := remote.cacheGet("unrelated"); !found || id != 99 {
					t.Error("mutation invalidated an unrelated inbound")
				}
				if operation == "rename" {
					if id, found := remote.cacheGetTag(replacement.Tag); !found || id != 7 {
						t.Error("rename lost new tag association")
					}
				}
			})
		}
	}
}
