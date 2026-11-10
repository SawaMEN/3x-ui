package controller

import (
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
