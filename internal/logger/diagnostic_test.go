package logger

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

type privateDiagnosticError struct{}

func (privateDiagnosticError) Error() string             { return "PRIVATE_REMOTE_BODY" }
func (privateDiagnosticError) DiagnosticSummary() string { return "remote HTTP status 403" }

func TestErrorSummaryRedactsCredentialsAndKeepsCause(t *testing.T) {
	input := `certificate failed password="space secret" token=abc-token Bearer jwt.secret.value {"apiToken":"json-token","privateKeyPEM":"json-key"} https://user:urlpass@example.org/private-sub?key=query-secret vmess://base64-secret`
	input += "\n-----BEGIN PRIVATE KEY-----\nPEM_SECRET\n-----END PRIVATE KEY-----\nnext line"
	got := ErrorSummary(errors.New(input))
	for _, secret := range []string{"space secret", "abc-token", "jwt.secret.value", "json-token", "json-key", "urlpass", "private-sub", "query-secret", "PEM_SECRET", "base64-secret", "\n"} {
		if strings.Contains(got, secret) {
			t.Errorf("secret leaked: %q in %s", secret, got)
		}
	}
	if !strings.Contains(got, "certificate failed") || !strings.Contains(got, "example.org") {
		t.Fatalf("cause lost: %s", got)
	}
	wrapped := fmt.Errorf("outer: %w", privateDiagnosticError{})
	if got := ErrorSummary(wrapped); got != "remote HTTP status 403" {
		t.Fatalf("unsafe wrapped diagnostic: %s", got)
	}
	request := &url.Error{Op: "Get", URL: "https://user:secret@example.org/private", Err: errors.New("connection refused")}
	if got := ErrorSummary(request); got != "connection refused" {
		t.Fatalf("URL leaked: %s", got)
	}
}

func TestDiagnosticIsBoundedAndRedactsIncompletePEM(t *testing.T) {
	if got := SanitizeDiagnostic("error -----BEGIN PRIVATE KEY-----" + strings.Repeat("SECRET", 20000)); strings.Contains(got, "SECRET") {
		t.Fatal("incomplete PEM leaked")
	}
	if got := SanitizeDiagnostic(strings.Repeat("a", 100000)); len(got) > 4200 || !strings.Contains(got, "truncated") {
		t.Fatalf("unbounded diagnostic: %d", len(got))
	}
	if got := SanitizeDiagnostic("Authorization: Bearer secret\r\nfailed"); strings.Contains(got, "secret") || !strings.Contains(got, "failed") {
		t.Fatalf("unsafe header: %s", got)
	}
}

func TestDiagnosticPreservesJoinedFailureCauses(t *testing.T) {
	err := fmt.Errorf("operation rollback: %w", errors.Join(context.Canceled, privateDiagnosticError{}, errors.New("disk full password=PRIVATE_PASSWORD")))
	got := ErrorSummary(err)
	for _, want := range []string{"operation canceled", "remote HTTP status 403", "disk full"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing cause %q in %s", want, got)
		}
	}
	if strings.Contains(got, "PRIVATE_") {
		t.Fatalf("joined error leaked secrets: %s", got)
	}
}

func TestDiagnosticRedactsExtendedSubscriptionURIs(t *testing.T) {
	for _, scheme := range []string{
		"tg", "vpn", "awg", "amneziawg", "naive+https", "naive+quic",
		"anytls", "shadowtls", "mieru", "snell", "sudoku", "masque-client",
		"fptn", "trusttunnel", "openflux", "vetroff", "vk-turn-proxy", "pingtunnel",
	} {
		got := SanitizeDiagnostic("invalid profile " + scheme + "://PRIVATE_CREDENTIAL@example.org")
		if strings.Contains(got, "PRIVATE_CREDENTIAL") || !strings.Contains(got, "invalid profile") {
			t.Errorf("%s: unsafe diagnostic %q", scheme, got)
		}
	}
}
