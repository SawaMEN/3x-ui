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
	if !strings.Contains(text, "\"clients\":[]") {
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
