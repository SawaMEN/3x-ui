package githubrelease

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const metadataLimit = 4 << 20

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

var (
	releaseAPIPath = regexp.MustCompile(`^/repos/([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)/releases/(latest|tags/(.+))$`)
	tagPart        = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
)

// Fetch reads one GitHub release. If the REST API rejects the request with
// 403 or 429, GitHub's public release page can still provide the tag and assets.
func Fetch(ctx context.Context, client *http.Client, endpoint string) (Release, error) {
	req, err := request(ctx, http.MethodGet, endpoint)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		apiErr := fmt.Errorf("GitHub release API returned HTTP %d", resp.StatusCode)
		path := releaseAPIPath.FindStringSubmatch(req.URL.Path)
		if (resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests) ||
			req.URL.Scheme != "https" || req.URL.Host != "api.github.com" || len(path) == 0 {
			return Release{}, apiErr
		}
		rel, err := fetchWeb(ctx, client, path[1], path[3])
		if err != nil {
			return Release{}, fmt.Errorf("%w; release page fallback: %w", apiErr, err)
		}
		return rel, nil
	}
	defer resp.Body.Close()
	body, err := readMetadata(resp.Body)
	if err != nil {
		return Release{}, err
	}
	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return Release{}, err
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return Release{}, errors.New("GitHub release is missing a tag")
	}
	return rel, nil
}

func readMetadata(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, metadataLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > metadataLimit {
		return nil, errors.New("release metadata exceeds size limit")
	}
	return data, nil
}

func request(ctx context.Context, method, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "3x-ui")
	return req, nil
}

func fetchWeb(ctx context.Context, client *http.Client, repo, tag string) (Release, error) {
	releasePath := "/" + repo + "/releases/"
	baseURL := "https://github.com" + releasePath
	if tag == "" {
		req, err := request(ctx, http.MethodHead, baseURL+"latest")
		if err != nil {
			return Release{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return Release{}, err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return Release{}, fmt.Errorf("latest release page returned HTTP %d", resp.StatusCode)
		}
		location := resp.Request.URL
		prefix := releasePath + "tag/"
		if location.Scheme != "https" || location.Host != "github.com" ||
			location.User != nil || location.RawQuery != "" || !strings.HasPrefix(location.Path, prefix) {
			return Release{}, fmt.Errorf("unexpected latest release URL")
		}
		tag = strings.TrimPrefix(location.Path, prefix)
	}
	if len(tag) > 128 {
		return Release{}, fmt.Errorf("invalid release tag")
	}
	for _, part := range strings.Split(tag, "/") {
		if !tagPart.MatchString(part) || part == "." || part == ".." {
			return Release{}, fmt.Errorf("invalid release tag")
		}
	}

	assetsURL := baseURL + "expanded_assets/" + url.PathEscape(tag)
	req, err := request(ctx, http.MethodGet, assetsURL)
	if err != nil {
		return Release{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Request.URL.String() != assetsURL {
		return Release{}, fmt.Errorf("unexpected release assets response (HTTP %d)", resp.StatusCode)
	}
	page, err := readMetadata(resp.Body)
	if err != nil {
		return Release{}, err
	}
	assets, err := parseAssets(bytes.NewReader(page), releasePath+"download/"+tag+"/")
	if err != nil {
		return Release{}, err
	}
	return Release{TagName: tag, Assets: assets}, nil
}

func parseAssets(body io.Reader, downloadPath string) ([]Asset, error) {
	var assets []Asset
	digests := make(map[string]string)
	seen := make(map[string]struct{})
	tokens := html.NewTokenizer(body)
	for kind := tokens.Next(); kind != html.ErrorToken; kind = tokens.Next() {
		if kind != html.StartTagToken {
			continue
		}
		element := tokens.Token()
		switch element.Data {
		case "a":
			for _, attr := range element.Attr {
				if attr.Key != "href" {
					continue
				}
				link, err := url.Parse(attr.Val)
				if err != nil || link.IsAbs() || link.Host != "" || link.User != nil ||
					link.RawQuery != "" || link.Fragment != "" {
					continue
				}
				name, ok := strings.CutPrefix(link.Path, downloadPath)
				if ok && name != "" && name != "." && name != ".." &&
					!strings.ContainsAny(name, "/\\\x00\r\n") {
					if _, exists := seen[name]; exists {
						continue
					}
					seen[name] = struct{}{}
					assets = append(assets, Asset{Name: name, URL: "https://github.com" + link.EscapedPath()})
				}
			}
		case "clipboard-copy":
			var name, value string
			for _, attr := range element.Attr {
				switch attr.Key {
				case "aria-label":
					if assetName, ok := strings.CutPrefix(attr.Val, "Copy to clipboard digest for "); ok {
						name = assetName
					}
				case "value":
					value = attr.Val
				}
			}
			digest, ok := strings.CutPrefix(value, "sha256:")
			if name != "" && ok && len(digest) == sha256.Size*2 {
				if _, err := hex.DecodeString(digest); err == nil {
					normalized := "sha256:" + strings.ToLower(digest)
					if previous, exists := digests[name]; exists && previous != normalized {
						return nil, fmt.Errorf("conflicting SHA-256 digests for release asset %q", name)
					}
					digests[name] = normalized
				}
			}
		}
	}
	if err := tokens.Err(); !errors.Is(err, io.EOF) {
		return nil, err
	}
	for i := range assets {
		assets[i].Digest = digests[assets[i].Name]
	}
	return assets, nil
}
