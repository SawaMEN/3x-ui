package recovery

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedRecoveryAuthenticatesFilesAndExtractsSafely(t *testing.T) {
	b := Bundle{Format: 1, DatabaseType: "sqlite"}
	b.Add("database/backup.db", "/etc/x-ui/x-ui.db", []byte("sqlite-checkpoint"))
	b.Add("keys/node_token_key.json", "/etc/x-ui/node_token_key.json", []byte("private-key-material"))
	password := "a-long-recovery-password"
	encoded, err := Encode(b, password)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("private-key-material")) {
		t.Fatal("plaintext key in encrypted recovery")
	}
	if _, err := Decode(encoded, "wrong-long-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
	tampered := bytes.Clone(encoded)
	tampered[len(tampered)-1] ^= 1
	if _, err := Decode(tampered, password); err == nil {
		t.Fatal("tampered recovery accepted")
	}
	restored, err := Decode(encoded, password)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if err := Extract(restored, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "keys/node_token_key.json"))
	if err != nil || string(data) != "private-key-material" {
		t.Fatalf("key restore: %q %v", data, err)
	}
	if err := Extract(restored, target); err == nil {
		t.Fatal("existing directory overwritten")
	}
	restored.Files[0].Path = "../escape"
	if err := Extract(restored, filepath.Join(t.TempDir(), "unsafe")); err == nil {
		t.Fatal("path traversal accepted")
	}
}
