package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type ManagedCoverSiteContent struct {
	HTML string `json:"html"`
}

// The inventory endpoint remains lightweight. Only an explicitly requested
// custom page is returned to an authenticated admin/master node.
func LocalManagedCoverSiteContent(host string) (ManagedCoverSiteContent, error) {
	var empty ManagedCoverSiteContent
	domain, err := managerDomain(host)
	if err != nil {
		return empty, err
	}
	cfg, err := (&SettingService{}).GetDomainFrontConfig()
	if err != nil {
		return empty, err
	}
	for _, site := range cfg.Domains {
		if site.Name == domain && site.Enabled && site.Mode == "sni_only" {
			if site.SiteCatalog != "custom" || site.SiteTemplate != "html" {
				return empty, errors.New("selected cover site does not have custom HTML")
			}
			return ManagedCoverSiteContent{HTML: site.SiteHTML}, nil
		}
	}
	return empty, errors.New("cover site not found")
}

func ReadManagedCoverSiteContent(ctx context.Context, nodeID int, host string) (ManagedCoverSiteContent, error) {
	var empty ManagedCoverSiteContent
	domain, err := managerDomain(host)
	if err != nil {
		return empty, err
	}
	if nodeID < 0 {
		return empty, errors.New("invalid node")
	}
	if nodeID == 0 {
		return LocalManagedCoverSiteContent(domain)
	}
	if database.GetDB() == nil {
		return empty, errors.New("node database unavailable")
	}
	var node model.Node
	if err := database.GetDB().First(&node, nodeID).Error; err != nil {
		return empty, err
	}
	if !node.Enable || node.Transitive || node.Scheme != "https" || node.TlsVerifyMode == "skip" {
		return empty, errors.New("cover-site content requires a direct node on verified HTTPS")
	}
	manager := runtime.GetManager()
	if manager == nil {
		return empty, errors.New("node manager unavailable")
	}
	remote, err := manager.RemoteFor(&node)
	if err != nil {
		return empty, err
	}
	content, err := remote.ReadLocalCoverSiteContent(ctx, domain)
	if err != nil {
		return empty, fmt.Errorf("read remote cover site: %w", err)
	}
	return ManagedCoverSiteContent{HTML: content.HTML}, nil
}
