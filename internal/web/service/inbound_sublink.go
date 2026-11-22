package service

import (
	"errors"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
)

type SubLinkProvider interface {
	SubLinksForSubId(host, subId string) ([]string, error)
	LinksForClient(host string, inbound *model.Inbound, email string) []string
	LinksForInbounds(host string, inbounds []*model.Inbound) []string
}

func RegisterSubLinkProvider(p SubLinkProvider) {
	subLinkProviderMu.Lock()
	defer subLinkProviderMu.Unlock()
	registeredSubLinkProvider = p
}

func (s *InboundService) GetSubLinks(host, subId string) ([]string, error) {
	provider := subLinkProvider()
	if provider == nil {
		return nil, common.NewError("sub link provider not registered")
	}
	return provider.SubLinksForSubId(host, subId)
}

func (s *InboundService) GetAllInboundLinks(host string, userId int) ([]string, error) {
	provider := subLinkProvider()
	if provider == nil {
		return nil, common.NewError("sub link provider not registered")
	}
	inbounds, err := s.GetInbounds(userId)
	if err != nil {
		return nil, err
	}
	return provider.LinksForInbounds(host, inbounds), nil
}

func (s *InboundService) GetAllClientLinks(host string, email string) ([]string, error) {
	if email == "" {
		return nil, common.NewError("client email is required")
	}
	provider := subLinkProvider()
	if provider == nil {
		return nil, common.NewError("sub link provider not registered")
	}
	rec, err := s.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return nil, err
	}
	inboundIds, err := s.clientService.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return nil, err
	}
	var links []string
	for _, ibId := range inboundIds {
		inbound, getErr := s.GetInbound(ibId)
		if getErr != nil {
			return nil, getErr
		}
		links = append(links, provider.LinksForClient(host, inbound, email)...)
	}
	return links, nil
}

// GetScopedInboundLinks uses the subscription renderer so server certificate
// pins, node addresses and endpoint overrides match the actual subscription.
func (s *InboundService) GetScopedInboundLinks(host string, inbound *model.Inbound, email string) ([]string, error) {
	provider := subLinkProvider()
	if provider == nil {
		return nil, common.NewError("sub link provider not registered")
	}
	if email == "" {
		return nil, common.NewError("client email is required")
	}
	links := provider.LinksForClient(host, inbound, email)
	if len(links) == 0 {
		return nil, errNoConnectionLinks
	}
	return links, nil
}

type nodeRouteLinkProvider interface {
	NodeRouteLinks(host string, inbound *model.Inbound, email string) ([]string, error)
}

func (s *InboundService) getNodeRouteLinks(host string, inbound *model.Inbound, email string) ([]string, error) {
	provider := subLinkProvider()
	routeProvider, ok := provider.(nodeRouteLinkProvider)
	if !ok {
		return s.GetScopedInboundLinks(host, inbound, email)
	}
	links, err := routeProvider.NodeRouteLinks(host, inbound, email)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, errNoConnectionLinks
	}
	return links, nil
}

var errNoConnectionLinks = errors.New("no connection links available; check client, certificate and public endpoint")

var (
	subLinkProviderMu         sync.RWMutex
	registeredSubLinkProvider SubLinkProvider
)

// subLinkProvider guards the registry: startup writes it while link requests read it.
func subLinkProvider() SubLinkProvider {
	subLinkProviderMu.RLock()
	defer subLinkProviderMu.RUnlock()
	return registeredSubLinkProvider
}
