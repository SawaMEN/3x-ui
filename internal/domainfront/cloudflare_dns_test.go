package domainfront

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/libdns/libdns"
)

func TestCloudflareDNS01CreatesAndDeletesOnlyOwnedTXT(t *testing.T) {
	id := strings.Repeat("a", 32)
	token := strings.Repeat("z", 40)
	var requests []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("missing scoped credential")
		}
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/zones/"+id+"/dns_records" {
			_, _ = fmt.Fprint(w, `{"success":true,"result":{"id":"`+id+`"}}`)
			return
		}
		if r.Method == http.MethodDelete && r.URL.Path == "/zones/"+id+"/dns_records/"+id {
			_, _ = fmt.Fprint(w, `{"success":true,"result":{"id":"`+id+`"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer api.Close()
	p, err := NewCloudflareTXTProvider("example.org", id, token)
	if err != nil {
		t.Fatal(err)
	}
	p.endpoint = api.URL
	p.client = api.Client()
	rr := libdns.RR{Type: "TXT", Name: "_acme-challenge.sub", Data: strings.Repeat("x", 43)}
	saved, err := p.AppendRecords(context.Background(), "example.org.", []libdns.Record{rr})
	if err != nil || len(saved) != 1 {
		t.Fatalf("create failed: %v, %v", err, saved)
	}
	if _, ok := saved[0].(cloudflareTXTRecord); !ok {
		t.Fatal("returned record must retain the provider's created ID")
	}
	if _, err := p.DeleteRecords(context.Background(), "example.org.", []libdns.Record{rr}); err == nil {
		t.Fatal("must refuse deleting an unowned DNS record")
	}
	if _, err := p.DeleteRecords(context.Background(), "example.org.", saved); err != nil {
		t.Fatalf("delete owned record: %v", err)
	}
	if len(requests) != 2 || !strings.HasPrefix(requests[0], "POST ") || !strings.HasPrefix(requests[1], "DELETE ") {
		t.Fatalf("unintended DNS API requests: %v", requests)
	}
}

func TestCloudflareDNS01RejectsOtherRecordsOrZones(t *testing.T) {
	p, err := NewCloudflareTXTProvider("example.org", strings.Repeat("a", 32), strings.Repeat("x", 40))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		zone   string
		record libdns.RR
	}{
		{"another.org", libdns.RR{Type: "TXT", Name: "_acme-challenge", Data: strings.Repeat("x", 43)}},
		{"example.org", libdns.RR{Type: "TXT", Name: "_acme-challenge.evil.org.", Data: strings.Repeat("x", 43)}},
		{"example.org", libdns.RR{Type: "A", Name: "_acme-challenge", Data: strings.Repeat("x", 43)}},
		{"example.org", libdns.RR{Type: "TXT", Name: "www", Data: strings.Repeat("x", 43)}},
		{"example.org", libdns.RR{Type: "TXT", Name: "_acme-challenge", Data: "not-a-token"}},
	} {
		if _, err := p.AppendRecords(context.Background(), tc.zone, []libdns.Record{tc.record}); err == nil {
			t.Fatalf("accepted invalid ACME record: %#v", tc)
		}
	}
	if _, err := NewCloudflareTXTProvider("example.org", "not-id", "token"); err == nil {
		t.Fatal("accepted invalid provider configuration")
	}
}
