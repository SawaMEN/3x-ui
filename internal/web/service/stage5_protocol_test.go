package service

import (
	"testing"

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
