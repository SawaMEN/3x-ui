package service

import (
	"reflect"
	"testing"
)

func TestExternalCertificateHookPortsNeverIncludeRemoteOrDNS(t *testing.T) {
	items := []CertificateInventoryItem{
		{Manager: "certbot", Challenge: "http-01", Ports: []int{80}, Source: "panel"},
		{Manager: "certbot", Challenge: "http-01", Ports: []int{80}, Source: "telemt"},
		{Manager: "certbot", Challenge: "dns-01", Source: "subscription"},
		{Manager: "remote", Ports: []int{443}, Source: "remote"},
		{Manager: "acme.sh", Challenge: "tls-alpn-01", Ports: []int{443}, Source: "panel"},
	}
	got, err := externalCertificateHookPorts(items, "certbot")
	if err != nil || !reflect.DeepEqual(got, []int{80}) {
		t.Fatalf("certbot ports = %v, error = %v", got, err)
	}
	got, err = externalCertificateHookPorts(items, "acme.sh")
	if err != nil || !reflect.DeepEqual(got, []int{443}) {
		t.Fatalf("acme.sh ports = %v, error = %v", got, err)
	}
	if _, err := externalCertificateHookPorts(items, "anything"); err == nil {
		t.Fatal("arbitrary manager was accepted")
	}
}
