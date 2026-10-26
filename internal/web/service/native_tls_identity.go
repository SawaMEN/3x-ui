package service

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"maps"
	"os"
	"strings"
)

// ResolveNativeTLSSettings applies the same certificate selection to the native
// listener and its subscription. lookup may read a runtime snapshot or live settings.
// It never reads a private key and never disables certificate verification.
func ResolveNativeTLSSettings(raw map[string]any, lookup func(string) (string, error)) (map[string]any, error) {
	tls := maps.Clone(raw)
	if tls == nil {
		tls = map[string]any{}
	}
	cert, _ := tls["certificatePath"].(string)
	key, _ := tls["keyPath"].(string)
	cert, key = strings.TrimSpace(cert), strings.TrimSpace(key)
	if (cert == "") != (key == "") {
		return nil, fmt.Errorf("provide both TLS certificate and private key, or neither")
	}
	if cert == "" && lookup != nil {
		for _, pair := range [][2]string{{"webCertFile", "webKeyFile"}, {"subCertFile", "subKeyFile"}} {
			c, err := lookup(pair[0])
			if err != nil {
				return nil, err
			}
			k, err := lookup(pair[1])
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(c) != "" && strings.TrimSpace(k) != "" {
				cert, key = strings.TrimSpace(c), strings.TrimSpace(k)
				break
			}
		}
	}
	if cert == "" {
		return nil, fmt.Errorf("requires a complete TLS certificate/private-key pair (inbound, panel HTTPS, or subscription HTTPS)")
	}
	tls["enabled"], tls["certificatePath"], tls["keyPath"] = true, cert, key
	name, _ := tls["serverName"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		var preferred []string
		if lookup != nil {
			for _, field := range []string{"webDomain", "subDomain"} {
				v, _ := lookup(field)
				preferred = append(preferred, v)
			}
		}
		name = NativeTLSCertificateName(cert, preferred...)
	}
	if name != "" {
		tls["serverName"] = name
	}
	return tls, nil
}

// NativeTLSCertificateName selects a certificate SAN rather than a dial address:
// the listener may be reached by IP or through an unrelated ShadowTLS cover SNI.
func NativeTLSCertificateName(path string, preferred ...string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	candidates := append([]string(nil), preferred...)
	for _, name := range cert.DNSNames {
		if strings.HasPrefix(name, "*.") {
			name = "vpn." + strings.TrimPrefix(name, "*.")
		}
		candidates = append(candidates, name)
	}
	for _, ip := range cert.IPAddresses {
		candidates = append(candidates, ip.String())
	}
	for _, name := range candidates {
		name = strings.TrimSpace(name)
		if name != "" && cert.VerifyHostname(name) == nil {
			return name
		}
	}
	return ""
}

// NativeTLSSetting reads only the small set of public TLS identity settings.
func NativeTLSSetting(key string) (string, error) {
	s := &SettingService{}
	switch key {
	case "webCertFile":
		return s.GetCertFile()
	case "webKeyFile":
		return s.GetKeyFile()
	case "subCertFile":
		return s.GetSubCertFile()
	case "subKeyFile":
		return s.GetSubKeyFile()
	case "webDomain":
		return s.GetWebDomain()
	case "subDomain":
		return s.GetSubDomain()
	default:
		return "", fmt.Errorf("unknown native TLS setting %q", key)
	}
}

// NativeTLSClientCertificate returns only parsed public certificates, never key
// material. A subscription can therefore trust the configured self-signed or
// private-CA listener without switching allowInsecure on.
func NativeTLSClientCertificate(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var result strings.Builder
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			continue
		}
		result.Write(pem.EncodeToMemory(block))
	}
	return result.String()
}
