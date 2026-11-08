package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

// PublishedTelegramProxies is an authenticated admin inventory, not a public
// subscription. Only local inbounds explicitly allowed on the parent appear.
func (s *InboundService) PublishedTelegramProxies(ctx context.Context, host string) ([]coremodel.TelegramProxyProfile, error) {
	var inbounds []*model.Inbound
	if err := database.GetDB().WithContext(ctx).Where("node_id IS NULL AND protocol = ? AND enable = ?", model.MTProto, true).Order("id ASC").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	result := []coremodel.TelegramProxyProfile{}
	for _, inbound := range inbounds {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		parent, _ := mtproto.Publication(inbound.Settings)
		if !parent {
			continue
		}
		clients, err := s.ListClientsForInbound(inbound.Id)
		if err != nil {
			return nil, err
		}
		provider := subLinkProvider()
		if provider == nil {
			return nil, fmt.Errorf("subscription link provider is unavailable")
		}
		emails := make(map[string]string, len(clients)*2)
		for _, client := range clients {
			if !client.Enable {
				continue
			}
			emails[strings.ToLower(client.Secret)] = client.Email
			if key := telegramClientSecretKey(client.Secret); emails[key] == "" {
				emails[key] = client.Email
			}
		}
		name := strings.TrimSpace(inbound.Remark)
		if name == "" {
			name = inbound.Tag
		}
		for _, link := range provider.LinksForInbounds(host, []*model.Inbound{inbound}) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			profile, ok := coremodel.ParseTelegramProxyLink(link, name)
			if !ok {
				continue
			}
			u, _ := url.Parse(link)
			secret := strings.ToLower(u.Query().Get("secret"))
			email := emails[secret]
			if email == "" {
				email = emails[telegramClientSecretKey(secret)]
			}
			if email == "" {
				continue
			}
			profile.Name += " · " + email
			result = append(result, profile)
		}
	}
	return result, nil
}

func RemoteTelegramProxies(parent context.Context) ([]coremodel.RemoteTelegramProxy, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	var nodes []*model.Node
	if err := database.GetDB().WithContext(ctx).Where("enable = ?", true).Order("sort_order ASC, id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	results := make([][]coremodel.TelegramProxyProfile, len(nodes))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(8, len(nodes)) {
		workers.Go(func() {
			for index := range jobs {
				if ctx.Err() != nil {
					continue
				}
				remote := runtime.NewRemote(nodes[index], nil)
				if manager := runtime.GetManager(); manager != nil {
					var err error
					remote, err = manager.RemoteFor(nodes[index])
					if err != nil {
						continue
					}
				}
				profiles, err := remote.FetchPublishedTelegramProxies(ctx)
				if err == nil {
					results[index] = profiles
				}
			}
		})
	}
	for index := range nodes {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	result := []coremodel.RemoteTelegramProxy{}
	for index, profiles := range results {
		for _, profile := range profiles {
			if clean, ok := coremodel.ParseTelegramProxyLink(profile.Link, profile.Name); ok {
				result = append(result, coremodel.RemoteTelegramProxy{NodeID: nodes[index].Id, NodeName: nodes[index].Name, Profile: clean})
			}
		}
	}
	return result, nil
}

func telegramClientSecretKey(secret string) string {
	secret = strings.ToLower(strings.TrimSpace(secret))
	if len(secret) >= 34 && (strings.HasPrefix(secret, "ee") || strings.HasPrefix(secret, "dd")) {
		return secret[2:34]
	}
	return secret
}
