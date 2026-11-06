package sidecaregress

import "testing"

func TestReadyRequiresAcknowledgedInboundIdentity(t *testing.T) {
	defer SetRuntimeReader(nil)
	SetRuntimeReader(func() []byte {
		return []byte(`{"inbounds":[{"type":"socks","tag":"original","listen_port":60000}],"endpoints":[{"type":"wireguard","tag":"vk","listen_port":60003}]}`)
	})
	if !Ready(1, "original", false) || Ready(1001, "wrapped", false) || Ready(1, "renamed", false) || !Ready(2, "vk", true) || Ready(2, "vk", false) {
		t.Fatal("sidecar was allowed to reuse another route")
	}
	SetRuntimeReader(func() []byte { return nil })
	if !Ready(1, "waiting-for-core", false) {
		t.Fatal("stopped core prevented harmless sidecar preparation")
	}
	SetRuntimeReader(func() []byte { return []byte(`invalid`) })
	if Ready(1, "original", false) {
		t.Fatal("invalid runtime state failed open")
	}
}
