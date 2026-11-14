package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

type RemoteCoverSite struct {
	Domain       string `json:"domain"`
	SiteCatalog  string `json:"siteCatalog"`
	SiteTemplate string `json:"siteTemplate"`
	SiteHTML     string `json:"siteHtml,omitempty"`
}

func (r *Remote) ListLocalCoverSites(ctx context.Context) ([]RemoteCoverSite, error) {
	return r.listLocalCoverSites(ctx, "panel/api/domains/cover-sites/local")
}

func (r *Remote) ListLocalCoverCertificateSources(ctx context.Context) ([]RemoteCoverSite, error) {
	return r.listLocalCoverSites(ctx, "panel/api/domains/cover-sites/local?certificates=1")
}

func (r *Remote) listLocalCoverSites(ctx context.Context, path string) ([]RemoteCoverSite, error) {
	response, err := r.do(ctx, http.MethodGet, path, nil)
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

type RemoteCoverSiteContent struct {
	HTML string `json:"html"`
}

func (r *Remote) ReadLocalCoverSiteContent(ctx context.Context, domain string) (RemoteCoverSiteContent, error) {
	var content RemoteCoverSiteContent
	response, err := r.do(ctx, http.MethodGet, "panel/api/domains/cover-sites/content/local?domain="+url.QueryEscape(domain), nil)
	if err != nil {
		return content, err
	}
	if err := json.Unmarshal(response.Obj, &content); err != nil {
		return content, errors.New("node cover-site content is invalid")
	}
	return content, nil
}
