//go:build !windows

package web

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func TestExternalRollbackDoesNotRestoreRevokedCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	script := "#!/bin/sh\nif grep -q '^listen_address = \"127.0.0.1:34587\"$' \"$1\"; then exit 1; fi\nexec sleep 60\n"
	if err := os.WriteFile(filepath.Join(dir, "trusttunnel_endpoint"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	ib := model.Inbound{Id: 999, Tag: "revision-rollback", Enable: true, Protocol: model.TrustTunnel, Listen: "127.0.0.1", Port: 34586, Settings: `{"hostname":"fixture.example.test","clients":[{"email":"first","enable":true,"password":"old-password"}]}`}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	mgr := externalvpn.GetManager()
	mgr.StopAll()
	defer mgr.StopAll()
	inst, err := externalvpn.FromInbound(&ib)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	ib.Port++
	ib.Settings = strings.ReplaceAll(ib.Settings, "old-password", "new-password")
	if err := db.Save(&ib).Error; err != nil {
		t.Fatal(err)
	}
	inst, err = externalvpn.FromInbound(&ib)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Ensure(inst); !errors.Is(err, sidecartraffic.ErrStaleDesired) {
		t.Fatalf("failed replacement restored obsolete credentials: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "externalvpn", "999", "credentials.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "old-password") {
		t.Fatal("rollback wrote revoked credentials back to disk")
	}
}
