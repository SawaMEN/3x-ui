package sub

import (
	"encoding/base64"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditVmessRemarkRejectsNonObjectsWithoutPanic(t *testing.T) {
	for _, payload := range []string{"null", " null ", "[]", "42", `"text"`} {
		t.Run(payload, func(t *testing.T) {
			defer func() {
				if recover() != nil {
					t.Error("renaming malformed VMess link panicked")
				}
			}()
			link := "vmess://" + base64.StdEncoding.EncodeToString([]byte(payload))
			if got := applyRemarkToLink(link, "renamed"); got != link {
				t.Error("non-object VMess payload changed")
			}
		})
	}
}

func TestAuditUnknownSubscriptionDoesNotRegisterDevice(t *testing.T) {
	router, _ := initHwidSubRouter(t, 0)
	for _, prefix := range []string{"/sub/", "/json/", "/clash/"} {
		response := requestSub(t, router, http.MethodGet, prefix+"unknown-subscription", "device-unknown", "")
		if response.Code != http.StatusNotFound {
			t.Fatalf("unknown subscription status = %d", response.Code)
		}
	}
	var count int64
	if err := database.GetDB().Model(&model.ClientHwid{}).Where("sub_id = ?", "unknown-subscription").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unknown subscription created %d device rows", count)
	}
}

func TestAuditHostFormatExclusionInBothSortModes(t *testing.T) {
	for _, mode := range []string{"inbound-first", "hosts-first"} {
		t.Run(mode, func(t *testing.T) {
			seedSubDB(t)
			setHostSortMode(t, mode)
			excluded := seedSubInbound(t, "order-sub", "excluded-all", 47200, 1, wsTLSStream)
			included := seedSubInbound(t, "order-sub", "included", 47201, 2, wsTLSStream)
			seedHost(t, &model.Host{InboundId: excluded.Id, Address: "excluded.example", ExcludeFromSubTypes: []string{"raw", "json", "clash"}})
			seedHost(t, &model.Host{InboundId: included.Id, Address: "included.example"})
			for _, format := range []string{"raw", "json", "clash"} {
				if got := renderedHostAddresses(t, format, NewSubService("")); !reflect.DeepEqual(got, []string{"included.example"}) {
					t.Errorf("%s exposed excluded/default endpoint: %v", format, got)
				}
			}
			for _, hiddify := range []bool{false, true} {
				svc := NewSubJsonService("", "", "", "", NewSubService(""))
				var body string
				var err error
				if hiddify {
					body, _, err = svc.GetHiddifySingBoxJson("order-sub", "sub.example")
				} else {
					body, _, err = svc.GetSingBoxJson("order-sub", "sub.example", false)
				}
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(body, "included.example") || strings.Contains(body, "203.0.113.5") || strings.Contains(body, "excluded.example") {
					t.Errorf("native format exposed excluded/default endpoint (hiddify=%v)", hiddify)
				}
			}
		})
	}
}
