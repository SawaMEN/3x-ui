package sub

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditHostOrderKeepsPageIdentity(t *testing.T) {
	seedSubDB(t)
	setHostSortMode(t, "hosts-first")
	a := seedSubInbound(t, "page-order", "alice", 4597, 0, wsTLSStream)
	b := seedSubInbound(t, "page-order", "bob", 4598, 1, wsTLSStream)
	seedHost(t, &model.Host{InboundId: a.Id, GroupId: "last", SortOrder: 2, Address: "alice-last.example"})
	seedHost(t, &model.Host{InboundId: b.Id, GroupId: "first", SortOrder: 0, Address: "bob-first.example"})
	seedHost(t, &model.Host{InboundId: a.Id, GroupId: "middle", SortOrder: 1, Address: "alice-middle.example"})
	s := NewSubService("")
	links, emails, online, traffic, err := s.GetSubs("page-order", "sub.example")
	if err != nil {
		t.Fatal(err)
	}
	page := s.BuildPageData("page-order", "", traffic, online, links, emails, "", "", "", "/", "", "")
	want := []string{"bob@e", "alice@e", "alice@e"}
	if len(page.Result) != len(want) {
		t.Fatalf("page link count=%d", len(page.Result))
	}
	for i, email := range want {
		if page.Emails[i] != email || !strings.Contains(page.Result[i], strings.TrimSuffix(email, "@e")) {
			t.Errorf("row %d: email=%q link=%q, want owner %q", i, page.Emails[i], page.Result[i], email)
		}
	}
}

func TestAuditUnrenderableProtocolDoesNotShiftPageIdentity(t *testing.T) {
	seedSubDB(t)
	seedSubProtocolInbound(t, "page-native", "no-raw-uri", 4599, 0, dnsTestStream, model.MASQUE)
	seedSubInbound(t, "page-native", "visible", 4600, 1, dnsTestStream)
	s := NewSubService("")
	links, emails, online, traffic, err := s.GetSubs("page-native", "sub.example")
	if err != nil {
		t.Fatal(err)
	}
	page := s.BuildPageData("page-native", "", traffic, online, links, emails, "", "", "", "/", "", "")
	if len(page.Result) != 1 || page.Emails[0] != "visible@e" {
		t.Fatalf("unrenderable protocol shifted visible link ownership: %+v", page.Emails)
	}
}
