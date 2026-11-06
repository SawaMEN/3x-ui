package controller

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/panel"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
)

var upgrader = ws.Upgrader{
	ReadBufferSize:    32768,
	WriteBufferSize:   32768,
	EnableCompression: true,
	CheckOrigin:       checkSameOrigin,
}

// checkSameOrigin rejects cross-origin WebSocket upgrades before session
// cookies are used. A hostname-only check is unsafe: http://host:8080 and
// https://host:443 are different origins even though the DNS name matches.
// Compare the effective port as well. TLS-terminating reverse proxies can
// leave r.TLS nil, so only enforce the Origin scheme when TLS is observable.
func checkSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Non-browser WebSocket clients do not always send Origin.
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Hostname() == "" ||
		(u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	if r.TLS != nil && u.Scheme != "https" {
		return false
	}
	host := r.Host
	hostPort := ""
	if parsedHost, port, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = parsedHost
		hostPort = port
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if !strings.EqualFold(u.Hostname(), host) {
		return false
	}
	originPort := u.Port()
	if originPort == "" {
		if u.Scheme == "https" {
			originPort = "443"
		} else {
			originPort = "80"
		}
	}
	if hostPort != "" {
		return hostPort == originPort
	}
	// Without an explicit request port, only an implicit or explicit
	// default port is a match. A custom Origin port is a different site.
	return u.Port() == "" || (u.Scheme == "https" && u.Port() == "443") ||
		(u.Scheme == "http" && u.Port() == "80")
}

// WebSocketController handles the HTTP→WebSocket upgrade for real-time updates.
// All per-connection lifecycle (pumps, hub registration) lives in
// panel.WebSocketService — this controller is HTTP-layer only.
type WebSocketController struct {
	BaseController
	service *panel.WebSocketService
}

// NewWebSocketController creates a controller wired to the given service.
func NewWebSocketController(svc *panel.WebSocketService) *WebSocketController {
	return &WebSocketController{service: svc}
}

// HandleWebSocket authenticates the request, upgrades the HTTP connection, and
// hands ownership of the connection off to the service.
func (w *WebSocketController) HandleWebSocket(c *gin.Context) {
	if !session.IsLogin(c) {
		logger.Warningf("Unauthorized WebSocket connection attempt from %s", getRemoteIp(c))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Failed to upgrade WebSocket connection:", err)
		return
	}

	w.service.HandleConnection(conn, getRemoteIp(c))
}
