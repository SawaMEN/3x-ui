package database

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestClientSeederDoesNotLogRejectedCredentials(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	inbound := &model.Inbound{Port: 443, Protocol: model.VLESS, Tag: "seeder-log-test", Settings: `{"clients":[{"email":"broken","password":"synthetic-sensitive-password","id":"synthetic-sensitive-uuid","privateKey":"synthetic-sensitive-key","enable":{}}]}`}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(old) })
	if err := seedClientsFromInboundJSON(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "skip client") {
		t.Fatal("missing useful migration diagnostic")
	}
	for _, secret := range []string{"synthetic-sensitive-password", "synthetic-sensitive-uuid", "synthetic-sensitive-key"} {
		if strings.Contains(output.String(), secret) {
			t.Error("rejected client credentials leaked into migration log")
		}
	}
}
