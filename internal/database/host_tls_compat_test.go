package database

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestExternalProxyImportRetainsTLSOverrides(t *testing.T) {
	host := externalProxyEntryToHost(1, 0, map[string]any{
		"dest": "edge.example", "port": float64(443), "forceTls": "tls", "remark": "edge",
		"sni": "sni.example", "verifyPeerCertByName": "cert.example", "cipherSuites": "TLS_AES_128_GCM_SHA256",
		"vlessRoute": "321", "allowInsecure": true, "serverDescription": "edge caption",
	})
	if host.VerifyPeerCertByName != "cert.example" || host.CipherSuites != "TLS_AES_128_GCM_SHA256" || host.VlessRoute != "321" || !host.AllowInsecure || host.ServerDescription != "edge caption" {
		t.Fatalf("TLS/share overrides lost during import: %+v", host)
	}
}

func TestExternalProxyImportRetainsLegacyStringLists(t *testing.T) {
	host := externalProxyEntryToHost(1, 0, map[string]any{"alpn": "h2,http/1.1", "pinnedPeerCertSha256": "pin-a,pin-b"})
	if !reflect.DeepEqual(host.Alpn, []string{"h2", "http/1.1"}) || !reflect.DeepEqual(host.PinnedPeerCertSha256, []string{"pin-a", "pin-b"}) {
		t.Fatalf("legacy TLS lists lost: alpn=%v pins=%v", host.Alpn, host.PinnedPeerCertSha256)
	}
}

func TestExternalProxyImportKeepsUnicodeRemark(t *testing.T) {
	for _, remark := range []string{strings.Repeat("я", 200), strings.Repeat("a", 255) + "猫tail"} {
		host := externalProxyEntryToHost(1, 0, map[string]any{"remark": remark})
		expected := []rune(remark)
		if len(expected) > 256 {
			expected = expected[:256]
		}
		if !utf8.ValidString(host.Remark) || host.Remark != string(expected) {
			t.Fatal("Unicode remark was truncated as bytes instead of characters")
		}
	}
}

func TestTLSRepairPreservesAdminEditsAndGroupIdentity(t *testing.T) {
	initMigrateDB(t)
	input := map[string]any{"dest": "edge.example", "port": float64(443), "sni": "sni.example", "remark": "old", "verifyPeerCertByName": "cert.example", "cipherSuites": "TLS_AES_128_GCM_SHA256", "alpn": "h2,http/1.1", "pinnedPeerCertSha256": "pin-a,pin-b"}
	stream, err := json.Marshal(map[string]any{"externalProxy": []any{input}})
	if err != nil {
		t.Fatal(err)
	}
	ib := seedInboundWithStream(t, "tls-repair", 30303, string(stream))
	old := legacyExternalProxyEntryToHost(ib.Id, 0, input)
	if err = db.Create(old).Error; err != nil {
		t.Fatal(err)
	}
	edited := legacyExternalProxyEntryToHost(ib.Id, 0, input)
	edited.HostHeader = "admin.example"
	if err = db.Create(edited).Error; err != nil {
		t.Fatal(err)
	}
	shared := legacyExternalProxyEntryToHost(ib.Id, 0, input)
	if err = db.Create(shared).Error; err != nil {
		t.Fatal(err)
	}
	sibling := *shared
	sibling.Id = 0
	if err = db.Create(&sibling).Error; err != nil {
		t.Fatal(err)
	}
	if err = repairImportedHostTLS(); err != nil {
		t.Fatal(err)
	}
	var repaired model.Host
	if err = db.First(&repaired, old.Id).Error; err != nil {
		t.Fatal(err)
	}
	if repaired.GroupId != old.GroupId || repaired.CreatedAt != old.CreatedAt || repaired.VerifyPeerCertByName != "cert.example" || len(repaired.Alpn) != 2 || len(repaired.PinnedPeerCertSha256) != 2 {
		t.Fatalf("legacy row not repaired in place: %+v", repaired)
	}
	for _, id := range []int{edited.Id, shared.Id, sibling.Id} {
		var kept model.Host
		if err = db.First(&kept, id).Error; err != nil {
			t.Fatal(err)
		}
		if kept.VerifyPeerCertByName != "" {
			t.Fatal("repair overwrote an edited or shared host")
		}
	}
	if err = repairImportedHostTLS(); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Model(&model.Host{}).Where("inbound_id = ?", ib.Id).Count(&count).Error; err != nil || count != 4 {
		t.Fatalf("repair created duplicate hosts: %d %v", count, err)
	}
}

func TestTLSRepairRollbackDoesNotMarkFailedRepairDone(t *testing.T) {
	initMigrateDB(t)
	input := map[string]any{"dest": "edge.example", "remark": "old", "cipherSuites": "TLS_AES_128_GCM_SHA256"}
	stream, _ := json.Marshal(map[string]any{"externalProxy": []any{input}})
	ib := seedInboundWithStream(t, "tls-rollback", 30304, string(stream))
	old := legacyExternalProxyEntryToHost(ib.Id, 0, input)
	if err := db.Create(old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_tls_repair BEFORE UPDATE ON hosts BEGIN SELECT RAISE(ABORT,'synthetic host write failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairImportedHostTLS(); err == nil {
		t.Fatal("synthetic write failure ignored")
	}
	var count int64
	if err := db.Model(&model.HistoryOfSeeders{}).Where("seeder_name = ?", "ImportedHostTLSOverrides").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed repair marked done: %d %v", count, err)
	}
	if err := db.Exec(`DROP TRIGGER reject_tls_repair`).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairImportedHostTLS(); err != nil {
		t.Fatal(err)
	}
	var repaired model.Host
	if err := db.First(&repaired, old.Id).Error; err != nil || repaired.CipherSuites != "TLS_AES_128_GCM_SHA256" {
		t.Fatalf("repair could not retry after rollback: %v", err)
	}
}
