package clientlimit

import "testing"

func TestStage6ConflictingRatesCannotSplitSharedBudget(t *testing.T) {
	s := New("127.0.0.1:0", "127.0.0.1:0")
	defer s.Close()
	if _, err := s.Apply(Plan{Routes: []Route{{ID: "one", Email: "alice", UploadMbps: 1}, {ID: "two", Email: "alice", UploadMbps: 2}}}); err == nil {
		t.Fatal("conflicting rates created two budgets for one client")
	}
	if s.ln != nil || len(s.entries) != 0 {
		t.Fatal("invalid plan changed the live state")
	}
}
