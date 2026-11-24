package controller

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
	"github.com/SawaMEN/3x-ui/v3/internal/web/middleware"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// getRemoteIp extracts the real IP address from the request headers or remote address.
func getRemoteIp(c *gin.Context) string {
	remoteIP, ok := extractTrustedIP(c.Request.RemoteAddr)
	if !ok {
		return "unknown"
	}
	if !isTrustedProxy(remoteIP) {
		return remoteIP
	}

	realIP, hasRealIP := "", false
	if values := c.Request.Header.Values("X-Real-IP"); len(values) == 1 {
		realIP, hasRealIP = extractTrustedIP(values[0])
	}
	forwardedIP, hasForwarded := forwardedForClient(strings.Join(c.Request.Header.Values("X-Forwarded-For"), ","))
	switch {
	case hasRealIP && hasForwarded && realIP != forwardedIP:
		// A proxy rewrites at most one of the two, so the other is the client's
		// own claim; neither may pick the identity the login limiter keys on.
		return remoteIP
	case hasForwarded:
		return forwardedIP
	case hasRealIP:
		return realIP
	}
	return remoteIP
}

func isTrustedForwardedRequest(c *gin.Context) bool {
	remoteIP, ok := extractTrustedIP(c.Request.RemoteAddr)
	return ok && isTrustedProxy(remoteIP)
}

func isTrustedProxy(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return proxyAddrInCIDRs(addr, trustedProxyCIDRs())
}

func proxyAddrInCIDRs(addr netip.Addr, cidrs string) bool {
	addr = addr.Unmap()
	for value := range strings.SplitSeq(cidrs, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(value); err == nil {
			// Handle IPv4-mapped prefixes consistently with unmapped peers.
			if prefix.Addr().Is4In6() && prefix.Bits() >= 96 {
				prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
			}
			if prefix.Contains(addr) {
				return true
			}
			continue
		}
		if proxyIP, err := netip.ParseAddr(value); err == nil && proxyIP.Unmap() == addr {
			return true
		}
	}
	return false
}

func trustedProxyCIDRs() (trusted string) {
	trusted = service.DefaultTrustedProxyCIDRs
	defer func() {
		_ = recover()
	}()
	settingService := service.SettingService{}
	if value, err := settingService.GetTrustedProxyCIDRs(); err == nil && strings.TrimSpace(value) != "" {
		trusted = value
	}
	return trusted
}

func extractTrustedIP(value string) (string, bool) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return "", false
	}

	if ip, ok := parseIPCandidate(candidate); ok {
		return ip.String(), true
	}

	if host, _, err := net.SplitHostPort(candidate); err == nil {
		if ip, ok := parseIPCandidate(host); ok {
			return ip.String(), true
		}
	}

	return "", false
}

func parseIPCandidate(value string) (netip.Addr, bool) {
	ip, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || ip.Zone() != "" {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

// jsonMsg sends a JSON response with a message and error status.
func jsonMsg(c *gin.Context, msg string, err error) {
	jsonMsgObj(c, msg, nil, err)
}

// jsonObj sends a JSON response with an object and error status.
func jsonObj(c *gin.Context, obj any, err error) {
	jsonMsgObj(c, "", obj, err)
}

func requestErrorContext(c *gin.Context) string {
	handler, loc := callerOutsideUtil()
	return fmt.Sprintf("[%s %s handler=%s %s]", c.Request.Method, middleware.RequestRoute(c), handler, loc)
}

func callerOutsideUtil() (string, string) {
	var pcs [12]uintptr
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		base := filepath.Base(frame.File)
		if base != "util.go" {
			name := frame.Function
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			return name, fmt.Sprintf("%s:%d", base, frame.Line)
		}
		if !more {
			break
		}
	}
	return "unknown", "unknown"
}

// jsonMsgObj sends a JSON response with a message, object, and error status.
func jsonMsgObj(c *gin.Context, msg string, obj any, err error) {
	m := entity.Msg{
		Obj: obj,
	}
	if err == nil {
		m.Success = true
		if msg != "" {
			m.Msg = msg
		}
	} else {
		m.Success = false
		middleware.MarkRequestFailureLogged(c)
		logger.Errorf("%s operation failed: %s", requestErrorContext(c), logger.ErrorSummary(err))
		errStr := err.Error()
		// Service validation can return an i18n key. Resolve it before
		// formatting the response, otherwise the UI displays the raw key.
		if strings.HasPrefix(errStr, "pages.") {
			if translated := I18nWeb(c, errStr); translated != "" && translated != errStr {
				errStr = translated
			}
		}
		if errStr != "" && msg != "" {
			m.Msg = msg + " (" + errStr + ")"
		} else if errStr != "" {
			m.Msg = errStr
		} else if msg != "" {
			m.Msg = msg
		} else {
			m.Msg = I18nWeb(c, "somethingWentWrong")
		}
	}
	c.JSON(http.StatusOK, m)
}

// pendingNodeObj returns a response object flagging that the save committed
// locally but a backing node was offline/disabled, so the change will be
// mirrored to the node once it reconnects. Returns nil when nothing is pending.
func pendingNodeObj(pending bool) any {
	if pending {
		return gin.H{"nodePending": true}
	}
	return nil
}

// pureJsonMsg sends a pure JSON message response with custom status code.
func pureJsonMsg(c *gin.Context, statusCode int, success bool, msg string) {
	if !success {
		middleware.MarkRequestFailureLogged(c)
		logger.Errorf("%s operation failed status=%d reason=%s", requestErrorContext(c), statusCode, logger.SanitizeDiagnostic(msg))
	}
	c.JSON(statusCode, entity.Msg{
		Success: success,
		Msg:     msg,
	})
}

// isAjax checks if the request is an AJAX request.
func isAjax(c *gin.Context) bool {
	return c.GetHeader("X-Requested-With") == "XMLHttpRequest"
}

// forwardedForClient walks X-Forwarded-For from the proxy end: entries a client sent
// come first, so the first hop that is not a trusted proxy is the address one saw.
func forwardedForClient(xff string) (string, bool) {
	// Resolve the trust configuration once per chain rather than once per hop.
	cidrs := trustedProxyCIDRs()
	client := ""
	for xff != "" {
		i := strings.LastIndexByte(xff, ',')
		ip, ok := extractTrustedIP(xff[i+1:])
		if !ok {
			break
		}
		client = ip
		addr, err := netip.ParseAddr(ip)
		if err != nil || !proxyAddrInCIDRs(addr, cidrs) || i < 0 {
			break
		}
		xff = xff[:i]
	}
	return client, client != ""
}
