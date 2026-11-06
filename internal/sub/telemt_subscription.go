package sub

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type (
	telemtSubscriptionProfile = coremodel.TelegramProxyProfile
	telemtSubscriptionPayload = coremodel.TelegramSubscription
)

func registerTelemtSubscriptionRoute(g *gin.RouterGroup) {
	// Keep the endpoint next to the configured browser subscription URL so it
	// continues to work when /sub/ is customized or exposed through a reverse
	// proxy that forwards the subscription prefix.
	settings := service.SettingService{}
	subPath, err := settings.GetSubPath()
	if err != nil {
		subPath = "/sub/"
	}
	path := "/" + strings.Trim(subPath, "/") + "/telemt/:subid"
	if strings.Trim(subPath, "/") == "" {
		path = "/telemt/:subid"
	}
	g.GET(path, serveTelemtSubscription)
}

func serveTelemtSubscription(c *gin.Context) {
	subID := strings.TrimSpace(c.Param("subid"))
	if subID == "" {
		c.Status(http.StatusNotFound)
		return
	}

	resolver := NewSubService("")
	_, host, _, _ := resolver.ResolveRequest(c)
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	// Validate the subscription through the same data path used by the normal
	// raw/browser subscription instead of depending on a particular DB schema.
	// This also rejects deleted or never-existing subIds consistently.
	subReq := resolver.ForRequest(host)
	subReq.subscriptionBody = false
	subs, emails, _, _, err := subReq.getSubs(subID)
	if err != nil || (subs == nil && len(emails) == 0) {
		writeSubError(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	settings := service.SettingService{}
	featureEnabled, err := settings.GetTelemtSubscriptionProxyEnable()
	if err != nil {
		logger.Warning("sub: failed to read Telemt subscription proxy setting:", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	visibility, err := settings.GetTelegramPageVisibility()
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	remoteProfiles := make(chan []telemtSubscriptionPayload, 1)
	go func() {
		if visibility.Remote {
			remoteProfiles <- remoteTelegramSubscriptions(ctx, subID)
		} else {
			remoteProfiles <- nil
		}
	}()
	telemt := service.TelemtService{}
	payload := telemtSubscriptionPayload{}
	if visibility.Local && featureEnabled && telemt.IsEnabled() {
		// Existing subscriptions may predate eager Telemt provisioning; ensure the
		// local profile idempotently without starting a stopped service.
		if personal, err := telemt.EnsureSubscriptionProxyContext(ctx, subID, host); err == nil {
			payload.Personal = &telemtSubscriptionProfile{
				Host: personal.Host,
				Port: personal.Port,
				TLS:  personal.TLS,
				Link: personal.Link,
			}
		} else {
			logger.Debug("sub: Telemt personal profile unavailable:", err)
		}

		if link, err := telemt.SubscriptionWebProxyLink(); err == nil && link != "" {
			payload.WebProxyEnabled = true
			payload.WebProxy = link
		}
	}
	seen := make(map[string]bool)
	add := func(link, name string) {
		profile, ok := telegramProfileFromLink(link, name)
		if ok && (visibility.Web || !strings.HasPrefix(profile.Link, "tg://webproxy")) && !seen[profile.Link] {
			seen[profile.Link] = true
			payload.Proxies = append(payload.Proxies, profile)
		}
	}
	if payload.Personal != nil {
		add(payload.Personal.Link, "")
	}
	add(payload.WebProxy, "Web Proxy")
	for _, group := range subs {
		for _, link := range splitLinkLines(group) {
			add(link, "")
		}
	}
	for _, remote := range <-remoteProfiles {
		for _, profile := range remote.Proxies {
			add(profile.Link, profile.Name)
		}
	}
	payload.Enabled = len(payload.Proxies) > 0
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, payload)
}

func telegramProfileFromLink(link, name string) (telemtSubscriptionProfile, bool) {
	return coremodel.ParseTelegramProxyLink(link, name)
}

// Node requests share one deadline and a small worker pool; an offline node
// cannot prevent available proxies from appearing or delay the main page.
func remoteTelegramSubscriptions(parent context.Context, subID string) []telemtSubscriptionPayload {
	var nodes []*model.Node
	if err := database.GetDB().Where("enable = ?", true).Order("sort_order ASC, id ASC").Find(&nodes).Error; err != nil {
		logger.Warning("sub: failed to load Telegram proxy nodes")
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	results := make([]telemtSubscriptionPayload, len(nodes))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(8, len(nodes)) {
		workers.Go(func() {
			for index := range jobs {
				if ctx.Err() != nil {
					continue
				}
				node := nodes[index]
				var resolver runtime.NodeEgressResolver
				if manager := runtime.GetManager(); manager != nil {
					resolver = manager
				}
				remote := runtime.NewRemote(node, resolver)
				profile, err := remote.FetchTelegramSubscription(ctx, subID)
				if err != nil || !profile.Enabled {
					continue
				}
				if profile.Personal != nil {
					results[index].Proxies = append(results[index].Proxies, telemtSubscriptionProfile{Name: node.Name, Link: profile.Personal.Link})
				}
				if profile.WebProxyEnabled && profile.WebProxy != "" {
					results[index].Proxies = append(results[index].Proxies, telemtSubscriptionProfile{Name: node.Name + " · Web Proxy", Link: profile.WebProxy})
				}
			}
		})
	}
	for i := range nodes {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	return results
}
