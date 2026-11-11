package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
	"github.com/SawaMEN/3x-ui/v3/internal/util/wirecodec"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type DomainController struct{}

func nodeDomainDiscovery(c *gin.Context) bool {
	scope := c.GetString("api_token_scope")
	return c.GetBool("api_authed") && (scope == model.ApiScopeNodeSync || scope == model.ApiScopeNodeAdmin)
}

// Node-to-node bearer authentication is sufficient for read-only discovery,
// but not for changing a gateway's DNS tokens, certificates, or listener config.
// Those high-impact actions require an interactive panel session.
func requireDomainAdminSession(c *gin.Context) bool {
	if c.GetBool("api_authed") {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false, "msg": "Domain mutations require an interactive administrator session",
		})
		return false
	}
	return true
}

func NewDomainController(g *gin.RouterGroup) {
	a := &DomainController{}
	g = g.Group("/domains")
	g.GET("/status", a.status)
	g.GET("/certificates", a.certificates)
	g.GET("/cover-sites", a.coverSites)
	g.GET("/cover-sites/local", a.localCoverSites)
	g.POST("/cover-sites/change-template", a.changeCoverSiteTemplate)
	g.POST("/cover-sites/template", a.nodeCoverSiteTemplate)
	g.GET("/certificates/events", a.certificateEvents)
	g.POST("/certificates/distribute", a.distributeCertificate)
	g.POST("/certificates/resolve", a.resolveCertificate)
	g.POST("/certificates/resolve-local", a.resolveLocalCertificate)
	g.POST("/certificates/install", a.installCertificate)
	g.GET("/dns/credential", a.dnsCredential)
	g.POST("/dns/credential", a.saveDNSCredential)
	g.POST("/certificates/renew", a.renewCertificate)
	g.GET("/sni", a.sni)
	g.POST("/apply", a.apply)
	g.POST("/preflight", a.preflight)
	g.POST("/probe/http01", a.probeHTTP01)
	g.POST("/check", a.check)
	g.POST("/recommend", a.recommend)
}

func requireNodeCoverSitePush(c *gin.Context) bool {
    if nodeDomainDiscovery(c) && c.GetHeader(wirecodec.MasterPushHeader) == "1" {
        return true
    }
    c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
        "success": false, "msg": "cover-site node API requires an authenticated master push",
    })
    return false
}

func (a *DomainController) localCoverSites(c *gin.Context) {
    if !requireNodeCoverSitePush(c) { return }
    c.Header("Cache-Control", "no-store")
    sites, err := service.LocalManagedCoverSites()
    jsonObj(c, sites, err)
}

func (a *DomainController) changeCoverSiteTemplate(c *gin.Context) {
    if !requireDomainAdminSession(c) { return }
    c.Header("Cache-Control", "no-store")
    c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
    var req service.ManagedCoverSiteChange
    if err := c.ShouldBindJSON(&req); err != nil { jsonObj(c, nil, err); return }
    result, err := service.ChangeManagedCoverSiteTemplate(c.Request.Context(), req)
    jsonObj(c, result, err)
}

func (a *DomainController) nodeCoverSiteTemplate(c *gin.Context) {
    if !requireNodeCoverSitePush(c) { return }
    c.Header("Cache-Control", "no-store")
    c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
    var req service.ManagedCoverSiteChange
    if err := c.ShouldBindJSON(&req); err != nil { jsonObj(c, nil, err); return }
    if req.NodeID != 0 {
        jsonObj(c, nil, fmt.Errorf("node may update only its own SNI cover sites"))
        return
    }
    result, err := service.ChangeLocalCoverSiteTemplate(c.Request.Context(), req)
    jsonObj(c, result, err)
}

// coverSites exposes only names and built-in templates for editable owned
// HTTPS cover sites. A remote node's private gateway config is never returned.
func (a *DomainController) coverSites(c *gin.Context) {
    if !requireDomainAdminSession(c) { return }
    c.Header("Cache-Control", "no-store")
    sites, err := service.ManagedCoverSites(c.Request.Context())
    jsonObj(c, sites, err)
}

// resolveCertificate is a read-only, administrator-specific lookup for the
// active inbound node. The browser receives local file paths, never PEM keys.
func (a *DomainController) resolveCertificate(c *gin.Context) {
    if !requireDomainAdminSession(c) { return }
    c.Header("Cache-Control", "no-store")
    c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
    var req struct {
        NodeID int `json:"nodeId"`
        Domain string `json:"domain" binding:"required"`
        RSAOnly bool `json:"rsaOnly,omitempty"`
    }
    if err := c.ShouldBindJSON(&req); err != nil { jsonObj(c, nil, err); return }
    result, err := service.ResolveTLSCertificate(c.Request.Context(), req.NodeID, req.Domain, req.RSAOnly)
    jsonObj(c, result, err)
}

// resolveLocalCertificate serves only a node's already validated paths to the
// authenticated master, not an arbitrary path supplied in the request.
func (a *DomainController) resolveLocalCertificate(c *gin.Context) {
    scope := c.GetString("api_token_scope")
    if !c.GetBool("api_authed") ||
        (scope != model.ApiScopeNodeSync && scope != model.ApiScopeNodeAdmin) ||
        c.GetHeader(wirecodec.MasterPushHeader) != "1" {
        c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
            "success": false, "msg": "node certificate lookup requires an authenticated master push",
        })
        return
    }
    c.Header("Cache-Control", "no-store")
    c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
    var req struct {
        Domain string `json:"domain" binding:"required"`
        RSAOnly bool `json:"rsaOnly,omitempty"`
    }
    if err := c.ShouldBindJSON(&req); err != nil { jsonObj(c, nil, err); return }
    result, err := service.ResolveLocalTLSCertificate(c.Request.Context(), req.Domain, req.RSAOnly)
    jsonObj(c, result, err)
}

// distributeCertificate requires an interactive administrator. Private keys
// never appear in its input or output: the server reads only local managed
// certificates and sends them to the explicitly selected node.
func (a *DomainController) distributeCertificate(c *gin.Context) {
	if !requireDomainAdminSession(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var request struct {
		NodeID int    `json:"nodeId" binding:"required"`
		Domain string `json:"domain" binding:"required"`
		RSAOnly bool `json:"rsaOnly,omitempty"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.DistributeTLSCertificate(c.Request.Context(), request.NodeID, request.Domain, request.RSAOnly)
	jsonObj(c, result, err)
}

// installCertificate is a narrow node-sync operation. Unlike arbitrary file
// uploads, it accepts no destination path and validates the TLS identity
// before writing to the panel's private certificate store.
func (a *DomainController) installCertificate(c *gin.Context) {
	scope := c.GetString("api_token_scope")
	if !c.GetBool("api_authed") ||
		(scope != model.ApiScopeNodeSync && scope != model.ApiScopeNodeAdmin) ||
		c.GetHeader(wirecodec.MasterPushHeader) != "1" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false, "msg": "certificate installation requires an authenticated node push",
		})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 160<<10)
	var request runtime.TLSCertificateInstall
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.InstallDistributedTLSCertificate(request)
	jsonObj(c, result, err)
}

// DNS API tokens are write-only. Authenticated clients can inspect only
// whether the server has a token; there is never a "read token" endpoint.
func (a *DomainController) dnsCredential(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	result, err := service.DomainDNSCredentialStateFor(c.DefaultQuery("provider", "cloudflare"))
	jsonObj(c, result, err)
}

func (a *DomainController) saveDNSCredential(c *gin.Context) {
	if !requireDomainAdminSession(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req struct {
		Provider string `json:"provider"`
		Token    string `json:"token"`
		Clear    bool   `json:"clear"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonObj(c, nil, err)
		return
	}
	if req.Clear && req.Token != "" {
		jsonObj(c, nil, fmt.Errorf("clear and token cannot be supplied together"))
		return
	}
	if req.Provider == "" {
		req.Provider = "cloudflare"
	}
	if err := service.SaveDomainDNSCredentialFor(req.Provider, req.Token, req.Clear); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.DomainDNSCredentialStateFor(req.Provider)
	jsonObj(c, result, err)
}

func (a *DomainController) status(c *gin.Context) {
	result, err := service.DomainFrontStatus()
	jsonObj(c, result, err)
}

// Renew exactly one already registered local certificate, never a caller-supplied path.
func (a *DomainController) renewCertificate(c *gin.Context) {
	if !requireDomainAdminSession(c) {
		return
	}
	var request struct {
		ID string `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.RenewDomainCertificate(c.Request.Context(), request.ID)
	jsonObj(c, result, err)
}

func (a *DomainController) certificateEvents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	result, err := service.CertificateAuditHistory()
	jsonObj(c, result, err)
}

func (a *DomainController) certificates(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if nodeDomainDiscovery(c) || c.Query("summary") == "1" {
		items, err := service.LocalDomainCertificateSummaries()
		jsonObj(c, gin.H{"items": items}, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	result, err := service.DomainCertificateInventoryScoped(ctx, c.Query("local") != "1")
	jsonObj(c, result, err)
}

// The authenticated main panel is the single source of SNI hostnames for
// local and remotely managed inbounds. File paths belong to each site's
// owning node and are not transferable between hosts.
func (a *DomainController) sni(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	nodeDiscovery := nodeDomainDiscovery(c)
	result, err := service.ManagedSNIDomains(c.Request.Context(), !nodeDiscovery && c.Query("local") != "1")
	if nodeDiscovery || c.Query("summary") == "1" {
		// Node discovery never needs absolute paths. Keep private paths on
		// the owning host even when its parent has a valid API token.
		for i := range result {
			result[i].CertFile = ""
			result[i].KeyFile = ""
		}
	}
	jsonObj(c, result, err)
}

func (a *DomainController) apply(c *gin.Context) {
	if !requireDomainAdminSession(c) {
		return
	}
	cfg := domainfront.DefaultConfig()
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.ApplyDomainFront(c.Request.Context(), cfg)
	jsonObj(c, result, err)
}

// preflight inspects a draft configuration without persisting it or issuing certificates.
func (a *DomainController) preflight(c *gin.Context) {
	cfg := domainfront.DefaultConfig()
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.PreflightDomainFront(c.Request.Context(), cfg)
	jsonObj(c, result, err)
}

// This endpoint is authenticated just like other node APIs. It never
// accepts arbitrary ports or paths and does not mutate gateway state.
func (a *DomainController) probeHTTP01(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var input struct {
		Name  string `json:"name" binding:"required"`
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		jsonObj(c, nil, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := service.ObservePublicHTTP01(c.Request.Context(), input.Name, input.Token)
	jsonObj(c, result, err)
}

func (a *DomainController) check(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonObj(c, nil, err)
		return
	}
	cfg, err := (&service.SettingService{}).GetDomainFrontConfig()
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	found := false
	port := cfg.PublicPort
	for _, d := range cfg.Domains {
		if d.Name == req.Name {
			found = true
			if d.Mode == "shared_sni" {
				port = 443
			}
		}
	}
	if !found {
		jsonObj(c, nil, fmt.Errorf("save this domain before checking DNS/TLS"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	result := domainfront.DomainFrontCheck{Name: req.Name, Addresses: []string{}, CheckedAt: time.Now().Unix()}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, req.Name)
	if err != nil {
		result.Error = err.Error()
		jsonObj(c, result, nil)
		return
	}
	for _, ip := range ips {
		result.Addresses = append(result.Addresses, ip.IP.String())
	}
	// Resolve and dial the *validated public IP*, not the hostname again:
	// prevent DNS rebinding to private panel endpoints during a TLS check.
	conn, err := netsafe.SSRFGuardedDialContext(ctx, "tcp", net.JoinHostPort(req.Name, strconv.Itoa(port)))
	if err != nil {
		result.Error = err.Error()
		jsonObj(c, result, nil)
		return
	}
	secure := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: req.Name})
	defer secure.Close()
	if err := secure.HandshakeContext(ctx); err != nil {
		result.Error = err.Error()
		jsonObj(c, result, nil)
		return
	}
	certs := secure.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		result.Error = "remote site did not present a certificate"
		jsonObj(c, result, nil)
		return
	}
	cert := certs[0]
	result.ExpiresAt = cert.NotAfter.Unix()
	result.DaysLeft = int(math.Ceil(time.Until(cert.NotAfter).Hours() / 24))
	result.Issuer = cert.Issuer.CommonName
	jsonObj(c, result, nil)
}

// recommend searches websites near the panel's public IP with the existing
// nearby-SNI discovery method. No input URL is accepted and no settings change.
func (a *DomainController) recommend(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 65*time.Second)
	defer cancel()
	result, err := service.ScanRedirectSites(ctx)
	jsonObj(c, result, err)
}
