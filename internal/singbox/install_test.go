package singbox

import (
	"reflect"
	"testing"
)

func TestReleaseVersionsIncludesPrereleases(t *testing.T) {
	releases := []releaseListInfo{
		{TagName: "v1.12.0", Prerelease: false},
		{TagName: "v1.13.0-beta.1", Prerelease: true},
		{TagName: "", Prerelease: true},
	}

	want := []ReleaseVersion{
		{Version: "v1.12.0", Prerelease: false},
		{Version: "v1.13.0-beta.1", Prerelease: true},
	}
	if got := releaseVersions(releases); !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %#v, want %#v", got, want)
	}
}

func TestIsPreReleaseVersion(t *testing.T) {
	stable := []string{"v1.12.0", "v1.13.0"}
	for _, version := range stable {
		if isPreReleaseVersion(version) {
			t.Fatalf("isPreReleaseVersion(%q) = true", version)
		}
	}

	unstable := []string{
		"v1.13.0-alpha.1",
		"v1.13.0-beta.2",
		"v1.13.0-rc.1",
		"v1.13.0-pre",
		"v1.13.0-dev",
		"v1.13.0-nightly.20260924",
	}
	for _, version := range unstable {
		if !isPreReleaseVersion(version) {
			t.Fatalf("isPreReleaseVersion(%q) = false", version)
		}
	}
}
