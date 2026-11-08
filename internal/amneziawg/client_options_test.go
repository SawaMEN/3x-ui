package amneziawg

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestClientOptionsPreserveSharedHandshakeFields(t *testing.T) {
	s := ServerSettings{Jc: 4, Jmin: 20, Jmax: 80, S1: 20, S2: 30, S3: 12, S4: 12, H1: "10", H2: "20", H3: "30", H4: "40", I1: "<r 64>"}
	zero, empty, mtu := 0, "", 1380
	got, err := ClientServerSettings(s, &model.AWGClientOptions{Jc: &zero, I1: &empty, MTU: &mtu})
	if err != nil {
		t.Fatal(err)
	}
	if got.Jc != 0 || got.I1 != "" || got.MTU != mtu {
		t.Fatal("client override was not applied")
	}
	if got.H1 != s.H1 || got.S4 != s.S4 || s.Jc != 4 || s.I1 != "<r 64>" {
		t.Fatal("shared server fields changed")
	}
	if _, err := ClientServerSettings(s, &model.AWGClientOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestClientOptionsRejectInvalidValues(t *testing.T) {
	s := ServerSettings{Jmin: 10, Jmax: 20}
	negative, lowMTU, largeMin, injection := -1, 1000, 30, "<r 20>\nPostUp = bad"
	for _, options := range []*model.AWGClientOptions{{Jc: &negative}, {MTU: &lowMTU}, {Jmin: &largeMin}, {I1: &injection}} {
		if _, err := ClientServerSettings(s, options); err == nil {
			t.Fatalf("accepted invalid overrides: %+v", options)
		}
	}
}
