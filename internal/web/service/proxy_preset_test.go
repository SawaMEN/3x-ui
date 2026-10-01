package service

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func proxyString(v string) *string { return &v }
func proxyInt(v int) *int          { return &v }

func TestValidateProxyPresetInput(t *testing.T) {
	in := ProxyPresetInput{
		Name:        "  CDN TLS  ",
		Description: " reusable host settings ",
		Config: model.ProxyPresetConfig{
			Port:          proxyInt(443),
			Security:      proxyString("tls"),
			MuxParams:     proxyString(` { "enabled": true } `),
			SockoptParams: proxyString(`{"tcpKeepAliveInterval":30}`),
			FinalMask:     proxyString(`{"tcp":["1-3"]}`),
			VlessRoute:    proxyString("443"),
		},
	}
	if err := validateProxyPresetInput(&in); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if in.Name != "CDN TLS" || in.Description != "reusable host settings" {
		t.Fatalf("input was not normalized: %#v", in)
	}
	if in.Config.MuxParams == nil || strings.Contains(*in.Config.MuxParams, " ") {
		t.Fatalf("muxParams was not compacted: %v", in.Config.MuxParams)
	}
}

func TestValidateProxyPresetInputRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		in   ProxyPresetInput
	}{
		{"empty name", ProxyPresetInput{}},
		{"bad port", ProxyPresetInput{Name: "x", Config: model.ProxyPresetConfig{Port: proxyInt(70000)}}},
		{"bad security", ProxyPresetInput{Name: "x", Config: model.ProxyPresetConfig{Security: proxyString("bogus")}}},
		{"bad mihomo", ProxyPresetInput{Name: "x", Config: model.ProxyPresetConfig{MihomoIpVersion: proxyString("bogus")}}},
		{"bad route", ProxyPresetInput{Name: "x", Config: model.ProxyPresetConfig{VlessRoute: proxyString("70000")}}},
		{"bad mux json", ProxyPresetInput{Name: "x", Config: model.ProxyPresetConfig{MuxParams: proxyString("[]")}}},
		{"bad final mask", ProxyPresetInput{Name: "x", Config: model.ProxyPresetConfig{FinalMask: proxyString("not-json")}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateProxyPresetInput(&tc.in); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
