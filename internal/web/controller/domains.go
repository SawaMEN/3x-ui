package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type DomainController struct{}

func NewDomainController(g *gin.RouterGroup) {
	a := &DomainController{}
	g = g.Group("/domains")
	g.GET("/status", a.status)
	g.GET("/sni", a.sni)
	g.POST("/apply", a.apply)
	g.POST("/check", a.check)
	g.POST("/recommend", a.recommend)
}

func (a *DomainController) status(c *gin.Context) {
	result, err := service.DomainFrontStatus()
	jsonObj(c, result, err)
}

// The authenticated main panel is the single source of SNI hostnames for
// local and remotely managed inbounds. File paths are local to this panel.
func (a *DomainController) sni(c *gin.Context) {
	result, err := service.ManagedSNIDomains()
	jsonObj(c, result, err)
}

func (a *DomainController) apply(c *gin.Context) {
	cfg := domainfront.DefaultConfig()
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.ApplyDomainFront(c.Request.Context(), cfg)
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
