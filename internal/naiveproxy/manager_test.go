package naiveproxy

import (
	"strings"
	"testing"
)

func TestRenderConfig(t *testing.T) {
	got, err := RenderConfig([]Inbound{
		{
			Tag:             "naive-443",
			Listen:          "0.0.0.0",
			Port:            443,
			CertificatePath: "/etc/ssl/cert.pem",
			KeyPath:         "/etc/ssl/key.pem",
			Users: []User{
				{Username: "alice@example.com", Password: "first secret"},
				{Username: "bob@example.com", Password: "second-secret"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		":443 {",
		`tls "/etc/ssl/cert.pem" "/etc/ssl/key.pem"`,
		`basic_auth "alice@example.com" "first secret"`,
		`basic_auth "bob@example.com" "second-secret"`,
		"hide_ip",
		"hide_via",
		"probe_resistance",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated config does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\tbind ") {
		t.Fatalf("wildcard listener must not emit bind:\n%s", got)
	}
}

func TestRenderConfigConcreteListen(t *testing.T) {
	got, err := RenderConfig([]Inbound{{
		Tag:             "naive-loopback",
		Listen:          "127.0.0.1",
		Port:            8443,
		CertificatePath: "/cert.pem",
		KeyPath:         "/key.pem",
		Users:           []User{{Username: "u", Password: "p"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `bind "127.0.0.1"`) {
		t.Fatalf("concrete listener must emit bind:\n%s", got)
	}
}

func TestRenderConfigRejectsDuplicatePort(t *testing.T) {
	_, err := RenderConfig([]Inbound{
		{Tag: "one", Port: 443, CertificatePath: "/c", KeyPath: "/k", Users: []User{{Username: "u1", Password: "p1"}}},
		{Tag: "two", Port: 443, CertificatePath: "/c", KeyPath: "/k", Users: []User{{Username: "u2", Password: "p2"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "same port") {
		t.Fatalf("expected duplicate-port error, got %v", err)
	}
}

func TestRenderConfigRequiresTLSAndUsers(t *testing.T) {
	if _, err := RenderConfig([]Inbound{{Tag: "missing-tls", Port: 443, Users: []User{{Username: "u", Password: "p"}}}}); err == nil {
		t.Fatal("expected missing TLS error")
	}
	if _, err := RenderConfig([]Inbound{{Tag: "missing-user", Port: 443, CertificatePath: "/c", KeyPath: "/k"}}); err == nil {
		t.Fatal("expected missing users error")
	}
}

func TestRenderConfigRejectsOnlyInvalidCredentials(t *testing.T) {
	_, err := RenderConfig([]Inbound{{
		Tag:             "invalid-users",
		Port:            443,
		CertificatePath: "/c",
		KeyPath:         "/k",
		Users: []User{
			{Username: "", Password: "secret"},
			{Username: "   ", Password: "secret"},
			{Username: "alice@example.com", Password: ""},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "no enabled users") {
		t.Fatalf("expected invalid-credentials error, got %v", err)
	}
}

func TestRenderConfigFiltersInvalidCredentials(t *testing.T) {
	got, err := RenderConfig([]Inbound{{
		Tag:             "mixed-users",
		Port:            443,
		CertificatePath: "/c",
		KeyPath:         "/k",
		Users: []User{
			{Username: "   ", Password: "ignored"},
			{Username: " alice@example.com ", Password: "secret"},
			{Username: "bob@example.com", Password: ""},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `basic_auth "alice@example.com" "secret"`) {
		t.Fatalf("expected trimmed valid credentials:\n%s", got)
	}
	if strings.Contains(got, "bob@example.com") || strings.Contains(got, "ignored") {
		t.Fatalf("invalid credentials must not be emitted:\n%s", got)
	}
}
