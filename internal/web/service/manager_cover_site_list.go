package service

import (
    "context"
    "sort"
    "sync"
    "time"

    "github.com/SawaMEN/3x-ui/v3/internal/database"
    "github.com/SawaMEN/3x-ui/v3/internal/database/model"
    "github.com/SawaMEN/3x-ui/v3/internal/domainfront"
    "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

// ManagedCoverSiteSummary intentionally excludes DNS credentials, listener
// settings, certificates, key paths and other node configuration.
type ManagedCoverSiteSummary struct {
    NodeID       int    `json:"nodeId"`
    Domain       string `json:"domain"`
    SiteCatalog  string `json:"siteCatalog"`
    SiteTemplate string `json:"siteTemplate"`
}

func coverSiteSummariesFromConfig(nodeID int, cfg domainfront.DomainFrontConfig) []ManagedCoverSiteSummary {
    var sites []ManagedCoverSiteSummary
    for _, d := range cfg.Domains {
        if !d.Enabled || d.Mode != "sni_only" {
            continue
        }
        catalog, template := d.SiteCatalog, d.SiteTemplate
        if catalog == "" && template == "" {
            catalog, template = "builtin", "notes"
        }
        sites = append(sites, ManagedCoverSiteSummary{
            NodeID: nodeID, Domain: d.Name, SiteCatalog: catalog, SiteTemplate: template,
        })
    }
    return sites
}

// LocalManagedCoverSites exposes only the already enabled SNI-only domains on
// this node. It never exposes the full Domain Front configuration.
func LocalManagedCoverSites() ([]ManagedCoverSiteSummary, error) {
    status, err := DomainFrontStatus()
    if err != nil {
        return nil, err
    }
    return coverSiteSummariesFromConfig(0, status.Config), nil
}

// ManagedCoverSites lists editable own websites on the main and direct nodes.
// It is administrative UI metadata only, never a general remote API.
func ManagedCoverSites(ctx context.Context) ([]ManagedCoverSiteSummary, error) {
    status, err := DomainFrontStatus()
    if err != nil {
        return nil, err
    }
    sites := coverSiteSummariesFromConfig(0, status.Config)
    db, manager := database.GetDB(), runtime.GetManager()
    if db != nil && manager != nil {
        var nodes []model.Node
        if err := db.Where("enable = ?", true).Limit(64).Find(&nodes).Error; err == nil {
            remoteCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
            defer cancel()
            results := make(chan []ManagedCoverSiteSummary, len(nodes))
            limit := make(chan struct{}, 6)
            var wg sync.WaitGroup
            for _, node := range nodes {
                if node.Id <= 0 || node.Transitive {
                    continue
                }
                wg.Add(1)
                go func(node model.Node) {
                    defer wg.Done()
                    select {
                    case limit <- struct{}{}:
                        defer func() { <-limit }()
                    case <-remoteCtx.Done():
                        return
                    }
                    remote, err := manager.RemoteFor(&node)
                    if err != nil {
                        return
                    }
                    remoteSites, err := remote.ListLocalCoverSites(remoteCtx)
                    if err != nil {
                        return
                    }
                    batch := make([]ManagedCoverSiteSummary, 0, len(remoteSites))
                    for _, site := range remoteSites {
                        domain, err := managerDomain(site.Domain)
                        if err != nil || domain != site.Domain {
                            continue
                        }
                        batch = append(batch, ManagedCoverSiteSummary{
                            NodeID: node.Id, Domain: domain,
                            SiteCatalog: site.SiteCatalog, SiteTemplate: site.SiteTemplate,
                        })
                    }
                    results <- batch
                }(node)
            }
            wg.Wait()
            close(results)
            for batch := range results {
                sites = append(sites, batch...)
            }
        }
    }
    sort.Slice(sites, func(i, j int) bool {
        if sites[i].NodeID != sites[j].NodeID {
            return sites[i].NodeID < sites[j].NodeID
        }
        return sites[i].Domain < sites[j].Domain
    })
    return sites, nil
}
