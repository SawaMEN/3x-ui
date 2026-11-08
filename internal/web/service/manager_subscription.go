package service

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func ServeManagerSubscription(w http.ResponseWriter, r *http.Request, token string) {
	if _, err := uuid.Parse(token); err != nil {
		http.NotFound(w, r)
		return
	}
	db := database.GetDB()
	var sub model.ManagerSubscription
	if err := db.Where("token = ? AND enabled = ?", token, true).First(&sub).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var client model.ClientRecord
	var traffic *coremodel.ClientTraffic
	if sub.ClientID > 0 {
		if err := db.First(&client, sub.ClientID).Error; err != nil || !client.Enable || client.ExpiryTime > 0 && client.ExpiryTime < time.Now().UnixMilli() {
			http.NotFound(w, r)
			return
		}
		var err error
		traffic, err = (&InboundService{}).GetClientTrafficByEmail(client.Email)
		if err != nil {
			http.Error(w, "traffic unavailable", http.StatusServiceUnavailable)
			return
		}
		if traffic != nil && client.TotalGB > 0 && (traffic.Up >= client.TotalGB || traffic.Down >= client.TotalGB-traffic.Up) {
			http.NotFound(w, r)
			return
		}
	}
	if sub.ClientID > 0 {
		allowed, err := (&ClientService{}).CheckSubscriptionDevice(w, r, client.SubID)
		if err != nil {
			http.Error(w, "device check unavailable", http.StatusServiceUnavailable)
			return
		}
		if !allowed {
			http.NotFound(w, r)
			return
		}
	}
	provider := subLinkProvider()
	if provider == nil {
		http.Error(w, "renderer unavailable", http.StatusServiceUnavailable)
		return
	}
	format := r.URL.Query().Get("format")
	// A default manager subscription can contain MASQUE alongside standard
	// proxy inbounds. Raw links have no MASQUE representation, so select native
	// JSON before rendering, preserving all published compatible connections.
	if (format == "" || format == "all") && sub.ClientID > 0 {
		var selected []int
		for _, entry := range sub.Published {
			if entry.ActiveInboundID > 0 {
				selected = append(selected, entry.ActiveInboundID)
			}
		}
		if len(selected) > 0 {
			var candidates []model.Inbound
			if err := db.Select("id").Where("id IN ? AND user_id = ? AND enable = ? AND protocol = ?", selected, sub.UserID, true, model.MASQUE).Find(&candidates).Error; err != nil {
				http.Error(w, "subscription lookup unavailable", http.StatusServiceUnavailable)
				return
			}
			for _, inbound := range candidates {
				var count int64
				if err := db.Model(&model.ClientInbound{}).Where("client_id = ? AND inbound_id = ?", client.Id, inbound.Id).Count(&count).Error; err != nil {
					http.Error(w, "subscription lookup unavailable", http.StatusServiceUnavailable)
					return
				}
				if count > 0 {
					format = "singbox"
					if strings.Contains(strings.ToLower(r.UserAgent()), "hiddify") {
						format = "hiddify"
					}
					break
				}
			}
		}
	}
	config := format == "singbox" || format == "hiddify" || format == "xray"
	if format != "" && format != "raw" && format != "base64" && format != "all" && !config {
		http.Error(w, "unknown format", http.StatusBadRequest)
		return
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	out := []string{}
	var configBody, configType string
	if config {
		renderer, ok := provider.(interface {
			ManagerFormat(string, model.ManagerSubscription, model.ClientRecord, string) (string, string, error)
		})
		if !ok {
			http.Error(w, "format unavailable", http.StatusServiceUnavailable)
			return
		}
		var err error
		configBody, configType, err = renderer.ManagerFormat(host, sub, client, format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotAcceptable)
			return
		}
		// When a selected sidecar cannot be encoded in native JSON,
		// render all the published connections as lossless URI links.
		// Never return a partial JSON subscription that hides VetroffProxy.
		if configBody == "" && configType == "" {
			config = false
		}
	}

	for _, e := range sub.Published {
		if config {
			break
		}
		if e.CustomLink != "" {
			out = append(out, e.CustomLink)
			continue
		}
		if e.ActiveInboundID < 1 || sub.ClientID < 1 {
			continue
		}
		var ib model.Inbound
		if err := db.Where("id = ? AND user_id = ? AND enable = ?", e.ActiveInboundID, sub.UserID, true).First(&ib).Error; err != nil {
			continue
		}
		var n int64
		if err := db.Model(&model.ClientInbound{}).Where("client_id = ? AND inbound_id = ?", client.Id, ib.Id).Count(&n).Error; err != nil || n == 0 {
			continue
		}
		if e.RelayID > 0 {
			var relay model.ManagerRelay
			if err := db.Where("id = ? AND user_id = ? AND installed = ?", e.RelayID, sub.UserID, true).First(&relay).Error; err != nil {
				continue
			}
			ib.ShareAddrStrategy, ib.ShareAddr = "custom", relay.Address
		}
		ib.Remark = strings.TrimSpace(e.Flag + " " + e.Label)
		if ib.Remark == "" {
			ib.Remark = sub.Name
		}
		if p, ok := provider.(interface {
			ManagerLinksForClient(string, *model.Inbound, string, string, bool) []string
		}); ok {
			links := p.ManagerLinksForClient(host, &ib, client.Email, ib.Remark, e.RelayID > 0)
			if len(links) == 0 {
				http.Error(w, "A selected protocol requires JSON. Use ?format=singbox or ?format=hiddify", http.StatusNotAcceptable)
				return
			}
			out = append(out, links...)
		} else {
			out = append(out, provider.LinksForClient(host, &ib, client.Email)...)
		}
	}
	for _, p := range []string{sub.Happ, sub.Incy} {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 && configBody == "" {
		http.Error(w, "subscription is not generated", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("profile-title", "base64:"+base64.StdEncoding.EncodeToString([]byte(sub.Name)))
	w.Header().Set("profile-update-interval", "1")
	if sub.Note != "" {
		w.Header().Set("announce", "base64:"+base64.StdEncoding.EncodeToString([]byte(sub.Note)))
	}
	if sub.SupportURL != "" {
		w.Header().Set("support-url", sub.SupportURL)
	}
	if sub.WebsiteURL != "" {
		w.Header().Set("profile-web-page-url", sub.WebsiteURL)
	}
	if traffic != nil {
		w.Header().Set("subscription-userinfo", fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", traffic.Up, traffic.Down, client.TotalGB, max(0, client.ExpiryTime/1000)))
	}
	body := strings.Join(out, "\n")
	if r.URL.Query().Get("format") != "raw" {
		body = base64.StdEncoding.EncodeToString([]byte(body))
	}
	if config {
		body = configBody
		w.Header().Set("Content-Type", configType)
	}
	_, _ = w.Write([]byte(body))
}
