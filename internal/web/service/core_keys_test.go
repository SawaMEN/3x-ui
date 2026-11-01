package service

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"testing"

	"golang.org/x/crypto/cryptobyte"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestCoreKeysDoNotRequireLegacyBinary(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	value, err := (&ServerService{}).GetNewX25519Cert()
	if err != nil {
		t.Fatal(err)
	}
	pair := value.(map[string]any)
	private, err := base64.RawURLEncoding.DecodeString(pair["privateKey"].(string))
	if err != nil {
		t.Fatal(err)
	}
	public, err := base64.RawURLEncoding.DecodeString(pair["publicKey"].(string))
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdh.X25519().NewPrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key.PublicKey().Bytes(), public) {
		t.Fatal("X25519 key pair does not match")
	}
	value, err = generateECHKeyPair("public.example")
	if err != nil {
		t.Fatal(err)
	}
	pair = value.(map[string]any)
	configs, err := base64.StdEncoding.DecodeString(pair["echConfigList"].(string))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := base64.StdEncoding.DecodeString(pair["echServerKeys"].(string))
	if err != nil {
		t.Fatal(err)
	}
	list := cryptobyte.String(configs)
	var config cryptobyte.String
	if !list.ReadUint16LengthPrefixed(&config) || !list.Empty() {
		t.Fatal("invalid ECH config list")
	}
	framed := cryptobyte.String(keys)
	var privateECH, configECH cryptobyte.String
	if !framed.ReadUint16LengthPrefixed(&privateECH) || !framed.ReadUint16LengthPrefixed(&configECH) || !framed.Empty() || !bytes.Equal(configECH, config) {
		t.Fatal("ECH server keys do not match config list")
	}
	var version, kem uint16
	var content, publicECH, suites, name, extensions cryptobyte.String
	var id, maxName uint8
	if !config.ReadUint16(&version) || version != 0xfe0d || !config.ReadUint16LengthPrefixed(&content) || !config.Empty() || !content.ReadUint8(&id) || !content.ReadUint16(&kem) || kem != 0x20 || !content.ReadUint16LengthPrefixed(&publicECH) || !content.ReadUint16LengthPrefixed(&suites) || !content.ReadUint8(&maxName) || !content.ReadUint8LengthPrefixed(&name) || string(name) != "public.example" || !content.ReadUint16LengthPrefixed(&extensions) || !content.Empty() {
		t.Fatal("invalid ECH config content")
	}
	key, err = ecdh.X25519().NewPrivateKey(privateECH)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key.PublicKey().Bytes(), publicECH) {
		t.Fatal("ECH public key mismatch")
	}
	if _, err = generateECHKeyPair("bad/name"); err == nil {
		t.Fatal("invalid public name accepted")
	}
}

func TestGeneratedECHKeysAcceptedByOwnerCore(t *testing.T) {
	setupBulkDB(t)
	cert, key := writeTestCertificate(t)
	pair, err := generateECHKeyPair("public.example")
	if err != nil {
		t.Fatal(err)
	}
	stream := map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"echServerKeys": pair.(map[string]any)["echServerKeys"], "certificates": []any{map[string]any{"certificateFile": cert, "keyFile": key}}}}
	encoded, err := json.Marshal(stream)
	if err != nil {
		t.Fatal(err)
	}
	seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, string(encoded))
	cfg, err := (&SingBoxService{}).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	checkGeneratedHiddifyConfig(t, cfg)
}
