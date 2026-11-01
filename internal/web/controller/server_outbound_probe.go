package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/core"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

const (
	defaultOutboundProbeURL = "https://www.gstatic.com/generate_204"
	warpOutboundProbeURL    = "https://www.cloudflare.com/cdn-cgi/trace"
)

// An outbound URL probe is an HTTPS transaction, not an ICMP ping. For WARP
// MASQUE, two different hosts prevent an isolated destination failure from
// being presented as an unavailable tunnel. User-specified URLs aren't retried.
type outboundDelayProbe interface {
	ProxyDelay(context.Context, string, string, time.Duration) (*singbox.ClashProxyDelay, error)
}

func measureOutboundDelay(ctx context.Context, probe outboundDelayProbe, tag, testURL string, timeout time.Duration, warp, customURL bool) (*singbox.ClashProxyDelay, string, error) {
	urls := []string{testURL}
	if warp && !customURL {
		urls = []string{warpOutboundProbeURL, testURL}
	}
	perAttempt := timeout / time.Duration(len(urls))
	if perAttempt < time.Millisecond {
		perAttempt = time.Millisecond
	}
	failures := make([]string, 0, len(urls))
	for _, target := range urls {
		result, err := probe.ProxyDelay(ctx, tag, target, perAttempt)
		if err == nil && result != nil && result.Delay > 0 {
			return result, target, nil
		}
		if err == nil {
			err = fmt.Errorf("ядро не вернуло положительную задержку")
		}
		if len(urls) == 1 {
			return nil, "", err
		}
		failures = append(failures, fmt.Sprintf("%s: %v", target, err))
		if ctx.Err() != nil {
			break
		}
	}
	return nil, "", fmt.Errorf(
		"WARP через MASQUE %q: HTTPS-проверка не прошла (%s). "+
			"Проверьте журнал sing-box на ошибки регистрации Cloudflare и доступность Cloudflare API; "+
			"если выбран HTTP/3 (QUIC/UDP), попробуйте HTTP/2 (TCP)",
		tag, strings.Join(failures, "; "),
	)
}

func (a *ServerController) initOutboundProbeRouter(g *gin.RouterGroup) {
	g.GET("/core/capabilities", a.getCoreCapabilities)
	g.POST("/singbox/outbound/check", a.checkSingBoxOutbound)
}

func (a *ServerController) selectedCore() (core.Type, error) {
	selected, err := a.settingService.GetCoreType()
	if err != nil {
		return "", err
	}
	coreType := core.Type(selected)
	if !coreType.Valid() {
		return "", fmt.Errorf("unsupported core type %q", selected)
	}
	return coreType, nil
}

func (a *ServerController) getCoreCapabilities(c *gin.Context) {
	coreType, err := a.selectedCore()
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	jsonObj(c, gin.H{
		"core":         coreType,
		"capabilities": coreType.Capabilities(),
	}, nil)
}

func (a *ServerController) checkSingBoxOutbound(c *gin.Context) {
	coreType, err := a.selectedCore()
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if !coreType.Capabilities().OutboundDelayProbe {
		jsonObj(c, nil, fmt.Errorf("selected core %q does not support outbound delay probes", coreType))
		return
	}
	if !service.NativeCore(string(coreType)).IsRunning() {
		jsonObj(c, nil, fmt.Errorf("%s is not running", coreType))
		return
	}

	tag := strings.TrimSpace(c.PostForm("tag"))
	if tag == "" {
		jsonObj(c, nil, fmt.Errorf("outbound tag is required"))
		return
	}

	rawURL := strings.TrimSpace(c.PostForm("url"))
	testURL := rawURL
	if testURL == "" {
		testURL = defaultOutboundProbeURL
	}
	warp := singbox.AppliedOutboundType(tag) == "masque"

	timeout := 5 * time.Second
	if warp && rawURL == "" {
		timeout = 10 * time.Second
	}
	if raw := strings.TrimSpace(c.PostForm("timeout")); raw != "" {
		milliseconds, err := strconv.Atoi(raw)
		if err != nil || milliseconds < 1 || milliseconds > 30000 {
			jsonObj(c, nil, fmt.Errorf("timeout must be between 1 and 30000 milliseconds"))
			return
		}
		timeout = time.Duration(milliseconds) * time.Millisecond
	}

	result, measuredURL, err := measureOutboundDelay(
		c.Request.Context(), singbox.NewClashStatsClient(), tag, testURL,
		timeout, warp, rawURL != "",
	)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	jsonObj(c, gin.H{
		"tag":     tag,
		"url":     measuredURL,
		"delay":   result.Delay,
		"delay2":  result.Delay2,
		"timeout": timeout.Milliseconds(),
	}, nil)
}
