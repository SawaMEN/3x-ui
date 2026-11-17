package coreinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"time"
)

type Version struct {
	Tag        string `json:"tag"`
	Prerelease bool   `json:"prerelease"`
}

// GitHub occasionally returns 5xx for its large release listings. Retrying with
// fewer releases avoids repeatedly requesting the same expensive response.
func temporaryReleaseAPIError(err error) bool {
	var status *httpStatusError
	if errors.As(err, &status) {
		switch status.code {
		case 429, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) && networkErr.Timeout()
}

// Return only releases with a binary for this machine; GitHub orders newest first.
func (i Installer) Versions(ctx context.Context, engine, goos, arch, arm string) ([]Version, error) {
	repo, err := repository(engine)
	if err != nil {
		return nil, err
	}
	listKey := versionListKey(repo, goos, arch, arm)
	if cached, ok := i.rememberedVersions(listKey); ok {
		return cached, nil
	}
	// Preserve the full list when GitHub is healthy. When a large response
	// exceeds the size cap, times out, or fails at the gateway, try smaller
	// pages rather than failing both Xray and sing-box version selectors.
	attempts := []struct {
		pageSize int
		delay    time.Duration
	}{
		{100, 0},
		{30, 200 * time.Millisecond},
		{10, 400 * time.Millisecond},
		{5, 800 * time.Millisecond},
	}
	var raw []byte
	for index, attempt := range attempts {
		if index > 0 {
			timer := time.NewTimer(attempt.delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=%d", repo, attempt.pageSize)
		raw, err = i.get(ctx, endpoint, 8<<20)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Large GitHub release payloads can exceed our JSON size guard even
		// with HTTP 200. Request smaller pages instead of returning the
		// misleading "download exceeds size limit" error to both selectors.
		if !temporaryReleaseAPIError(err) && !errors.Is(err, errDownloadExceedsLimit) {
			return nil, err
		}
	}
	if err != nil {
		return nil, fmt.Errorf("GitHub release list unavailable after retrying smaller responses: %w", err)
	}
	var releases []struct {
		Release
		Draft      bool `json:"draft"`
		Prerelease bool `json:"prerelease"`
	}
	if err := json.Unmarshal(raw, &releases); err != nil {
		return nil, err
	}
	versions := []Version{}
	seen := map[string]bool{}
	valid := regexp.MustCompile(`^v?[0-9][A-Za-z0-9._-]{0,79}$`)
	for _, rel := range releases {
		if rel.Draft || seen[rel.Tag] || !valid.MatchString(rel.Tag) {
			continue
		}
		name, err := AssetName(engine, rel.Tag, goos, arch, arm)
		if err != nil {
			return nil, err
		}
		for _, asset := range rel.Assets {
			if asset.Name == name {
				versions = append(versions, Version{Tag: rel.Tag, Prerelease: rel.Prerelease})
				i.rememberRelease(repo, rel.Release, asset)
				seen[rel.Tag] = true
				break
			}
		}
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no compatible releases available for %s/%s", goos, arch)
	}
	i.rememberVersions(listKey, versions)
	return versions, nil
}
