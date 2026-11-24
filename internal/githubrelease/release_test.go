package githubrelease

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestFetchTaggedReleaseAfterForbidden(t *testing.T) {
	const api = "https://api.github.com/repos/owner/project/releases/tags/app%2Fv1.2.3"
	const web = "https://github.com/owner/project/releases/expanded_assets/app%2Fv1.2.3"
	const download = "/owner/project/releases/download/app%2Fv1.2.3/"
	digest := "sha256:" + strings.Repeat("a", 64)
	var requests []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		address := req.URL.String()
		requests = append(requests, address)
		resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: req}
		switch address {
		case api:
			resp.StatusCode = http.StatusForbidden
			resp.Body = http.NoBody
		case web:
			page := fmt.Sprintf(`
<a href="%sengine-linux.tar.gz">engine-linux.tar.gz</a>
<a href="https://other.example%sengine-linux.tar.gz">external asset</a>
<a href="%sengine-linux.tar.gz.sha256">checksum file</a>
<a href="/owner/project/archive/refs/tags/app%%2Fv1.2.3.tar.gz">Source code</a>
<clipboard-copy value="%s" aria-label="Copy to clipboard digest for engine-linux.tar.gz"></clipboard-copy>`, download, download, download, digest)
			resp.Body = io.NopCloser(strings.NewReader(page))
		default:
			return nil, fmt.Errorf("unexpected URL %s", address)
		}
		return resp, nil
	})}

	rel, err := Fetch(t.Context(), client, api)
	want := []Asset{
		{Name: "engine-linux.tar.gz", URL: "https://github.com" + download + "engine-linux.tar.gz", Digest: digest},
		{Name: "engine-linux.tar.gz.sha256", URL: "https://github.com" + download + "engine-linux.tar.gz.sha256"},
	}
	if err != nil || rel.TagName != "app/v1.2.3" || !slices.Equal(rel.Assets, want) {
		t.Fatalf("release = %+v, error = %v", rel, err)
	}
	if !slices.Equal(requests, []string{api, web}) {
		t.Fatalf("requests = %v", requests)
	}
}

func TestFetchRejectsInvalidReleaseMetadata(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"tag_name":"v1"}{}`,
		`{"tag_name":"v1"}` + strings.Repeat(" ", metadataLimit),
	} {
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Request:    req,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})}
		if _, err := Fetch(t.Context(), client, "https://api.github.com/repos/owner/project/releases/latest"); err == nil {
			t.Fatal("accepted invalid release metadata")
		}
	}
}

func TestParseAssetsDeduplicatesLinks(t *testing.T) {
	const prefix = "/owner/project/releases/download/v1/"
	const page = `<a href="/owner/project/releases/download/v1/file.tar.gz">file</a><a href="/owner/project/releases/download/v1/file.tar.gz">duplicate</a>`
	assets, err := parseAssets(strings.NewReader(page), prefix)
	if err != nil || len(assets) != 1 || assets[0].Name != "file.tar.gz" {
		t.Fatalf("assets = %+v, error = %v", assets, err)
	}
}

func TestParseAssetsRejectsConflictingDigests(t *testing.T) {
	const prefix = "/owner/project/releases/download/v1/"
	first := strings.Repeat("a", 64)
	second := strings.Repeat("b", 64)
	page := func(other string) string {
		return fmt.Sprintf(`<a href="%sfile.tar.gz">file</a><clipboard-copy value="sha256:%s" aria-label="Copy to clipboard digest for file.tar.gz"></clipboard-copy><clipboard-copy value="sha256:%s" aria-label="Copy to clipboard digest for file.tar.gz"></clipboard-copy>`, prefix, first, other)
	}
	if _, err := parseAssets(strings.NewReader(page(second)), prefix); err == nil {
		t.Fatal("accepted conflicting asset digests")
	}
	assets, err := parseAssets(strings.NewReader(page(strings.ToUpper(first))), prefix)
	if err != nil || len(assets) != 1 || assets[0].Digest != "sha256:"+first {
		t.Fatalf("identical digests must be accepted: %+v, %v", assets, err)
	}
}

func TestFetchPreservesHTTPErrorStatus(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: http.NoBody, Request: req}, nil
	})}
	_, err := Fetch(t.Context(), client, "https://api.github.com/repos/owner/project/releases/latest")
	var statusErr *HTTPError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a typed 404 response, got %v", err)
	}
}
