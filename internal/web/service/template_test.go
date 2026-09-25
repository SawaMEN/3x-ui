package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeTemplateContentRemovesSecretsAndClients(t *testing.T) {
	raw := []byte("{\"protocol\":\"vless\",\"settings\":{\"clients\":[{\"id\":\"uuid\",\"password\":\"secret\"}],\"decryption\":\"none\"},\"streamSettings\":{\"network\":\"xhttp\",\"security\":\"reality\",\"realitySettings\":{\"privateKey\":\"PRIVATE\",\"publicKey\":\"PUBLIC\",\"shortIds\":[\"abcd\"],\"serverName\":\"example.com\"},\"tlsSettings\":{\"serverName\":\"tls.example.com\",\"certificates\":[{\"certificateFile\":\"/secret.pem\"}]}},\"apiKey\":\"top-secret\"}")

	clean, warnings, err := sanitizeTemplateContent("inbound", raw)
	if err != nil {
		t.Fatalf("sanitizeTemplateContent: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal(clean, &obj); err != nil {
		t.Fatalf("invalid sanitized JSON: %v", err)
	}
	b, _ := json.Marshal(obj)
	text := string(b)
	for _, secret := range []string{"PRIVATE", "PUBLIC", "secret", "top-secret", "example.com", "tls.example.com", "/secret.pem"} {
		if strings.Contains(text, secret) {
			t.Fatalf("sanitized template still contains %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, `"clients":[]`) {
		t.Fatalf("clients collection should be preserved as an empty array: %s", text)
	}
	if len(warnings) < 4 {
		t.Fatalf("expected sanitization warnings, got %v", warnings)
	}
}

func TestSanitizeTemplateContentRejectsUnknownKind(t *testing.T) {
	_, _, err := sanitizeTemplateContent("wireguard", []byte("{\"x\":1}"))
	if err == nil {
		t.Fatal("expected unknown kind to be rejected")
	}
}

func TestSanitizeTemplateContentRemovesFlatOutboundCredentials(t *testing.T) {
	raw := []byte(`{"outbounds":[{"protocol":"vless","tag":"proxy","settings":{"address":"edge.example","port":443,"id":"credential-uuid","encryption":"none"},"streamSettings":{"realitySettings":{"mldsa65Seed":"signing-seed","shortId":"abcd"}}},{"protocol":"hysteria","settings":{"auth":"hy-secret"}},{"type":"tuic","uuid":"tuic-uuid","tls":{"reality":{"private_key":"private-secret"}}}]}`)
	clean, warnings, err := sanitizeTemplateContent("xray_config", raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"credential-uuid", "signing-seed", "abcd", "hy-secret", "tuic-uuid", "private-secret"} {
		if strings.Contains(string(clean), secret) {
			t.Errorf("credential %q remained in template: %s", secret, clean)
		}
	}
	if !strings.Contains(string(clean), `"tag": "proxy"`) || len(warnings) < 6 {
		t.Fatalf("sanitization lost configuration or warnings: %s, %v", clean, warnings)
	}
}

func TestSanitizeTemplateKeepsSocksAuthenticationMode(t *testing.T) {
	clean, _, err := sanitizeTemplateContent("inbound", []byte(`{"protocol":"socks","settings":{"auth":"password","accounts":[{"user":"alice","pass":"secret"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(clean), `"auth": "password"`) || strings.Contains(string(clean), "secret") {
		t.Fatalf("sanitized SOCKS settings = %s", clean)
	}
}
