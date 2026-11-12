package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

type RemoteCoverSite struct {
	Domain       string `json:"domain"`
	SiteCatalog  string `json:"siteCatalog"`
	SiteTemplate string `json:"siteTemplate"`
}

func (r *Remote) ListLocalCoverSites(ctx context.Context) ([]RemoteCoverSite, error) {
	response, err := r.do(ctx, http.MethodGet, "panel/api/domains/cover-sites/local", nil)
	if err != nil {
		return nil, err
	}
	var sites []RemoteCoverSite
	if err := json.Unmarshal(response.Obj, &sites); err != nil {
		return nil, errors.New("node cover-site inventory is invalid")
	}
	if len(sites) > 100 {
		return nil, errors.New("node returned too many cover sites")
	}
	return sites, nil
}

func (r *Remote) ChangeLocalCoverSiteTemplate(ctx context.Context, site RemoteCoverSite) error {
	if site.Domain == "" || site.SiteCatalog == "" || site.SiteTemplate == "" {
		return errors.New("incomplete cover-site template change")
	}
	_, err := r.do(ctx, http.MethodPost, "panel/api/domains/cover-sites/template", site)
	return err
}
