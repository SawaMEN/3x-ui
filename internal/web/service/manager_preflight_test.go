package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagerCertificateCheckIgnoresInactiveSecuritySettings(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "old-certificate.pem")
	for _, security := range []string{"none", "reality", "tls"} {
		t.Run(security, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"security": security, "tlsSettings": map[string]any{"certificates": []map[string]any{{"certificateFile": missing, "keyFile": missing}}}})
			if err != nil {
				t.Fatal(err)
			}
			ib := &model.Inbound{Protocol: model.VLESS, Settings: `{}`, StreamSettings: string(raw)}
			err = managerCheckCertificates(ib)
			if security == "tls" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("active TLS certificate was not checked: %v", err)
				}
			} else if err != nil {
				t.Fatalf("inactive TLS settings blocked %s: %v", security, err)
			}
		})
	}
	raw, err := json.Marshal(map[string]any{"tls": map[string]any{"enabled": false, "certificatePath": missing, "keyPath": missing}})
	if err != nil {
		t.Fatal(err)
	}
	if err := managerCheckCertificates(&model.Inbound{Protocol: model.Mieru, Settings: string(raw), StreamSettings: `{}`}); err != nil {
		t.Fatalf("disabled native TLS checked: %v", err)
	}
}
