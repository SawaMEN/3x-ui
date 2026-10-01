package service

import (
	"reflect"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func TestEffectiveHostGroupAppliesOnlyPresetFields(t *testing.T) {
	port := 8443
	security := "tls"
	sni := "preset.example.com"
	base := &entity.HostGroup{
		GroupId:    "group-a",
		InboundIds: []int{7},
		Hosts:      []string{"edge.example.com:443"},
		Remark:     "edge",
		Port:       443,
		Security:   "same",
		Sni:        "base.example.com",
		Path:       "/base",
	}

	effective := effectiveHostGroup(base, model.ProxyPresetConfig{
		Port:     &port,
		Security: &security,
		Sni:      &sni,
	})
	if effective.Port != 8443 || effective.Security != "tls" || effective.Sni != sni {
		t.Fatalf("effective group did not apply preset: %+v", effective)
	}
	if effective.Path != "/base" || effective.Remark != "edge" {
		t.Fatalf("preset changed inherited fields: %+v", effective)
	}
	if !reflect.DeepEqual(effective.InboundIds, base.InboundIds) || !reflect.DeepEqual(effective.Hosts, base.Hosts) {
		t.Fatalf("preset changed host identity: %+v", effective)
	}

	changed := previewChangedFields(base, effective)
	want := []string{"port", "security", "sni"}
	if !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed fields = %#v, want %#v", changed, want)
	}
}

func TestProxyPresetPreviewWarningsWithoutInboundLookup(t *testing.T) {
	warnings, err := proxyPresetPreviewWarnings(nil, 1, &entity.HostGroup{
		Security:               "none",
		Sni:                    "ignored.example.com",
		OverrideSniFromAddress: true,
		KeepSniBlank:           true,
		AllowInsecure:          true,
		ExcludeFromSubTypes:    []string{"raw", "json", "clash"},
	})
	if err != nil {
		t.Fatalf("warnings returned error: %v", err)
	}
	if len(warnings) != 3 {
		t.Fatalf("warnings = %#v, want 3 warnings", warnings)
	}
}
