package model

import (
	"encoding/json"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func strPtr(v string) *string { return &v }
func boolPtr(v bool) *bool    { return &v }
func intPtr(v int) *int       { return &v }

func TestApplyProxyPresetConfigOnlyTouchesExplicitFields(t *testing.T) {
	h := &Host{
		GroupId:         "group-a",
		InboundId:       7,
		Remark:          "edge",
		Address:         "edge.example.com",
		Port:            443,
		Security:        "same",
		Sni:             "old.example.com",
		AllowInsecure:   true,
		MihomoIpVersion: "dual",
	}
	cfg := ProxyPresetConfig{
		Port:          intPtr(8443),
		Security:      strPtr("tls"),
		Sni:           strPtr("preset.example.com"),
		AllowInsecure: boolPtr(false),
	}

	ApplyProxyPresetConfig(h, cfg)

	if h.Port != 8443 || h.Security != "tls" || h.Sni != "preset.example.com" || h.AllowInsecure {
		t.Fatalf("preset was not applied: %+v", h)
	}
	if h.GroupId != "group-a" || h.InboundId != 7 || h.Remark != "edge" || h.Address != "edge.example.com" {
		t.Fatalf("identity fields changed: %+v", h)
	}
	if h.MihomoIpVersion != "dual" {
		t.Fatalf("unset preset field changed: %q", h.MihomoIpVersion)
	}
}

func TestApplyProxyPresetConfigCanExplicitlyClearValues(t *testing.T) {
	empty := ""
	emptySlice := []string{}
	h := &Host{Sni: "old", Alpn: []string{"h2"}, Security: "tls"}
	cfg := ProxyPresetConfig{Sni: &empty, Alpn: &emptySlice}

	ApplyProxyPresetConfig(h, cfg)

	if h.Sni != "" {
		t.Fatalf("SNI = %q, want cleared", h.Sni)
	}
	if len(h.Alpn) != 0 {
		t.Fatalf("ALPN = %#v, want cleared", h.Alpn)
	}
	if h.Security != "tls" {
		t.Fatalf("unset security changed to %q", h.Security)
	}
}

func TestAssignedProxyPresetIsExplicitAndDoesNotChangeBaseReads(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Host{}, &ProxyPreset{}, &HostProxyPreset{}); err != nil {
		t.Fatal(err)
	}

	base := Host{GroupId: "group-a", InboundId: 1, Remark: "edge", Address: "edge.example.com", Port: 443, Security: "same", Sni: "base.example.com"}
	if err := db.Create(&base).Error; err != nil {
		t.Fatal(err)
	}
	cfg := ProxyPresetConfig{Port: intPtr(8443), Security: strPtr("tls"), Sni: strPtr("preset.example.com")}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	preset := ProxyPreset{UserId: 1, Name: "TLS", Config: string(encoded), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.Create(&preset).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&HostProxyPreset{GroupId: "group-a", UserId: 1, PresetId: preset.Id}).Error; err != nil {
		t.Fatal(err)
	}
	InvalidateProxyPresetCache()

	var stored Host
	if err := db.First(&stored, base.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Port != 443 || stored.Security != "same" || stored.Sni != "base.example.com" {
		t.Fatalf("normal DB read was unexpectedly modified: %+v", stored)
	}

	effective := stored
	if err := ApplyAssignedProxyPreset(db, &effective); err != nil {
		t.Fatal(err)
	}
	if effective.Port != 8443 || effective.Security != "tls" || effective.Sni != "preset.example.com" {
		t.Fatalf("effective host does not contain preset: %+v", effective)
	}
	if stored.Port != 443 || stored.Security != "same" || stored.Sni != "base.example.com" {
		t.Fatalf("applying preset mutated source host: %+v", stored)
	}

	if err := db.Where("group_id = ?", "group-a").Delete(&HostProxyPreset{}).Error; err != nil {
		t.Fatal(err)
	}
	InvalidateProxyPresetCache()
	restored := stored
	if err := ApplyAssignedProxyPreset(db, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Port != 443 || restored.Security != "same" || restored.Sni != "base.example.com" {
		t.Fatalf("unassign did not preserve base host values: %+v", restored)
	}
}
