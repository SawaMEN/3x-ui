package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestSubscriptionReportKeepsSupportedLinksAndExplainsUnsupported(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	rows, _, warnings, err := parseOutboundSubscriptionReport([]byte("vless://11111111-1111-4111-8111-111111111111@example.com:443?security=tls#valid\nnot-a-protocol://user:private-password@example.org#unsupported\nssh://user:pass@example.com:22#ssh"), true)
	if err != nil || len(rows) != 2 || len(warnings) != 1 {
		t.Fatalf("rows=%v warnings=%v err=%v", rows, warnings, err)
	}
	if !strings.Contains(warnings[0], "not-a-protocol") || strings.Contains(warnings[0], "private-password") {
		t.Fatal(warnings)
	}
}

func TestSubscriptionReportNativeSettingsRemainVerbatim(t *testing.T) {
	rows, _, _, err := parseOutboundSubscriptionReport([]byte(`[{"type":"naive","tag":"naive","server":"example.com","insecure_concurrency":12,"quic":true,"extra_headers":{"Header":"value"},"tls":{"enabled":true}},{"type":"future-protocol","tag":"future","custom":{"nested":[1,2]}}]`), true)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	settings := rows[0]["settings"].(map[string]any)
	if settings["insecure_concurrency"] != float64(12) || settings["quic"] != true || settings["extra_headers"].(map[string]any)["Header"] != "value" {
		t.Fatal(settings)
	}
	if rows[1]["settings"].(map[string]any)["custom"] == nil {
		t.Fatal("future settings were dropped")
	}
}

func TestSubscriptionDependencyCyclesAndDependentsAreExcluded(t *testing.T) {
	source := []any{
		map[string]any{"tag": "a", "protocol": "singbox:socks", "settings": map[string]any{"detour": "b"}},
		map[string]any{"tag": "b", "protocol": "singbox:socks", "settings": map[string]any{"detour": "a"}},
		map[string]any{"tag": "chain", "protocol": "singbox:socks", "settings": map[string]any{"detour": "b"}},
		map[string]any{"tag": "valid", "protocol": "singbox:socks"},
	}
	kept, issues := pruneSubscriptionDependencies(source, source)
	if len(kept) != 1 || kept[0].(map[string]any)["tag"] != "valid" || len(issues) != 3 {
		t.Fatal(kept, issues)
	}
}

func writeSubscriptionCheckBinary(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := os.WriteFile(filepath.Join(dir, singbox.GetBinaryName()), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestSubscriptionKernelValidationRejectsSuccessfulInvalidPlaceholder(t *testing.T) {
	writeSubscriptionCheckBinary(t, "echo 'ERROR unknown outbound type; private-password' >&2\nexit 0\n")
	row := map[string]any{"protocol": "singbox:future-protocol", "tag": "future", "settings": map[string]any{"password": "private-password"}}
	err := validateSubscriptionNativeMember(row, CoreTypeHiddify)
	if err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatalf("diagnostic=%v", err)
	}
}

func TestSubscriptionKernelValidationPassesAllSettingsAndDoesNotStartCore(t *testing.T) {
	writeSubscriptionCheckBinary(t, "[ \"$1\" = check ] && [ \"$2\" = -c ] || exit 6\ncat \"$3\" > \"$XUI_BIN_FOLDER/checked.json\"\n")
	row := map[string]any{"protocol": "singbox:naive", "tag": "proxy", "settings": map[string]any{"server": "example.com", "server_port": 443, "username": "u", "password": "private-password", "insecure_concurrency": 12, "tls": map[string]any{"enabled": true}}}
	if err := validateSubscriptionNativeMember(row, CoreTypeHiddify); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(singbox.GetBinaryPath()), "checked.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["inbounds"] != nil {
		t.Fatal("validation created listeners")
	}
	out := cfg["outbounds"].([]any)[1].(map[string]any)
	if out["insecure_concurrency"] != float64(12) || out["password"] != "private-password" {
		t.Fatal(out)
	}
}

func TestSubscriptionCompatibilityIssuesAreReturnedToUI(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	kept, issues, _ := filterSubscriptionOutboundsForCore("test", CoreTypeHiddify, []any{map[string]any{"protocol": "loopback", "tag": "unsupported", "settings": map[string]any{"inboundTag": "x"}}})
	if len(kept) != 0 || len(issues) != 1 {
		t.Fatal(kept, issues)
	}
}

func TestSubscriptionUnsupportedRefreshKeepsWorkingSourceAsWarning(t *testing.T) {
	setupSettingTestDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	id := serveOutboundSubscription(t, "sub-", func(n int) string {
		if n == 1 {
			return "vless://11111111-1111-4111-8111-111111111111@example.com:443?security=tls#valid"
		}
		return "not-supported://secret@example.com"
	})
	svc := &OutboundSubscriptionService{}
	first, err := svc.Refresh(id)
	if err != nil || len(first) != 1 {
		t.Fatal(first, err)
	}
	before, _ := svc.Get(id)
	second, err := svc.Refresh(id)
	if err != nil || len(second) != 1 {
		t.Fatal(second, err)
	}
	after, _ := svc.Get(id)
	if after.LastFetchedOutbounds != before.LastFetchedOutbounds || after.LastError != "" || !strings.Contains(after.LastWarning, "сохранены") {
		t.Fatalf("%+v", after)
	}
}

func TestSubscriptionInstalledKernelPreservesURIOptionsAndSkipsUnknownType(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY for installed-kernel checks")
	}
	t.Setenv("XUI_BIN_FOLDER", filepath.Dir(binary))
	rows, _, warnings, err := parseOutboundSubscriptionReport([]byte("naive+https://user:private-password@example.com:443?insecure_concurrency=12#naive\nunknown-protocol://user:password@example.com"), true)
	if err != nil || len(rows) != 1 || len(warnings) != 1 {
		t.Fatal(rows, warnings, err)
	}
	settings := rows[0]["settings"].(map[string]any)
	if settings["insecure_concurrency"] != 12 && settings["insecure_concurrency"] != float64(12) {
		t.Fatal(settings)
	}
	source := []any{map[string]any(rows[0]), map[string]any{"tag": "unsupported", "protocol": "singbox:nonexistent-protocol", "settings": map[string]any{}}}
	kept, issues, _ := filterSubscriptionOutboundsForCore("installed kernel", CoreTypeHiddify, source)
	if len(kept) != 1 || len(issues) != 1 {
		t.Fatalf("kept=%v issues=%v", kept, issues)
	}
}

func TestSubscriptionReportRejectsDuplicateTagsIncludingMalformedProfiles(t *testing.T) {
	for _, body := range []string{
		`[{"tag":"same","type":42},{"tag":"same","type":"socks","server":"example.com"}]`,
		`[{"tag":"same","type":"socks","server":"example.com"},{"tag":"same","type":42}]`,
		`[{"tag":"same","type":42},{"tag":"same","type":false}]`,
	} {
		_, _, _, err := parseOutboundSubscriptionReport([]byte(body), true)
		if err == nil || !strings.Contains(err.Error(), "duplicate subscription connection tag") {
			t.Fatalf("ambiguous subscription accepted: %s; err=%v", body, err)
		}
	}
}

func TestSubscriptionKernelValidationRedactsAuthenticationFields(t *testing.T) {
	for _, test := range []struct{ field, secret string }{
		{"uuid", "private-credential-value"},
		{"auth", "private-credential-value"},
		{"auth_str", "private-credential-value"},
		{"password", "private-credential-value"},
		{"psk", "private-credential-value"},
		{"private_key", "private-credential-value"},
		{"password", "abc"},
	} {
		t.Run(test.field+"/"+test.secret, func(t *testing.T) {
			field, secret := test.field, test.secret
			writeSubscriptionCheckBinary(t, "echo 'ERROR invalid credential: "+secret+"' >&2\nexit 1\n")
			row := map[string]any{"protocol": "singbox:future-protocol", "tag": "proxy", "settings": map[string]any{field: secret}}
			err := validateSubscriptionNativeMember(row, CoreTypeHiddify)
			if err == nil || strings.Contains(err.Error(), secret) {
				t.Fatalf("credential exposed in diagnostic: %v", err)
			}
		})
	}
}

func TestSubscriptionKernelValidationUsesDiagnosticSeverity(t *testing.T) {
	for _, test := range []struct {
		name, output string
		wantError    bool
	}{
		{"warning", "WARN[0000] unsupported legacy option ignored", false},
		{"name", "INFO[0000] outbound/error-proxy: configuration accepted", false},
		{"plain-error", "ERROR invalid profile", true},
		{"timed-error", "ERROR[0000] invalid profile", true},
		{"colored-error", "\x1b[31mERROR\x1b[0m[0000] invalid profile", true},
		{"fatal", "FATAL[0000] invalid profile", true},
		{"warning-before-error", "WARN[0000] legacy option ignored\nERROR[0000] invalid profile", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeSubscriptionCheckBinary(t, "printf '%s\\n' '"+test.output+"'\nexit 0\n")
			row := map[string]any{"protocol": "singbox:socks", "tag": "proxy", "settings": map[string]any{"server": "example.com", "server_port": 1080}}
			err := validateSubscriptionNativeMember(row, CoreTypeHiddify)
			if (err != nil) != test.wantError {
				t.Fatalf("diagnostic %q: err=%v; wantError=%v", test.output, err, test.wantError)
			}
			if test.wantError && !strings.Contains(err.Error(), "invalid profile") {
				t.Fatalf("actual error omitted from diagnostic: %v", err)
			}
		})
	}
}
