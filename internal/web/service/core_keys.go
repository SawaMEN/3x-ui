package service

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/cryptobyte"
)

// generateECHKeyPair emits RFC ECHConfigList and the key framing consumed by
// the native TLS adapter. No separate core executable is required.
func generateECHKeyPair(sni string) (any, error) {
	if sni == "" {
		sni = "cloudflare-ech.com"
	}
	if len(sni) > 255 || strings.ContainsAny(sni, " \t\r\n/\x00") {
		return nil, fmt.Errorf("invalid ECH public name")
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	var config cryptobyte.Builder
	config.AddUint16(0xfe0d)
	config.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddUint8(0)
		b.AddUint16(0x20)
		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(key.PublicKey().Bytes()) })
		b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint16(1); b.AddUint16(1) })
		b.AddUint8(0)
		b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes([]byte(sni)) })
		b.AddUint16(0)
	})
	configBytes, err := config.Bytes()
	if err != nil {
		return nil, err
	}
	var list, keys cryptobyte.Builder
	list.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(configBytes) })
	keys.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(key.Bytes()) })
	keys.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(configBytes) })
	configs, err := list.Bytes()
	if err != nil {
		return nil, err
	}
	serverKeys, err := keys.Bytes()
	if err != nil {
		return nil, err
	}
	return map[string]any{"echConfigList": base64.StdEncoding.EncodeToString(configs), "echServerKeys": base64.StdEncoding.EncodeToString(serverKeys)}, nil
}
