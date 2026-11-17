package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestProbePanelEdition(t *testing.T) {
	tests := []struct {
		name        string
		editionJSON string
		want        string
	}{
		{"fork", `,"panelEdition":"sawamen"`, "sawamen"},
		{"original without marker", "", "unmarked"},
		{"old fork without marker", `,"panelEdition":""`, "unmarked"},
		{"other fork", `,"panelEdition":"someone-else"`, "unmarked"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"success":true,"obj":{"panelVersion":"v3.0","xray":{"state":"running"}%s}}`, tc.editionJSON)
			}))
			defer srv.Close()

			u, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(u.Port())
			if err != nil {
				t.Fatal(err)
			}
			node := &model.Node{
				Id: 1, Name: "test", Scheme: "http", Address: u.Hostname(),
				Port: port, BasePath: "/", AllowPrivateAddress: true,
			}
			patch, err := (&NodeService{}).Probe(context.Background(), node)
			if err != nil {
				t.Fatalf("Probe failed: %v", err)
			}
			if patch.PanelEdition != tc.want {
				t.Fatalf("PanelEdition = %q, want %q", patch.PanelEdition, tc.want)
			}
			if got := patch.ToUI(true).PanelEdition; got != tc.want {
				t.Fatalf("UI edition = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProbePanelEditionRetainedOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	node := &model.Node{
		Id: 1, Name: "test", Scheme: "http", Address: u.Hostname(),
		Port: port, BasePath: "/", AllowPrivateAddress: true, PanelEdition: "sawamen",
	}
	patch, err := (&NodeService{}).Probe(context.Background(), node)
	if err == nil {
		t.Fatal("expected probe to fail")
	}
	if patch.PanelEdition != "sawamen" {
		t.Fatalf("PanelEdition = %q, want preserved sawamen", patch.PanelEdition)
	}
}
