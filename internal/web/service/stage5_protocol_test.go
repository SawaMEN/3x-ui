package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestStage5LocalProtocolCompatibility(t *testing.T) {
	setupBulkDB(t)
	s := &SettingService{}
	for _, tc := range []struct {
		core     string
		protocol model.Protocol
		allowed  bool
	}{
		{CoreTypeSingBoxUpstream, model.Snell, false},
		{CoreTypeSingBoxUpstream, model.Tunnel, false},
		{CoreTypeSingBoxUpstream, model.VLESS, true},
		{CoreTypeSingBox, model.Tunnel, false},
		{CoreTypeSingBox, model.Snell, true},
		{CoreTypeXray, model.AnyTLS, false},
		{CoreTypeXray, model.VLESS, true},
	} {
		t.Run(tc.core+"/"+string(tc.protocol), func(t *testing.T) {
			if err := s.setString("coreType", tc.core); err != nil {
				t.Fatal(err)
			}
			err := validateInboundRuntimeTarget(&model.Inbound{Protocol: tc.protocol})
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v: %v", tc.allowed, err)
			}
		})
	}
}

func TestStage5IncompatibleProtocolNeverPersists(t *testing.T) {
	setupBulkDB(t)
	if err := (&SettingService{}).setString("coreType", CoreTypeSingBoxUpstream); err != nil {
		t.Fatal(err)
	}
	s := &InboundService{}
	ib := &model.Inbound{Protocol: model.Snell, Tag: "stage5-snell", Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}
	if _, _, err := s.AddInbound(ib); err == nil {
		t.Fatal("unsupported Snell was accepted")
	}
	var count int64
	if err := database.GetDB().Model(&model.Inbound{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("add changed storage: count=%d err=%v", count, err)
	}
	ib = &model.Inbound{Protocol: model.VLESS, Tag: "stage5-stored", Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	candidate := *ib
	candidate.Protocol = model.Snell
	if _, _, err := s.UpdateInbound(&candidate); err == nil {
		t.Fatal("unsupported update was accepted")
	}
	stored, err := s.GetInbound(ib.Id)
	if err != nil || stored.Protocol != model.VLESS {
		t.Fatalf("update changed storage: %+v %v", stored, err)
	}
	if err := database.GetDB().Model(ib).Update("protocol", model.Snell).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetInboundEnable(ib.Id, true); err == nil {
		t.Fatal("imported unsupported inbound was enabled")
	}
	stored, err = s.GetInbound(ib.Id)
	if err != nil || stored.Enable {
		t.Fatalf("enable changed storage: %+v %v", stored, err)
	}
}
