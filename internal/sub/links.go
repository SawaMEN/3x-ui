package sub

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type LinkProvider struct {
	settingService service.SettingService
}

func NewLinkProvider() *LinkProvider {
	return &LinkProvider{}
}

func (p *LinkProvider) build(host string) *SubService {
	remarkTemplate, _ := p.settingService.GetRemarkTemplate()
	svc := NewSubService(remarkTemplate)
	svc.PrepareForRequest(host)
	return svc
}

func (p *LinkProvider) SubLinksForSubId(host, subId string) ([]string, error) {
	svc := p.build(host)
	links, _, _, _, err := svc.GetSubs(subId, host)
	if err != nil {
		return nil, err
	}
	return normalizeGeneratedLinks(links), nil
}

func (p *LinkProvider) LinksForClient(host string, inbound *model.Inbound, email string) []string {
	if !sudokuInboundUsable(inbound) {
		return nil
	}
	svc := p.build(host)
	if !svc.refreshSudokuCredentials(inbound) {
		return nil
	}
	svc.projectThroughFallbackMaster(inbound)
	if client, ok := svc.clientForLink(inbound, email); ok {
		endpoints, bound := svc.clientHostEndpoints(inbound, client, "raw", svc.hostEndpoints(inbound, "raw"))
		if bound || endpoints != nil {
			return splitLinkLines(svc.linkFromHosts(inbound, client, endpoints))
		}
	}
	return splitLinkLines(svc.GetLink(inbound, email))
}

func (p *LinkProvider) LinksForInbounds(host string, inbounds []*model.Inbound) []string {
	svc := p.build(host)
	var raw []string
	for _, inbound := range inbounds {
		if !sudokuInboundUsable(inbound) {
			continue
		}
		if !svc.refreshSudokuCredentials(inbound) {
			continue
		}
		raw = append(raw, svc.inboundLinks(inbound)...)
	}
	return normalizeGeneratedLinks(raw)
}

// normalizeGeneratedLinks flattens multi-link entries and removes exact
// duplicates while preserving their first-seen order. This is shared by the
// raw subscription endpoint and link-export APIs so clients see the same set.
func normalizeGeneratedLinks(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, entry := range raw {
		for _, link := range splitLinkLines(entry) {
			if _, duplicate := seen[link]; duplicate {
				continue
			}
			seen[link] = struct{}{}
			out = append(out, link)
		}
	}
	return out
}

type mieruLinkSettings struct {
	ShareLinkFormat string `json:"shareLinkFormat"`
	Clients         []struct {
		Email string `json:"email"`
	} `json:"clients"`
}

// mieruShareLinkFormat resolves the format for the generated native/simple
// pair from the Mieru inbound that owns the link user. Client e-mail is unique
// across panel records, so it is a stable identity even when client data was
// normalized into the clients table. Missing/unknown settings intentionally
// fall back to the interoperable Hiddify simple-sharing format.
func mieruShareLinkFormat(simpleLink string) string {
	u, err := url.Parse(simpleLink)
	if err != nil || u.User == nil {
		return "hiddify"
	}
	email := strings.TrimSpace(u.User.Username())
	if email == "" {
		return "hiddify"
	}

	db := database.GetDB()
	if db == nil {
		return "hiddify"
	}
	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", model.Mieru).Find(&inbounds).Error; err != nil {
		return "hiddify"
	}
	for i := range inbounds {
		var settings mieruLinkSettings
		if json.Unmarshal([]byte(inbounds[i].Settings), &settings) != nil {
			continue
		}
		for _, client := range settings.Clients {
			if client.Email != email {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(settings.ShareLinkFormat), "native") {
				return "native"
			}
			return "hiddify"
		}
	}
	return "hiddify"
}

// splitLinkLines normalizes one generated multi-link entry into the lines that
// can be placed in a generic multi-protocol subscription.
//
// Mieru is a special case. The generator keeps both the native full-profile
// mieru:// form and the interoperable simple-sharing mierus:// form internally.
// They describe the same endpoint. Generic subscription parsers such as
// Hiddify/ray2sing treat both schemes as simple URLs, so only one representation
// must leave the panel. Existing inbounds default to mierus://; an inbound with
// shareLinkFormat=native keeps the protobuf mieru:// representation instead.
func splitLinkLines(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	hasSimpleMieru := false
	hasNativeMieru := false
	simpleMieru := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "mierus://"):
			hasSimpleMieru = true
			if simpleMieru == "" {
				simpleMieru = part
			}
		case strings.HasPrefix(part, "mieru://"):
			hasNativeMieru = true
		}
	}
	preferNativeMieru := hasSimpleMieru && hasNativeMieru && mieruShareLinkFormat(simpleMieru) == "native"

	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if hasSimpleMieru && hasNativeMieru {
			if preferNativeMieru && strings.HasPrefix(p, "mierus://") {
				continue
			}
			if !preferNativeMieru && strings.HasPrefix(p, "mieru://") {
				continue
			}
		}
		if _, duplicate := seen[p]; duplicate {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// ManagerLinksForClient applies per-entry names and relay endpoints through the native renderer.
func (p *LinkProvider) ManagerLinksForClient(host string, ib *model.Inbound, email, label string, relay bool) []string {
	var links []string
	if relay {
		svc := p.build(host)
		copyInbound := *ib
		svc.projectThroughFallbackMaster(&copyInbound)
		copyInbound.ShareAddrStrategy, copyInbound.ShareAddr = "custom", ib.ShareAddr
		links = splitLinkLines(svc.GetLink(&copyInbound, email))
	} else {
		links = p.LinksForClient(host, ib, email)
	}
	for i, raw := range links {
		links[i] = applyRemarkToLink(raw, label)
	}
	return links
}

// ManagerFormat renders only the published selection and never adds other attachments of the same client.
func (p *LinkProvider) ManagerFormat(host string, sub model.ManagerSubscription, client model.ClientRecord, format string) (string, string, error) {
	svc := p.build(host)
	svc.managerInbounds = []*model.Inbound{}
	svc.managerExternal = []externalLinkEntry{}
	db := database.GetDB()
	for _, entry := range sub.Published {
		if entry.CustomLink != "" {
			svc.managerExternal = append(svc.managerExternal, externalLinkEntry{Kind: model.ExternalLinkKindLink, Value: entry.CustomLink, Remark: entry.Label, Email: client.Email, Enable: true, Active: true})
			continue
		}
		var ib model.Inbound
		if err := db.Where("id = ? AND user_id = ? AND enable = ?", entry.ActiveInboundID, sub.UserID, true).First(&ib).Error; err != nil {
			return "", "", err
		}
		var n int64
		if err := db.Model(&model.ClientInbound{}).Where("client_id = ? AND inbound_id = ?", client.Id, ib.Id).Count(&n).Error; err != nil {
			return "", "", err
		}
		if n == 0 {
			return "", "", fmt.Errorf("client no longer attached to selected inbound")
		}
		ib.Remark = strings.TrimSpace(entry.Flag + " " + entry.Label)
		if ib.Remark == "" {
			ib.Remark = sub.Name
		}
		if entry.RelayID > 0 {
			var relay model.ManagerRelay
			if err := db.Where("id = ? AND user_id = ? AND installed = ?", entry.RelayID, sub.UserID, true).First(&relay).Error; err != nil {
				return "", "", err
			}
			ib.ShareAddrStrategy, ib.ShareAddr = "custom", relay.Address
		}
		svc.managerInbounds = append(svc.managerInbounds, &ib)
	}
	renderer := NewSubJsonService("", "", "", "", svc)
	var body string
	var err error
	switch format {
	case "singbox":
		body, _, err = renderer.GetSingBoxJson(client.SubID, host, false)
	case "hiddify":
		body, _, err = renderer.GetHiddifySingBoxJson(client.SubID, host)
	case "xray":
		body, _, err = renderer.GetJson(client.SubID, host, true)
	default:
		return "", "", fmt.Errorf("unknown format")
	}
	if err != nil {
		return "", "", fmt.Errorf("selected protocols are incompatible with %s: %w", format, errSubscriptionFormatUnsupported)
	}
	return body, "application/json; charset=utf-8", nil
}
