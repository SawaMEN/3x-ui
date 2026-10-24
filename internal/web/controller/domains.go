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

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type DomainController struct{}

func NewDomainController(g *gin.RouterGroup) {
	a := &DomainController{}
	g = g.Group("/domains")
	g.GET("/status", a.status)
	g.POST("/apply", a.apply)
	g.POST("/check", a.check)
}

func (a *DomainController) status(c *gin.Context) {
	result, err := service.DomainFrontStatus()
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
	for _, d := range cfg.Domains {
		if d.Name == req.Name {
			found = true
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
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: req.Name}}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(req.Name, strconv.Itoa(cfg.PublicPort)))
	if err != nil {
		result.Error = err.Error()
		jsonObj(c, result, nil)
		return
	}
	defer conn.Close()
	cert := conn.(*tls.Conn).ConnectionState().PeerCertificates[0]
	result.ExpiresAt = cert.NotAfter.Unix()
	result.DaysLeft = int(math.Ceil(time.Until(cert.NotAfter).Hours() / 24))
	result.Issuer = cert.Issuer.CommonName
	jsonObj(c, result, nil)
}
