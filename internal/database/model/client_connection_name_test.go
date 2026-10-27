package model

import "testing"

func TestClientConnectionNameModes(t *testing.T) {
	for _, tc := range []struct{ mode, remark, want string }{
		{ConnectionNameRemark, " Frankfurt ", "Frankfurt"},
		{ConnectionNameRemarkClient, "Frankfurt", "Frankfurt-alice"},
		{ConnectionNameClient, "Frankfurt", "alice"},
		{ConnectionNameRemark, "", "alice"},
		{ConnectionNameRemarkClient, " \t ", "alice"},
		{ConnectionNameClient, "", "alice"},
	} {
		t.Run(tc.mode+"/"+tc.remark, func(t *testing.T) {
			c := Client{Email: "alice", ConnectionNameMode: &tc.mode}
			if err := c.ValidateConnectionNameMode(); err != nil {
				t.Fatal(err)
			}
			if got := c.ToRecord().ToClient().ConnectionDisplayName(tc.remark); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	for _, mode := range []string{"", "invalid", "remark-client-client"} {
		c := Client{ConnectionNameMode: &mode}
		if err := c.ValidateConnectionNameMode(); err == nil {
			t.Fatalf("accepted %q", mode)
		}
	}
}

func TestMergeClientConnectionNameModePreservesOmissionAndNewerSelection(t *testing.T) {
	mode := ConnectionNameRemark
	existing := &ClientRecord{ConnectionNameMode: &mode, UpdatedAt: 2}
	MergeClientRecord(existing, &ClientRecord{UpdatedAt: 3})
	if existing.ConnectionNameMode == nil || *existing.ConnectionNameMode != mode {
		t.Fatal("omission erased mode")
	}
	next := ConnectionNameClient
	MergeClientRecord(existing, &ClientRecord{ConnectionNameMode: &next, UpdatedAt: 1})
	if *existing.ConnectionNameMode != mode {
		t.Fatal("stale snapshot changed mode")
	}
	MergeClientRecord(existing, &ClientRecord{ConnectionNameMode: &next, UpdatedAt: 4})
	if *existing.ConnectionNameMode != next {
		t.Fatal("newer selection lost")
	}
}
