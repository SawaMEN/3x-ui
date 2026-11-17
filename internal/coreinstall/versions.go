package coreinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
)

type Version struct {
	Tag        string `json:"tag"`
	Prerelease bool   `json:"prerelease"`
}

// Return only releases with a binary for this machine; GitHub orders newest first.
func (i Installer) Versions(ctx context.Context, engine, goos, arch, arm string) ([]Version, error) {
	repo, err := repository(engine)
	if err != nil {
		return nil, err
	}
	raw, err := i.get(ctx, "https://api.github.com/repos/"+repo+"/releases?per_page=100", 8<<20)
	if err != nil {
		return nil, err
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
				seen[rel.Tag] = true
				break
			}
		}
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no compatible releases available for %s/%s", goos, arch)
	}
	return versions, nil
}
