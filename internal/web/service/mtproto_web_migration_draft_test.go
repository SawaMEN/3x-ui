package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestMtprotoMigrationDraftDoesNotReserveTCP443(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	vpn := model.Inbound{Protocol: model.VLESS, Enable: true, Port: 443, Listen: "0.0.0.0", Tag: "vpn-443", Settings: "{}"}
	if err := db.Create(&vpn).Error; err != nil {
		t.Fatal(err)
	}
	draft := model.Inbound{Protocol: model.MTProto, Port: 443, Tag: "mtproto-draft", Settings: `{"webProxy":{"enabled":true,"migrationDraft":true,"domain":"web.example.com","sharedIngress":{"enabled":true}}}`}
	if !mtprotoWebMigrationDraft(&draft) || len(inboundListenerSpans(&draft)) != 0 {
		t.Fatal("draft unexpectedly reserves a socket")
	}
	if conflict, err := checkPortConflictTx(db, &draft, 0); err != nil || conflict != nil {
		t.Fatalf("disabled draft rejected: %v %v", conflict, err)
	}
	draft.Enable = true
	if mtprotoWebMigrationDraft(&draft) || ValidateMtprotoWebInbound(&draft) == nil {
		t.Fatal("ordinary activation accepted a migration draft")
	}
	if conflict, err := checkPortConflictTx(db, &draft, 0); err != nil || conflict == nil {
		t.Fatalf("active draft bypassed port conflict: %v %v", conflict, err)
	}
	draft.Enable = false
	draft.Settings = `{"webProxy":{"enabled":true}}`
	if len(inboundListenerSpans(&draft)) == 0 {
		t.Fatal("ordinary disabled WEB inbound lost its reservation")
	}
}

func TestManagedWebMigrationAtomicActivationAndRollback(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	vpn := model.Inbound{
		Protocol: model.VLESS, Enable: true, Port: 443, Listen: "0.0.0.0", Tag: "vpn-443",
		Settings: "{}", StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`,
	}
	draft := model.Inbound{
		Protocol: model.MTProto, Port: 443, Tag: "mtproto-draft",
		Settings: `{"webProxy":{"enabled":true,"migrationDraft":true,"domain":"web.example.com","sharedIngress":{"enabled":true}},"clients":[{"email":"fixture","secret":"fixture-secret"}]}`,
	}
	for _, row := range []*model.Inbound{&vpn, &draft} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	move, ok := telemtVPNMigrationCandidate(vpn, "web.example.com", nil)
	if !ok || move.Reason != "" {
		t.Fatalf("unexpected VPN plan: %+v", move)
	}
	move.NewPort = telemtVPNMigrationFirstPort
	ingress := &TelemtWebIngress{Enabled: true, InboundIDs: []int{vpn.Id}}
	stale := draft
	stale.Settings += " "
	if err := stageTelemtVPNMigrationWithDraft([]telemtVPNMigrationPlan{move}, &stale, ingress); err == nil {
		t.Fatal("stale draft accepted")
	}
	var saved model.Inbound
	if err := db.First(&saved, vpn.Id).Error; err != nil || saved.Port != 443 {
		t.Fatal("failed draft activation changed VPN port")
	}
	if err := stageTelemtVPNMigrationWithDraft([]telemtVPNMigrationPlan{move}, &draft, ingress); err != nil {
		t.Fatal(err)
	}
	var active model.Inbound
	if err := db.First(&active, draft.Id).Error; err != nil {
		t.Fatal(err)
	}
	opts, err := mtprotoWebForInbound(&active)
	if err != nil || !active.Enable || opts.MigrationDraft || !telemtWebIngressEqual(opts.SharedIngress, ingress) {
		t.Fatalf("draft was not activated: %v", err)
	}
	journal, err := loadTelemtVPNMigration(db)
	if err != nil || journal == nil || journal.ManagedInboundID != draft.Id {
		t.Fatalf("managed journal missing: %v", err)
	}
	raw, err := json.Marshal(journal)
	if err != nil || strings.Contains(string(raw), "fixture-secret") {
		t.Fatal("migration journal exposed client secret")
	}
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&active, draft.Id).Error; err != nil {
		t.Fatal(err)
	}
	editedIngress := &TelemtWebIngress{Enabled: true, InboundIDs: []int{999}}
	edited, err := patchManagedMigrationDraft(active.Settings, editedIngress, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&active).Update("settings", edited).Error; err != nil {
		t.Fatal(err)
	}
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&active, draft.Id).Error; err != nil || active.Settings != edited {
		t.Fatal("repeated rollback overwrote the restored draft")
	}
	if err := db.First(&active, draft.Id).Error; err != nil || !mtprotoWebMigrationDraft(&active) {
		t.Fatalf("draft not restored: %v", err)
	}
	if err := db.First(&saved, vpn.Id).Error; err != nil || saved.Port != 443 || saved.Listen != vpn.Listen {
		t.Fatalf("VPN socket not restored: %v", err)
	}
}
