package service

import (
	"strings"
	"testing"
)

func TestReleaseMetadataBodyBounds(t *testing.T) {
	for _, test := range []struct {
		name, body string
		wantError  bool
	}{
		{"valid", `{"tag_name":"v3.0.0"}`, false},
		{"trailing garbage", `{"tag_name":"v3.0.0"} invalid`, true},
		{"oversized", `{"tag_name":"` + strings.Repeat("x", maxReleaseMetadataBytes) + `"}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var release Release
			err := decodeReleaseMetadata(strings.NewReader(test.body), &release)
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v, wantError=%v", err, test.wantError)
			}
		})
	}
}
