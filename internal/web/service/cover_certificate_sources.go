package service

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

// Certificate sources differ from editable templates: Telemt WEB also owns a
// website identity, but its content is not edited through Domain Front.
func LocalCoverCertificateSources() ([]ManagedCoverSiteSummary, error) {
	sites, err := LocalManagedCoverSites()
	if err != nil {
		return nil, err
	}
	state, err := readTelemtWebState()
	if err != nil {
		logger.Errorf("Certificate source Telemt state read failed: %s", logger.ErrorSummary(err))
	}
	if err == nil && state.Enabled && telemtWebFrontendActive(state) {
		sites = appendTelemtCertificateSources(sites, telemtWebHosts(state))
	}
	if sites == nil {
		sites = []ManagedCoverSiteSummary{}
	}
	return sites, nil
}

func appendTelemtCertificateSources(sites []ManagedCoverSiteSummary, hosts []string) []ManagedCoverSiteSummary {
	seen := map[string]bool{}
	for _, site := range sites {
		seen[site.Domain] = true
	}
	for _, host := range hosts {
		name, err := distributionDomain(host)
		if err != nil || seen[name] {
			continue
		}
		seen[name] = true
		sites = append(sites, ManagedCoverSiteSummary{Domain: name, SiteCatalog: "telemt"})
	}
	return sites
}

type CoverCertificateSourceNode struct {
	NodeID   int    `json:"nodeId"`
	NodeName string `json:"nodeName"`
	State    string `json:"state"`
}

type CoverCertificateSourceReport struct {
	Sites []ManagedCoverSiteSummary    `json:"sites"`
	Nodes []CoverCertificateSourceNode `json:"nodes"`
}

func coverSourceErrorState(err error) string {
	switch {
	case runtime.IsRemoteHTTPStatus(err, http.StatusNotFound), runtime.IsRemoteHTTPStatus(err, http.StatusMethodNotAllowed):
		return "update_required"
	case runtime.IsRemoteHTTPStatus(err, http.StatusUnauthorized):
		return "unauthorized"
	case runtime.IsRemoteHTTPStatus(err, http.StatusForbidden):
		return "access_denied"
	default:
		// Remote errors may contain response bodies or credentials. Expose a code,
		// never the node's error text, address, token, or private certificate paths.
		return "unavailable"
	}
}

func CoverCertificateSources(ctx context.Context) (CoverCertificateSourceReport, error) {
	sites, err := LocalCoverCertificateSources()
	report := CoverCertificateSourceReport{Sites: sites, Nodes: []CoverCertificateSourceNode{}}
	if err != nil {
		return report, err
	}
	db, manager := database.GetDB(), runtime.GetManager()
	if db == nil {
		return report, nil
	}
	var nodes []model.Node
	if err := db.Where("enable = ?", true).Limit(64).Find(&nodes).Error; err != nil {
		logger.Errorf("Certificate source node inventory failed: %s", logger.ErrorSummary(err))
		return report, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	type result struct {
		node  CoverCertificateSourceNode
		sites []ManagedCoverSiteSummary
	}
	results := make(chan result, len(nodes))
	limit := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(node model.Node) {
			defer wg.Done()
			out := result{node: CoverCertificateSourceNode{NodeID: node.Id, NodeName: node.Name, State: "unavailable"}}
			defer func() { results <- out }()
			if node.Transitive {
				out.node.State = "direct_required"
				logger.Warningf("Certificate source skipped node_id=%d reason=direct_required", node.Id)
				return
			}
			if node.Scheme != "https" || node.TlsVerifyMode == "skip" {
				out.node.State = "https_required"
				logger.Warningf("Certificate source skipped node_id=%d reason=https_required", node.Id)
				return
			}
			if manager == nil {
				logger.Errorf("Certificate source discovery failed node_id=%d reason=node_manager_unavailable", node.Id)
				return
			}
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				return
			}
			remote, err := manager.RemoteFor(&node)
			if err != nil {
				logger.Errorf("Certificate source connection failed node_id=%d error=%s", node.Id, logger.ErrorSummary(err))
				return
			}
			remoteSites, err := remote.ListLocalCoverCertificateSources(ctx)
			if err != nil {
				out.node.State = coverSourceErrorState(err)
				logger.Errorf("Certificate source discovery failed node_id=%d state=%s error=%s", node.Id, out.node.State, logger.ErrorSummary(err))
				return
			}
			for _, site := range remoteSites {
				domain, err := distributionDomain(site.Domain)
				if err != nil || domain != site.Domain {
					logger.Errorf("Certificate source response rejected node_id=%d reason=invalid_hostname", node.Id)
					continue
				}
				out.sites = append(out.sites, ManagedCoverSiteSummary{
					NodeID: node.Id, Domain: domain,
					SiteCatalog: site.SiteCatalog, SiteTemplate: site.SiteTemplate,
				})
			}
			out.node.State = "empty"
			if len(out.sites) > 0 {
				out.node.State = "ready"
			}
		}(node)
	}
	wg.Wait()
	close(results)
	for out := range results {
		report.Nodes = append(report.Nodes, out.node)
		report.Sites = append(report.Sites, out.sites...)
	}
	sort.Slice(report.Nodes, func(i, j int) bool { return report.Nodes[i].NodeID < report.Nodes[j].NodeID })
	sort.Slice(report.Sites, func(i, j int) bool {
		if report.Sites[i].NodeID != report.Sites[j].NodeID {
			return report.Sites[i].NodeID < report.Sites[j].NodeID
		}
		return report.Sites[i].Domain < report.Sites[j].Domain
	})
	return report, nil
}
