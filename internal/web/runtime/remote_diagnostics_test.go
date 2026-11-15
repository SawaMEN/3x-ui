package runtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/op/go-logging"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

func TestRemoteFailuresLogStatusAndOperationWithoutUntrustedData(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	t.Cleanup(logger.CloseLogger)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("PRIVATE_BODY Bearer secret-response"))
	}))
	defer server.Close()
	host, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	remote := NewRemote(&model.Node{Id: 4321, Scheme: "http", Address: host, Port: port, BasePath: "/PRIVATE_BASE/", ApiToken: "PRIVATE_TOKEN", AllowPrivateAddress: true}, nil)
	if _, err := remote.ListLocalCoverCertificateSources(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	logs := strings.Join(logger.GetFileLogs(100, "error"), "\n")
	for _, want := range []string{"node_id=4321", "operation=ListLocalCoverCertificateSources", "method=GET", "status 403"} {
		if !strings.Contains(logs, want) {
			t.Errorf("missing %q in %s", want, logs)
		}
	}
	for _, secret := range []string{"PRIVATE_BODY", "PRIVATE_BASE", "PRIVATE_TOKEN", "secret-response"} {
		if strings.Contains(logs, secret) {
			t.Errorf("secret leaked: %s", secret)
		}
	}
	for _, err := range []error{&remoteHTTPError{status: 401, snippet: "PRIVATE_BODY"}, &remoteAPIError{msg: "PRIVATE_BODY"}} {
		if strings.Contains(logger.ErrorSummary(fmt.Errorf("outer: %w", err)), "PRIVATE_BODY") {
			t.Fatal("wrapped response leaked")
		}
	}
}
