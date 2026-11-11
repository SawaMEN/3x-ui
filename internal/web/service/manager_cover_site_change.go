package service

import (
    "context"
    "errors"
    "fmt"

    "github.com/SawaMEN/3x-ui/v3/internal/database"
    "github.com/SawaMEN/3x-ui/v3/internal/database/model"
    "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

// ManagedCoverSiteChange changes only the static site template on an existing
// owned SNI-only domain. It cannot create names, replace ACME settings,
// change ports or alter inbound routes.
type ManagedCoverSiteChange struct {
    NodeID       int    `json:"nodeId"`
    Domain       string `json:"domain"`
    SiteCatalog  string `json:"siteCatalog"`
    SiteTemplate string `json:"siteTemplate"`
}

func ChangeLocalCoverSiteTemplate(ctx context.Context, change ManagedCoverSiteChange) (ManagedCoverSiteSummary, error) {
    var empty ManagedCoverSiteSummary
    domain, err := managerDomain(change.Domain)
    if err != nil {
        return empty, err
    }
    if err := validateTelemtWebSite(&TelemtWebSiteOptions{
        Mode: "catalog", Catalog: change.SiteCatalog, Template: change.SiteTemplate,
    }); err != nil {
        return empty, fmt.Errorf("invalid cover-site template: %w", err)
    }

    managerCoverSiteMu.Lock()
    defer managerCoverSiteMu.Unlock()

    current, err := (&SettingService{}).GetDomainFrontConfig()
    if err != nil {
        return empty, err
    }
    found := false
    for i := range current.Domains {
        entry := &current.Domains[i]
        if entry.Name != domain {
            continue
        }
        if !entry.Enabled || entry.Mode != "sni_only" {
            return empty, errors.New("only an existing enabled SNI-only website can change its template")
        }
        if entry.SiteCatalog == change.SiteCatalog && entry.SiteTemplate == change.SiteTemplate {
            return ManagedCoverSiteSummary{
                Domain: domain, SiteCatalog: change.SiteCatalog, SiteTemplate: change.SiteTemplate,
            }, nil
        }
        entry.SiteCatalog = change.SiteCatalog
        entry.SiteTemplate = change.SiteTemplate
        found = true
        break
    }
    if !found {
        return empty, errors.New("SNI cover site not found on selected node")
    }
    if _, err := ApplyDomainFront(ctx, current); err != nil {
        return empty, err
    }
    return ManagedCoverSiteSummary{
        Domain: domain, SiteCatalog: change.SiteCatalog, SiteTemplate: change.SiteTemplate,
    }, nil
}

func ChangeManagedCoverSiteTemplate(ctx context.Context, change ManagedCoverSiteChange) (ManagedCoverSiteSummary, error) {
    var empty ManagedCoverSiteSummary
    if change.NodeID < 0 {
        return empty, errors.New("invalid node")
    }
    if change.NodeID == 0 {
        return ChangeLocalCoverSiteTemplate(ctx, change)
    }
    if database.GetDB() == nil {
        return empty, errors.New("node database unavailable")
    }
    var node model.Node
    if err := database.GetDB().First(&node, change.NodeID).Error; err != nil {
        return empty, err
    }
    if !node.Enable || node.Transitive || node.Scheme != "https" || node.TlsVerifyMode == "skip" {
        return empty, errors.New("cover-site changes require a direct node on verified HTTPS")
    }
    manager := runtime.GetManager()
    if manager == nil {
        return empty, errors.New("node manager unavailable")
    }
    remote, err := manager.RemoteFor(&node)
    if err != nil {
        return empty, err
    }
    domain, err := managerDomain(change.Domain)
    if err != nil {
        return empty, err
    }
    if err := remote.ChangeLocalCoverSiteTemplate(ctx, runtime.RemoteCoverSite{
        Domain: domain, SiteCatalog: change.SiteCatalog, SiteTemplate: change.SiteTemplate,
    }); err != nil {
        return empty, fmt.Errorf("remote cover-site change failed: %w", err)
    }
    return ManagedCoverSiteSummary{
        NodeID: change.NodeID, Domain: domain,
        SiteCatalog: change.SiteCatalog, SiteTemplate: change.SiteTemplate,
    }, nil
}
