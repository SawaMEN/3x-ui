package coreinstall

import (
	"context"
	"errors"
	"fmt"
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

func TestVersionsRecoverFromTemporaryGitHubErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				wantSize := "100"
				if calls == 2 {
					wantSize = "30"
				}
				if r.URL.Path != "/repos/SagerNet/sing-box/releases" || r.URL.Query().Get("per_page") != wantSize {
					t.Fatalf("unexpected release request: %s", r.URL)
				}
				if calls == 1 {
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[
					{"tag_name":"v1.2.0","assets":[{"name":"sing-box-1.2.0-linux-amd64.tar.gz"}]},
					{"tag_name":"v1.1.0","assets":[{"name":"sing-box-1.1.0-linux-arm64.tar.gz"}]}
				]`)), Header: make(http.Header)}, nil
			})}
			versions, err := (Installer{Client: client}).Versions(context.Background(), "sing-box-upstream", "linux", "amd64", "")
			if err != nil || len(versions) != 1 || versions[0].Tag != "v1.2.0" || calls != 2 {
				t.Fatalf("versions=%v calls=%d err=%v", versions, calls, err)
			}
		})
	}
}

func TestVersionsDoesNotRetryPermanentGitHubError(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	_, err := (Installer{Client: client}).Versions(context.Background(), "sing-box-upstream", "linux", "amd64", "")
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") || calls != 1 {
		t.Fatalf("permanent error was retried or masked: calls=%d err=%v", calls, err)
	}
}

func TestVersionsStopsAfterBoundedRetries(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		wantSizes := []string{"100", "30", "10", "10"}
		if calls >= len(wantSizes) || r.URL.Query().Get("per_page") != wantSizes[calls] {
			t.Fatalf("unexpected attempt %d: %s", calls+1, r.URL)
		}
		calls++
		return &http.Response{StatusCode: http.StatusGatewayTimeout, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	_, err := (Installer{Client: client}).Versions(context.Background(), "sing-box-upstream", "linux", "amd64", "")
	if err == nil || !strings.Contains(err.Error(), "HTTP 504") || calls != 4 {
		t.Fatalf("expected four bounded attempts, got calls=%d err=%v", calls, err)
	}
}

func TestVersionsRetryRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		cancel()
		return &http.Response{StatusCode: http.StatusGatewayTimeout, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	_, err := (Installer{Client: client}).Versions(ctx, "sing-box-upstream", "linux", "amd64", "")
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancelled request retried: calls=%d err=%v", calls, err)
	}
}
