package coreinstall

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestVersionsIncludeOnlyCompatibleReleaseAssets(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.github.com/repos/SagerNet/sing-box/releases?per_page=100" {
			t.Fatal(r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[
		{"tag_name":"v1.2.0-alpha.1","prerelease":true,"assets":[{"name":"sing-box-1.2.0-alpha.1-linux-amd64.tar.gz"}]},
		{"tag_name":"v1.1.0","assets":[{"name":"sing-box-1.1.0-linux-amd64.tar.gz"}]},
		{"tag_name":"v1.0.0","assets":[{"name":"sing-box-1.0.0-linux-arm64.tar.gz"}]},
		{"tag_name":"v0.9.0","draft":true,"assets":[{"name":"sing-box-0.9.0-linux-amd64.tar.gz"}]}
		]`))}, nil
	})}
	versions, err := (Installer{Client: client}).Versions(context.Background(), "sing-box-upstream", "linux", "amd64", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || !versions[0].Prerelease || versions[1].Tag != "v1.1.0" {
		t.Fatalf("unexpected versions: %+v", versions)
	}
	if _, err := (Installer{Client: client}).Versions(context.Background(), "sing-box", "linux", "amd64", ""); err == nil {
		t.Fatal("bundled core must not use upstream releases")
	}
}
