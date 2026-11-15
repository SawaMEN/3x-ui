package service

import (
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type telemtStatsResponseTransport struct {
	body   string
	status int
}

func TestMtprotoWebHistorySaturatesWithoutOverflow(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		stored, source, current int64
		seed                    bool
	}{
		{"first sample", 0, 0, math.MaxInt64, false},
		{"growing counter", database.TrafficMax - 1, 100, math.MaxInt64, true},
		{"reset counter", math.MaxInt64 - 1, 1000, 100, true},
		{"already at cap", database.TrafficMax, 100, 120, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupConflictDB(t)
			if tc.seed {
				row := model.TelemtUserHistory{Username: "alice", TotalBytes: tc.stored, SourceTotalBytes: tc.source}
				if err := database.GetDB().Create(&row).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := persistTelemtUserSnapshot([]telemtUserStats{{Username: "alice", TotalOctets: tc.current}}); err != nil {
				t.Fatal(err)
			}
			var row model.TelemtUserHistory
			if err := database.GetDB().Where("username = ?", "alice").Take(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.TotalBytes != database.TrafficMax || row.SourceTotalBytes != tc.current {
				t.Fatalf("unbounded history: total=%d source=%d; want %d/%d", row.TotalBytes, row.SourceTotalBytes, database.TrafficMax, tc.current)
			}
			// The cap must not prevent the source cursor or activity advancing.
			if err := persistTelemtUserSnapshot([]telemtUserStats{{Username: "alice", TotalOctets: tc.current, CurrentConnections: 1}}); err != nil {
				t.Fatal(err)
			}
			if err := database.GetDB().Where("username = ?", "alice").Take(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.TotalBytes != database.TrafficMax || row.LastSeenAt == 0 || row.CurrentConnections != 1 {
				t.Fatalf("capped history lost activity: %+v", row)
			}
		})
	}
}

func (r telemtStatsResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: r.status, Body: io.NopCloser(strings.NewReader(r.body)), Header: make(http.Header), Request: req}, nil
}

func TestMtprotoWebRejectsMalformedCounterSnapshots(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"missing users", `{"ok":true}`},
		{"null users", `{"ok":true,"data":null}`},
		{"missing counter", `{"ok":true,"data":[{"username":"alice"}]}`},
		{"null counter", `{"ok":true,"data":[{"username":"alice","total_octets":null}]}`},
		{"negative counter", `{"ok":true,"data":[{"username":"alice","total_octets":-1}]}`},
		{"empty identity", `{"ok":true,"data":[{"username":"  ","total_octets":10}]}`},
		{"duplicate identity", `{"ok":true,"data":[{"username":"alice","total_octets":110},{"username":"alice","total_octets":100}]}`},
		{"normalized duplicate", `{"ok":true,"data":[{"username":" alice ","total_octets":110},{"username":"alice","total_octets":120}]}`},
		{"trailing garbage", `{"ok":true,"data":[]}broken`},
		{"second document", `{"ok":true,"data":[]}{"ok":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: telemtStatsResponseTransport{body: tc.body, status: http.StatusOK}}
			users, err := fetchTelemtUserStats(client)
			if err == nil || len(users) != 0 {
				t.Fatalf("unsafe snapshot accepted: users=%+v err=%v", users, err)
			}
		})
	}
}

func TestMtprotoWebValidCounterSnapshots(t *testing.T) {
	for _, body := range []string{
		`{"ok":true,"data":[]}`,
		`{"ok":true,"data":[{"username":" alice ","total_octets":0,"current_connections":0}]}`,
		`{"ok":true,"data":[{"username":"alice","total_octets":9223372036854775807,"current_connections":1,"future_field":true}]}`,
	} {
		client := &http.Client{Transport: telemtStatsResponseTransport{body: body, status: http.StatusOK}}
		users, err := fetchTelemtUserStats(client)
		if err != nil {
			t.Fatalf("valid response rejected: %v", err)
		}
		if len(users) > 0 && users[0].Username != "alice" {
			t.Fatalf("identity not normalized: %+v", users)
		}
	}
}

func TestMtprotoWebStatsPreserveRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &http.Client{Transport: telemtMonitorTransport{ctx: ctx}}
	if _, err := fetchTelemtUserStats(client); err == nil {
		t.Fatal("canceled stats request succeeded")
	}
}

func TestMtprotoWebMalformedStatsDoNotRecountHistory(t *testing.T) {
	for _, broken := range []string{
		`{"ok":true,"data":[{"username":"alice"}]}`,
		`{"ok":true,"data":[{"username":"alice","total_octets":null}]}`,
		`{"ok":true,"data":[{"username":"alice","total_octets":-1}]}`,
		`{"ok":true,"data":[{"username":"alice","total_octets":110},{"username":"alice","total_octets":100}]}`,
	} {
		t.Run(broken, func(t *testing.T) {
			setupConflictDB(t)
			poll := func(body string) error {
				users, err := fetchTelemtUserStats(&http.Client{Transport: telemtStatsResponseTransport{body: body, status: http.StatusOK}})
				if err != nil {
					return err
				}
				return persistTelemtUserSnapshot(users)
			}
			if err := poll(`{"ok":true,"data":[{"username":"alice","total_octets":100}]}`); err != nil {
				t.Fatal(err)
			}
			// Exercise the same successful-fetch gate used by both WEB traffic
			// and the history monitor. A failed fetch must not reach persistence.
			_ = poll(broken)
			if err := poll(`{"ok":true,"data":[{"username":"alice","total_octets":120}]}`); err != nil {
				t.Fatal(err)
			}
			var row model.TelemtUserHistory
			if err := database.GetDB().Where("username = ?", "alice").Take(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.TotalBytes != 120 || row.SourceTotalBytes != 120 {
				t.Fatalf("malformed poll corrupted accounting: total=%d source=%d; want 120/120", row.TotalBytes, row.SourceTotalBytes)
			}
		})
	}
}
