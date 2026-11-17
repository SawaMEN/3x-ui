package routingcatalog

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParsePortableLists(t *testing.T) {
	got, err := Parse(strings.NewReader("\ufeff# list\nExample.COM.\ndomain:example.com\n0.0.0.0 ads.example.com tracking.example.com # hosts\n127.0.0.1 localhost\n203.0.113.12/24\n2001:db8::/32\nfull:exact.example.com\nregexp:^test[0-9]+\\.example\\.com$\nkeyword:Example\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Entries{Domains: []string{"domain:ads.example.com", "domain:example.com", "domain:tracking.example.com", "full:exact.example.com", "keyword:example", "regexp:^test[0-9]+\\.example\\.com$"}, IPs: []string{"2001:db8::/32", "203.0.113.0/24"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseRejectsInvalidAndEmptyPolicies(t *testing.T) {
	for _, input := range []string{"", "# empty\n", "<html>error</html>", "good.example.com\ninclude:google", "https://example.com", "regexp:[", "domain:-bad.example.com", "domain:a..com", "1.2.3.4/99", "fe80::1%eth0", "example.com extra-token"} {
		t.Run(input, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(input)); err == nil {
				t.Fatal("invalid list accepted")
			}
		})
	}
}

func TestParseBoundsEntriesAndLineSize(t *testing.T) {
	if _, err := Parse(strings.NewReader("regexp:" + strings.Repeat("a", 20<<10))); err == nil {
		t.Fatal("oversized line accepted")
	}
	var input strings.Builder
	for i := 0; i <= MaxEntries; i++ {
		fmt.Fprintf(&input, "keyword:entry%d\n", i)
	}
	if _, err := Parse(strings.NewReader(input.String())); err == nil {
		t.Fatal("oversized entry count accepted")
	}
}
