package controller

import (
	"net/http"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestDomainActivityCoreMode(t *testing.T) {
	cases := map[string]string{
		service.CoreTypeSingBox:         "active",
		service.CoreTypeSingBoxUpstream: "active",
		service.CoreTypeXray:            "recent",
		"unknown":                       "",
		"":                              "",
	}
	for core, want := range cases {
		if got := domainActivityCoreMode(core); got != want {
			t.Errorf("domainActivityCoreMode(%q) = %q; want %q", core, got, want)
		}
	}
}

func TestDomainActivityNodeSyncIsReadOnly(t *testing.T) {
	if !scopeAllows(nodeSyncScopeAllow, "/server/domainActivity", http.MethodGet) {
		t.Fatal("node-sync must be able to read local domain activity")
	}
	if scopeAllows(nodeSyncScopeAllow, "/server/domainActivity", http.MethodPost) ||
		scopeAllows(nodeSyncScopeAllow, "/server/domainActivity/config", http.MethodPost) {
		t.Fatal("node-sync must not be allowed to configure domain monitoring")
	}
	if _, ok := monitorScopeAllow["/server/domainActivity"]; ok {
		t.Fatal("monitor tokens must not expose per-client domain activity")
	}
}
