package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/wirecodec"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func TestCoverCertificateFleetValidationAndRemoval(t *testing.T) {
	setupBulkDB(t)
	for _, input := range []struct {
		source  int
		domain  string
		targets []int
	}{
		{-1, "web.example.org", nil},
		{0, "https://example.org", nil},
		{0, "web.example.org", []int{0}},
		{0, "web.example.org", []int{-1}},
		{0, "web.example.org", make([]int, 33)},
	} {
		if _, err := SaveCoverCertificateFleet(context.Background(), input.source, input.domain, input.targets); err == nil {
			t.Fatalf("accepted invalid enrollment: %+v", input)
		}
	}
	// An offline or deleted source must not prevent stopping delivery.
	if err := saveFirewallJSON(coverCertificateFleetKey, []CoverCertificateFleet{{SourceNodeID: 77, Domain: "web.example.org", Targets: []CoverCertificateTarget{{NodeID: 8, State: "retrying"}}}}); err != nil {
		t.Fatal(err)
	}
	rows, err := SaveCoverCertificateFleet(context.Background(), 77, "WEB.EXAMPLE.ORG.", nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("remove unavailable source: %+v %v", rows, err)
	}
	stored, err := CoverCertificateFleets()
	if err != nil || len(stored) != 0 {
		t.Fatalf("persisted removal: %+v %v", stored, err)
	}
}

func TestCoverCertificateFleetRetriesWithoutLosingRecipients(t *testing.T) {
	setupBulkDB(t)
	rows := []CoverCertificateFleet{{SourceNodeID: 0, Domain: "web.example.org", Targets: []CoverCertificateTarget{{NodeID: 8, State: "pending"}, {NodeID: 9, State: "pending"}}}}
	if err := saveFirewallJSON(coverCertificateFleetKey, rows); err != nil {
		t.Fatal(err)
	}
	// No site/cert exists. Both recipients must remain enrolled and retryable,
	// not be reported as ready or lost when issuance is still pending.
	syncCoverCertificateFleet(context.Background())
	after, err := CoverCertificateFleets()
	if err != nil || len(after) != 1 || len(after[0].Targets) != 2 {
		t.Fatalf("recipients lost: %+v %v", after, err)
	}
	for _, dst := range after[0].Targets {
		if dst.State != "retrying" || dst.DeliveredHash != "" || dst.CheckedAt == 0 || dst.ExpiresAt != 0 {
			t.Fatalf("bad failure state: %+v", dst)
		}
	}
}

func TestCoverCertificateExportRefusesUnownedOrDisabledSite(t *testing.T) {
	setupBulkDB(t)
	for _, enabled := range []bool{false, true} {
		cfg := domainfront.DefaultConfig()
		cfg.Domains = []domainfront.DomainFrontDomain{{Name: "web.example.org", Enabled: enabled, Mode: "direct", TLS: "auto"}}
		if err := saveFirewallJSON("domainFrontConfig", cfg); err != nil {
			t.Fatal(err)
		}
		result, err := ExportLocalCoverCertificate(context.Background(), "web.example.org")
		if err == nil || result.PrivateKeyPEM != "" || result.CertificatePEM != "" {
			t.Fatalf("exported unrelated identity: %+v %v", result, err)
		}
	}
}

func TestDistributedCertificateConsumerMatching(t *testing.T) {
	cert := DistributedTLSCertificate{Domain: "web.site.ru", CertFile: "/private/current/fullchain.pem", KeyFile: "/private/current/privkey.pem"}
	for _, tc := range []struct {
		raw  string
		used bool
	}{
		{`{"tlsSettings":{"certificates":[{"certificateFile":"/private/current/fullchain.pem"}]}}`, true},
		{`{"hostname":"web.site.ru"}`, true},
		{`{"certificate":"/private/current/fullchain.pem.bak"}`, false},
		{`{"serverName":"another.site.ru"}`, false},
	} {
		var value any
		if err := json.Unmarshal([]byte(tc.raw), &value); err != nil {
			t.Fatal(err)
		}
		if distributedCertificateUsed(value, cert) != tc.used {
			t.Fatalf("wrong consumer decision for %s", tc.raw)
		}
	}
}

// Run in a fresh process so the test-only CA store never changes another
// test's trust decisions (x509 caches system roots process-wide).
func TestCoverCertificateFleetDeliveryAndRenewal(t *testing.T) {
	if os.Getenv("XUI_FLEET_TRUST_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCoverCertificateFleetDeliveryAndRenewal$", "-test.count=1")
		cmd.Env = append(os.Environ(), "XUI_FLEET_TRUST_TEST=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("delivery integration: %v\n%s", err, out)
		}
		return
	}
	setupBulkDB(t)
	useTestRuntimeManager(t)
	host := "web.site.ru"
	cert1, key1 := testDistributionPair(t, host, time.Now().Add(24*time.Hour))
	cert2, key2 := testDistributionPair(t, host, time.Now().Add(48*time.Hour))
	roots := filepath.Join(t.TempDir(), "roots.pem")
	if err := os.WriteFile(roots, append(append([]byte{}, cert1...), cert2...), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", roots)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	payload := runtime.TLSCertificateInstall{Domain: host, CertificatePEM: string(cert1), PrivateKeyPEM: string(key1)}
	var payloadMu sync.Mutex
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/panel/api/domains/certificates/export-cover" || r.Header.Get(wirecodec.MasterPushHeader) != "1" {
			http.Error(w, "wrong source request", http.StatusBadRequest)
			return
		}
		payloadMu.Lock()
		defer payloadMu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": payload})
	}))
	defer source.Close()
	var installs atomic.Int32
	var rejectApply atomic.Bool
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/panel/api/domains/certificates/install" || r.Header.Get(wirecodec.MasterPushHeader) != "1" {
			http.Error(w, "wrong recipient request", http.StatusBadRequest)
			return
		}
		var incoming runtime.TLSCertificateInstall
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			t.Error(err)
			http.Error(w, "bad input", http.StatusBadRequest)
			return
		}
		if !incoming.Apply {
			t.Error("renewed identity must request runtime apply")
		}
		installed, err := InstallAndApplyDistributedTLSCertificate(r.Context(), incoming)
		if err != nil {
			t.Error(err)
			http.Error(w, "install failed", http.StatusInternalServerError)
			return
		}
		installs.Add(1)
		if rejectApply.Load() {
			installed.Applied = false
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": installed})
	}))
	defer target.Close()
	addNode := func(server *httptest.Server) int {
		address, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
		port, _ := strconv.Atoi(portText)
		sum := sha256.Sum256(server.Certificate().Raw)
		node := model.Node{Name: "fleet-test-" + portText, Scheme: "https", Address: address, Port: port, BasePath: "/", ApiToken: "test-only", Enable: true, AllowPrivateAddress: true, TlsVerifyMode: "pin", PinnedCertSha256: base64.StdEncoding.EncodeToString(sum[:])}
		if err := database.GetDB().Create(&node).Error; err != nil {
			t.Fatal(err)
		}
		return node.Id
	}
	sourceID, targetID := addNode(source), addNode(target)
	if err := saveFirewallJSON(coverCertificateFleetKey, []CoverCertificateFleet{{SourceNodeID: sourceID, Domain: host, Targets: []CoverCertificateTarget{{NodeID: targetID, State: "pending"}}}}); err != nil {
		t.Fatal(err)
	}
	syncCoverCertificateFleet(context.Background())
	rows, err := CoverCertificateFleets()
	if err != nil || len(rows) != 1 || rows[0].Targets[0].State != "ready" || installs.Load() != 1 {
		t.Fatalf("initial delivery: %+v %v installs=%d", rows, err, installs.Load())
	}
	originalHash := rows[0].Targets[0].DeliveredHash
	syncCoverCertificateFleet(context.Background())
	if installs.Load() != 1 {
		t.Fatal("unchanged certificate was redelivered")
	}
	payloadMu.Lock()
	payload.CertificatePEM, payload.PrivateKeyPEM = string(cert2), string(key2)
	payloadMu.Unlock()
	rejectApply.Store(true)
	syncCoverCertificateFleet(context.Background())
	rows, err = CoverCertificateFleets()
	if err != nil || rows[0].Targets[0].State != "retrying" || rows[0].Targets[0].DeliveredHash != originalHash {
		t.Fatalf("unacknowledged apply reported ready: %+v %v", rows, err)
	}
	rejectApply.Store(false)
	syncCoverCertificateFleet(context.Background())
	rows, err = CoverCertificateFleets()
	if err != nil || installs.Load() != 3 || rows[0].Targets[0].State != "ready" || rows[0].Targets[0].DeliveredHash == originalHash {
		t.Fatalf("renewal delivery: %+v %v installs=%d", rows, err, installs.Load())
	}
	copies := LocalDistributedTLSCertificates()
	if len(copies) != 1 || copies[0].Domain != host || copies[0].ExpiresAt != rows[0].Targets[0].ExpiresAt {
		t.Fatalf("recipient inventory: %+v", copies)
	}
	// The inbound workflow can use an already managed main-node certificate
	// without a cover site, creating additive DNS-01 renewal enrollment.
	if err := saveFirewallJSON(coverCertificateFleetKey, []CoverCertificateFleet{}); err != nil {
		t.Fatal(err)
	}
	mainDir := t.TempDir()
	mainCert, mainKey := filepath.Join(mainDir, "fullchain.pem"), filepath.Join(mainDir, "privkey.pem")
	if err := os.WriteFile(mainCert, cert2, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainKey, key2, 0600); err != nil {
		t.Fatal(err)
	}
	settings := &SettingService{}
	for k, v := range map[string]string{"webDomain": host, "webCertFile": mainCert, "webKeyFile": mainKey} {
		if err := settings.setString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := (&ManagerService{}).PrepareInboundCertificate(context.Background(), 1, InboundCertificateRequest{NodeID: targetID, Domain: host})
	if err != nil || prepared.State != "pending" || prepared.CertFile != "" {
		t.Fatalf("one-click enrollment: %+v %v", prepared, err)
	}
	rows, _ = CoverCertificateFleets()
	if len(rows) != 1 || !rows[0].MainCertificate || rows[0].SourceNodeID != 0 {
		t.Fatalf("main certificate enrollment: %+v", rows)
	}
	syncCoverCertificateFleet(context.Background())
	rows, _ = CoverCertificateFleets()
	if rows[0].Targets[0].State != "ready" {
		t.Fatalf("main certificate was not delivered: %+v", rows)
	}

}
