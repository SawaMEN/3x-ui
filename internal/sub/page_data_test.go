package sub

import (
	"reflect"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
)

// A single getSubs entry can hold several links (one per host of an inbound)
// joined by newlines. BuildPageData must split them into one entry per link, with
// the email replicated, so the subpage renders one row per host instead of
// collapsing them onto a single mangled line.
func TestBuildPageData_SplitsMultiHostLinks(t *testing.T) {
	s := &SubService{}
	subs := []string{
		"vless://a@h1:443?type=tcp#DE-john@x\nvless://a@h2:443?type=tcp#DE-john@x\nvless://a@h3:443?type=tcp#DE-john@x",
		"vless://b@h:443?type=tcp#FR-alice@x",
	}
	emails := []string{"john@x", "alice@x"}

	page := s.BuildPageData("s1", "", coremodel.ClientTraffic{}, 0, subs, emails, "", "", "", "/", "", "")

	if len(page.Result) != 4 {
		t.Fatalf("Result len = %d, want 4 (3 host links + 1 single link)", len(page.Result))
	}
	for i, link := range page.Result {
		if strings.Contains(link, "\n") {
			t.Fatalf("Result[%d] still multi-line: %q", i, link)
		}
	}
	wantEmails := []string{"john@x", "john@x", "john@x", "alice@x"}
	if !reflect.DeepEqual(page.Emails, wantEmails) {
		t.Fatalf("Emails = %v, want %v", page.Emails, wantEmails)
	}
}

// The subscription page must show the same unique connections as the
// downloadable raw subscription, with email kept in sync after filtering.
func TestBuildPageData_DeduplicatesAmneziaWGAliases(t *testing.T) {
	first := testAWGShareLink("Primary", "vpn.example.com:51820")
	alias := testAWGShareLink("Alias", "vpn.example.com:51820")
	other := testAWGShareLink("Secondary", "vpn2.example.com:51820")
	page := (&SubService{}).BuildPageData(
		"sub", "", coremodel.ClientTraffic{}, 0,
		[]string{first + "\n" + alias, other, first},
		[]string{"alice@example.com", "bob@example.com", "alice@example.com"},
		"", "", "", "/", "", "",
	)
	if want := []string{first, other}; !reflect.DeepEqual(page.Result, want) {
		t.Fatalf("page links = %v, want %v", page.Result, want)
	}
	if want := []string{"alice@example.com", "bob@example.com"}; !reflect.DeepEqual(page.Emails, want) {
		t.Fatalf("page emails = %v, want %v", page.Emails, want)
	}
}

func TestSubIsOnline(t *testing.T) {
	tests := []struct {
		name   string
		sub    []string
		online []string
		want   bool
	}{
		{name: "nobody online", sub: []string{"a@x"}, online: nil, want: false},
		{name: "no sub emails", sub: nil, online: []string{"a@x"}, want: false},
		{name: "sub client online", sub: []string{"a@x"}, online: []string{"z@x", "a@x"}, want: true},
		{name: "only other clients online", sub: []string{"a@x"}, online: []string{"z@x"}, want: false},
		{name: "any of several sub entries online", sub: []string{"a@x", "b@x"}, online: []string{"b@x"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subIsOnline(tt.sub, tt.online); got != tt.want {
				t.Fatalf("subIsOnline(%v, %v) = %v, want %v", tt.sub, tt.online, got, tt.want)
			}
		})
	}
}

func TestBuildPageData_IsOnlineFalseWithoutLiveConnections(t *testing.T) {
	s := &SubService{}

	page := s.BuildPageData("s1", "", coremodel.ClientTraffic{}, 0, []string{"vless://a@h1:443?type=tcp#DE-john@x"}, []string{"john@x"}, "", "", "", "/", "", "")

	if page.IsOnline {
		t.Fatal("IsOnline must be false when the subscription's client has no live connection")
	}
}
