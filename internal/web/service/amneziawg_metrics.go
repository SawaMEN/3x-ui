package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"
	"github.com/SawaMEN/3x-ui/v3/internal/amneziawgnet"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// AmneziaWGMetrics exports aggregate metrics to authenticated scrapers. Labels
// never include email, keys, endpoint IPs or tunnel addresses.
func (s *ServerService) AmneziaWGMetrics() (string, error) {
	var inbounds []*model.Inbound
	db := database.GetDB()
	if err := db.Where("protocol = ? AND enable = ? AND node_id IS NULL", model.AmneziaWG, true).Order("id").Find(&inbounds).Error; err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# HELP awg_inbounds_configured Enabled local AWG inbounds.\n# TYPE awg_inbounds_configured gauge\n")
	fmt.Fprintf(&b, "awg_inbounds_configured %d\n", len(inbounds))
	b.WriteString("# HELP awg_inbound_running Embedded AWG inbound is running.\n# TYPE awg_inbound_running gauge\n")
	b.WriteString("# HELP awg_inbound_peers Configured enabled peers before quota filtering.\n# TYPE awg_inbound_peers gauge\n")
	b.WriteString("# HELP awg_inbound_received_bytes Bytes received since device start.\n# TYPE awg_inbound_received_bytes gauge\n")
	b.WriteString("# HELP awg_inbound_sent_bytes Bytes sent since device start.\n# TYPE awg_inbound_sent_bytes gauge\n")
	online := map[string]bool{}
	now := time.Now()
	for _, inbound := range inbounds {
		inst, ok := amneziawg.InstanceFromInbound(inbound)
		if !ok {
			fmt.Fprintf(&b, "awg_inbound_running{inbound_id=\"%d\"} 0\n", inbound.Id)
			continue
		}
		diag := amneziawgnet.Diagnose(inbound.Id, inst.Peers)
		running := 0
		if diag.Running {
			running = 1
		}
		fmt.Fprintf(&b, "awg_inbound_running{inbound_id=\"%d\"} %d\n", inbound.Id, running)
		fmt.Fprintf(&b, "awg_inbound_peers{inbound_id=\"%d\"} %d\n", inbound.Id, len(inst.Peers))
		var rx, tx uint64
		for _, peer := range diag.Clients {
			rx += peer.RxBytes
			tx += peer.TxBytes
			age := now.Sub(peer.LastHandshake)
			if !peer.LastHandshake.IsZero() && age >= 0 && age < amneziawgOnlineWindow {
				online[peer.Email] = true
			}
		}
		fmt.Fprintf(&b, "awg_inbound_received_bytes{inbound_id=\"%d\"} %d\n", inbound.Id, rx)
		fmt.Fprintf(&b, "awg_inbound_sent_bytes{inbound_id=\"%d\"} %d\n", inbound.Id, tx)
	}
	b.WriteString("# HELP awg_current_online Unique peers with a handshake in the last 180 seconds.\n# TYPE awg_current_online gauge\n")
	fmt.Fprintf(&b, "awg_current_online %d\n", len(online))
	b.WriteString("# HELP awg_active_users Users attached to AWG with recorded activity in the rolling window (all protocols).\n# TYPE awg_active_users gauge\n")
	for _, window := range []struct {
		name     string
		duration time.Duration
	}{{"24h", 24 * time.Hour}, {"30d", 30 * 24 * time.Hour}} {
		var count int64
		// last_online belongs to a shared client, so activity from another
		// attached protocol also counts. The metric documents this explicitly.
		err := db.Table("client_traffics AS traffic").Joins("JOIN clients AS client ON client.email = traffic.email").
			Joins("JOIN client_inbounds AS link ON link.client_id = client.id").Joins("JOIN inbounds AS inbound ON inbound.id = link.inbound_id").
			Where("inbound.protocol = ? AND inbound.node_id IS NULL AND traffic.last_online >= ?", model.AmneziaWG, now.Add(-window.duration).UnixMilli()).
			Distinct("client.id").Count(&count).Error
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "awg_active_users{window=\"%s\"} %d\n", window.name, count)
	}
	return b.String(), nil
}
