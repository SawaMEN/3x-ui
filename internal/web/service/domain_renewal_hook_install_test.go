package service

import (
	"strings"
	"testing"
)

func TestExternalCertificateHookScriptQuotesExecutable(t *testing.T) {
	script := certificateHookScript("/opt/panel path/x'ui", "begin", "certbot")
	if !strings.Contains(script, "certificate-hook begin certbot") ||
		!strings.Contains(script, "'\"'\"'") ||
		!strings.Contains(script, certificateHookMarker) {
		t.Fatalf("unsafe certificate hook script: %q", script)
	}
}

func TestACMESHRenewalHookPatchPreservesAdministratorHooks(t *testing.T) {
	raw := "Le_Domain='example.org'\nLe_PreHook='systemctl reload nginx'\n"
	if _, _, err := patchACMESHRenewalHooks(raw, "/safe/pre", "/safe/post"); err == nil {
		t.Fatal("overwrote an existing administrator hook")
	}
	safe := "Le_Domain='example.org'\nLe_PreHook=''\n"
	updated, changed, err := patchACMESHRenewalHooks(safe, "/safe/pre", "/safe/post")
	if err != nil || !changed || !strings.Contains(updated, "Le_PostHook='/safe/post'") ||
		!strings.Contains(updated, "Le_Domain='example.org'") {
		t.Fatalf("invalid ACME hook migration: changed=%v err=%v text=%s", changed, err, updated)
	}
	again, changed, err := patchACMESHRenewalHooks(updated, "/safe/pre", "/safe/post")
	if err != nil || changed || again != updated {
		t.Fatalf("acme.sh hook migration must be idempotent: changed=%v err=%v", changed, err)
	}
}
