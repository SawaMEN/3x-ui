package externalvpn

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func sudokuTestLink(t *testing.T, changes map[string]any) string {
	t.Helper()
	payload := map[string]any{
		"h": "node.example", "p": 443,
		"k": strings.Repeat("ab", 32),
		"a": "prefer_entropy", "e": "chacha20-poly1305",
		"m": 15832, "hm": "ws", "hx": "on",
	}
	for key, value := range changes {
		payload[key] = value
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return "sudoku://" + base64.RawURLEncoding.EncodeToString(data)
}

func TestSudokuNodeRoutingUsesGlobalIsolatedSOCKS(t *testing.T) {
	link := sudokuTestLink(t, nil)
	raw := map[string]any{"tag": "node-route-7", "protocol": "sudoku", "settings": map[string]any{"link": link}}
	if !IsAdditionalOutbound(raw) {
		t.Fatal("Sudoku was not recognized as a managed outbound")
	}
	if err := ValidateAdditionalOutbound(raw); err != nil {
		t.Fatal(err)
	}
	data, err := sudokuOutboundConfig(link)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["mode"] != "client" || cfg["proxy_mode"] != "global" || cfg["transport"] != "tcp" {
		t.Fatalf("client mode might bypass node route: %s", data)
	}
	if cfg["server_address"] != "node.example:443" || cfg["local_port"] != float64(1080) {
		t.Fatalf("link local port or server address leaked into bridge: %s", data)
	}
	if urls, ok := cfg["rule_urls"].([]any); !ok || len(urls) != 0 {
		t.Fatalf("PAC/direct routing rules must not be imported: %s", data)
	}
	if cfg["key"] != strings.Repeat("ab", 32) {
		t.Fatal("split key was not passed to the client")
	}
	httpmask := cfg["httpmask"].(map[string]any)
	if httpmask["mode"] != "ws" || httpmask["multiplex"] != "on" {
		t.Fatalf("Sudoku mask settings lost: %s", data)
	}
}

func TestSudokuNodeRoutingRejectsInvalidLinks(t *testing.T) {
	cases := []map[string]any{
		{"h": "node.example\nother"},
		{"h": "user@node.example"},
		{"p": 0},
		{"p": 65536},
		{"k": "not-hex"},
		{"e": "bogus"},
		{"a": "bogus"},
		{"hm": "garbage"},
		{"hx": "garbage"},
		{"z": "unexpected"},
	}
	for _, change := range cases {
		if _, err := sudokuOutboundConfig(sudokuTestLink(t, change)); err == nil {
			t.Errorf("accepted invalid Sudoku link: %+v", change)
		}
	}
	for _, link := range []string{"", "sudoku://", "sudoku://%%%20", "https://node.example", "sudoku://abc?x"} {
		if _, err := sudokuOutboundConfig(link); err == nil {
			t.Errorf("accepted invalid link %q", link)
		}
	}
	raw := map[string]any{"tag": "node-route-7", "protocol": "sudoku", "settings": map[string]any{"link": sudokuTestLink(t, nil), "enableUDP": true}}
	if err := ValidateAdditionalOutbound(raw); err == nil {
		t.Fatal("UDP must not silently bypass the Sudoku bridge")
	}
}
