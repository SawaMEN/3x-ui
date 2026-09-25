package naiveproxy

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

func TestMergeNaiveUsersPrefersNormalizedCredentials(t *testing.T) {
	got := mergeNaiveUsers(
		[]model.ClientRecord{{Email: "alice@example.com", Password: "normalized", Enable: true}},
		[]model.Client{{Email: "alice@example.com", Password: "legacy", Enable: true}},
		nil,
	)
	if len(got) != 1 || got[0].Password != "normalized" {
		t.Fatalf("expected normalized password, got %#v", got)
	}
}

func TestMergeNaiveUsersFallsBackToLegacyPassword(t *testing.T) {
	got := mergeNaiveUsers(
		[]model.ClientRecord{{Email: "alice@example.com", Enable: true}},
		[]model.Client{{Email: "alice@example.com", Password: "legacy", Enable: true}},
		nil,
	)
	if len(got) != 1 || got[0].Password != "legacy" {
		t.Fatalf("expected legacy password fallback, got %#v", got)
	}
}

func TestMergeNaiveUsersDoesNotResurrectDisabledNormalizedClient(t *testing.T) {
	got := mergeNaiveUsers(
		[]model.ClientRecord{{Email: "alice@example.com", Enable: false}},
		[]model.Client{{Email: "alice@example.com", Password: "legacy", Enable: true}},
		nil,
	)
	if len(got) != 0 {
		t.Fatalf("disabled normalized client must stay disabled, got %#v", got)
	}
}

func TestMergeNaiveUsersHonorsTrafficDisableAndLegacyOnlyClients(t *testing.T) {
	got := mergeNaiveUsers(
		[]model.ClientRecord{{Email: "alice@example.com", Password: "a", Enable: true}},
		[]model.Client{
			{Email: "alice@example.com", Password: "old-a", Enable: true},
			{Email: "bob@example.com", Password: "b", Enable: true},
		},
		[]xray.ClientTraffic{
			{Email: "alice@example.com", Enable: false},
			{Email: "bob@example.com", Enable: true},
		},
	)
	if len(got) != 1 || got[0].Username != "bob@example.com" || got[0].Password != "b" {
		t.Fatalf("expected only enabled legacy client bob, got %#v", got)
	}
}
