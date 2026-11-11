package service

import (
    "context"
    "errors"
    "fmt"
    "path/filepath"
    "time"

    "github.com/SawaMEN/3x-ui/v3/internal/database"
    "github.com/SawaMEN/3x-ui/v3/internal/database/model"
    "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

// ResolveLocalTLSCertificate reports paths only for verified, locally usable
// TLS certificate/key pairs. It never reads or exports the private key.
func ResolveLocalTLSCertificate(ctx context.Context, domain string) (DistributedTLSCertificate, error) {
    name, err := distributionDomain(domain)
    if err != nil {
        return DistributedTLSCertificate{}, err
    }
    sites, err := ManagedSNIDomains(ctx, false)
    if err != nil {
        return DistributedTLSCertificate{}, err
    }
    for _, site := range sites {
        if site.Name == name && site.CertFile != "" && site.KeyFile != "" &&
            site.ExpiresAt > time.Now().Unix() &&
            filepath.IsAbs(site.CertFile) && filepath.IsAbs(site.KeyFile) {
            return DistributedTLSCertificate{
                Domain: name, CertFile: site.CertFile, KeyFile: site.KeyFile,
                ExpiresAt: site.ExpiresAt,
            }, nil
        }
    }
    return DistributedTLSCertificate{}, errors.New("no valid local TLS certificate for this SNI")
}

// ResolveTLSCertificate selects only the chosen inbound node. No general
// remote inventory leaks private filesystem paths; an administrator requests
// this one matching pair specifically while editing an inbound.
func ResolveTLSCertificate(ctx context.Context, nodeID int, domain string) (DistributedTLSCertificate, error) {
    name, err := distributionDomain(domain)
    if err != nil {
        return DistributedTLSCertificate{}, err
    }
    if nodeID == 0 {
        return ResolveLocalTLSCertificate(ctx, name)
    }
    if nodeID < 0 || database.GetDB() == nil {
        return DistributedTLSCertificate{}, errors.New("invalid node")
    }
    var node model.Node
    if err := database.GetDB().First(&node, nodeID).Error; err != nil {
        return DistributedTLSCertificate{}, err
    }
    if !node.Enable || node.Transitive {
        return DistributedTLSCertificate{}, errors.New("node is unavailable or read-only")
    }
    manager := runtime.GetManager()
    if manager == nil {
        return DistributedTLSCertificate{}, errors.New("node manager is unavailable")
    }
    remote, err := manager.RemoteFor(&node)
    if err != nil {
        return DistributedTLSCertificate{}, err
    }
    resolved, err := remote.ResolveLocalTLSCertificate(ctx, name)
    if err != nil {
        return DistributedTLSCertificate{}, fmt.Errorf("node certificate lookup failed: %w", err)
    }
    if resolved.Domain != name || resolved.ExpiresAt <= time.Now().Unix() ||
        !filepath.IsAbs(resolved.CertFile) || !filepath.IsAbs(resolved.KeyFile) {
        return DistributedTLSCertificate{}, errors.New("node returned an invalid certificate reference")
    }
    resolved.NodeID = nodeID
    return resolved, nil
}
