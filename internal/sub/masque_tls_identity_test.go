package sub

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/masque"
)

func TestMASQUEAutomaticTLSIsIncludedInFullSubscription(t *testing.T) {
	setupInfoNodeTestDB(t)
	cert, key := anyTLSCertificate(t, "masque-auto.example")
	db := database.GetDB()
	for name, value := range map[string]string{"webCertFile": cert, "webKeyFile": key, "webDomain": "masque-auto.example"} {
		if err := db.Create(&model.Setting{Key: name, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	ib := &model.Inbound{UserId: 1, Enable: true, Protocol: model.MASQUE, Port: 8443, Settings: `{"clients":[{"email":"alice","password":"secret","enable":true}]}`}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "alice", Password: "secret", SubID: "masque-auto", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{InboundId: ib.Id, ClientId: client.Id}).Error; err != nil {
		t.Fatal(err)
	}
	data, _, err := NewSubJsonService("", "", "", "", NewSubService("")).GetHiddifySingBoxJson("masque-auto", "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, `"server_name": "masque-auto.example"`) || !strings.Contains(data, "BEGIN CERTIFICATE") || strings.Contains(data, "PRIVATE KEY") || strings.Contains(data, key) {
		t.Fatalf("incorrect TLS identity/trust: %s", data)
	}
	node := 7
	remote := &model.Inbound{NodeID: &node}
	settings := masque.Settings{TLS: masque.TLS{CertificatePath: cert, KeyPath: key, ServerName: "remote.example"}}
	if got := resolveMASQUESubscriptionTLS(remote, settings); got.TLS.ServerName != "remote.example" {
		t.Fatal("remote paths read from central panel")
	}
}
