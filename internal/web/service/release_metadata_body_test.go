package service

import (
	"os"
	"path/filepath"
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

func TestReleaseMetadataAvoidsRedundantSettingWrites(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "binary")
	if err := writeReleaseMetadata(binary, " v1 "); err != nil {
		t.Fatal(err)
	}
	stored := "v1"
	writes := 0
	get := func() (string, error) { return stored, nil }
	set := func(tag string) error {
		stored = tag
		writes++
		return nil
	}
	if got := resolveInstalledReleaseTag(binary, get, set); got != "v1" || writes != 0 {
		t.Fatalf("unchanged tag = %q, writes = %d", got, writes)
	}
	if err := writeReleaseMetadata(binary, "v2"); err != nil {
		t.Fatal(err)
	}
	if got := resolveInstalledReleaseTag(binary, get, set); got != "v2" || writes != 1 {
		t.Fatalf("changed tag = %q, writes = %d", got, writes)
	}
	if err := writeReleaseMetadata(binary, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(releaseMetadataPath(binary)); !os.IsNotExist(err) {
		t.Fatalf("metadata was not removed: %v", err)
	}
	if err := writeReleaseMetadata(binary, ""); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseMetadataReportsRemovalFailure(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "binary")
	path := releaseMetadataPath(binary)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeReleaseMetadata(binary, ""); err == nil {
		t.Fatal("removal failure was ignored")
	}
}
