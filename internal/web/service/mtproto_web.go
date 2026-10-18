package service

import (
	"context"
	"crypto/sha256"
	"net/http"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
)

type mtprotoWebSettings struct {
	Enabled  bool     `json:"enabled"`
	Domain   string   `json:"domain"`
	Frontend string   `json:"frontend"`
	Carrier  string   `json:"carrier"`
	Carriers []string `json:"carriers"`
}

func mtprotoWebForInbound(ib *model.Inbound) (mtprotoWebSettings, error) {
	if ib == nil || ib.Protocol != model.MTProto {
		return mtprotoWebSettings{}, nil
	}
	var settings struct {
		WebProxy mtprotoWebSettings `json:"webProxy"`
	}
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		return mtprotoWebSettings{}, err
	}
	return settings.WebProxy, nil
}

func mtprotoWebCandidates() ([]*model.Inbound, error) {
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("mtproto web: database is unavailable")
	}
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ? AND node_id IS NULL AND enable = ?", model.MTProto, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	selected := make([]*model.Inbound, 0, 1)
	for _, ib := range inbounds {
		opts, err := mtprotoWebForInbound(ib)
		if err != nil {
			return nil, err
		}
		if opts.Enabled {
			selected = append(selected, ib)
		}
	}
	return selected, nil
}

// ValidateMtprotoWebInbound ensures only one local HTTPS frontend owns port 443.
// This is checked before database mutations as well as by periodic reconciliation.
func ValidateMtprotoWebInbound(ib *model.Inbound) error {
	if ib == nil || ib.Protocol != model.MTProto {
		return nil
	}
	opts, err := mtprotoWebForInbound(ib)
	if err != nil || !opts.Enabled {
		return err
	}
	if ib.NodeID != nil {
		return errors.New("MTProto WEB Proxy is currently supported only on the local server")
	}
	if !ib.Enable {
		return nil
	}
	if !telemtWebDomainPattern.MatchString(strings.TrimSpace(opts.Domain)) {
		return errors.New("MTProto WEB Proxy requires a valid public domain")
	}
	if _, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{Frontend: opts.Frontend, Carrier: opts.Carrier, Carriers: opts.Carriers}); err != nil {
		return err
	}
	others, err := mtprotoWebCandidates()
	if err != nil {
		return err
	}
	for _, other := range others {
		if other.Id != ib.Id {
			return fmt.Errorf("MTProto WEB Proxy already belongs to inbound %d; only one HTTPS frontend per server is supported", other.Id)
		}
	}
	return nil
}

var mtprotoWebSyncMu sync.Mutex
var mtprotoWebLastError time.Time

// ReconcileMtprotoWebProxy derives service state from MTProto inbounds.
// Ordinary MTProto sidecars remain managed independently by internal/mtproto.
// Legacy installations with an unowned WEB Proxy are never changed implicitly.
func ReconcileMtprotoWebProxy(ctx context.Context) error {
	if !mtprotoWebSyncMu.TryLock() {
		return nil
	}
	defer mtprotoWebSyncMu.Unlock()
	selected, err := mtprotoWebCandidates()
	if err != nil {
		return err
	}
	if len(selected) > 1 {
		return fmt.Errorf("mtproto web: %d enabled WEB inbounds; only one is supported", len(selected))
	}
	state, err := readTelemtWebState()
	if err != nil {
		return err
	}

	if len(selected) == 0 || !mtprotoWebHasUsers(selected[0]) {
		if state.ManagedInboundID == 0 {
			return nil
		}
		return (TelemtService{}).DisableWebProxy()
	}

	ib := selected[0]
	opts, err := mtprotoWebForInbound(ib)
	if err != nil {
		return err
	}
	domain := normalizeTelemtWebDomain(opts.Domain)
	normalized, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{Frontend: opts.Frontend, Carrier: opts.Carrier, Carriers: opts.Carriers})
	if err != nil {
		return err
	}
	if state.Enabled && state.ManagedInboundID == 0 {
		return errors.New("mtproto web: legacy WEB Proxy is active; migrate or disable it before creating a managed WEB inbound")
	}
	if state.Enabled && state.ManagedInboundID != 0 && state.ManagedInboundID != ib.Id {
		// Ownership moves in two steps: stop the old frontend first, then
		// allow the new inbound to acquire port 443 on the next reconcile.
		return (TelemtService{}).DisableWebProxy()
	}

	fingerprint := mtprotoWebFingerprint(ib)
	unchanged := state.Enabled && state.ManagedInboundID == ib.Id &&
		state.Domain == domain && telemtWebFrontend(state) == normalized.Frontend &&
		telemtWebCarrier(state) == normalized.Carrier &&
		strings.Join(state.Carriers, ",") == strings.Join(normalized.Carriers, ",")
	svc := TelemtService{}
	if unchanged && state.UserFingerprint == fingerprint && svc.WebProxyTelemtActive() {
		return nil
	}
	if unchanged {
		cfg, cfgErr := svc.GetConfig()
		if cfgErr != nil {
			return cfgErr
		}
		if saveErr := svc.SaveConfig(cfg); saveErr != nil {
			return saveErr
		}
		state.UserFingerprint = fingerprint
		if err := writeTelemtWebState(state); err != nil {
			return err
		}
		return svc.EnsureWebProxyBackend()
	}
	// Avoid repeatedly attempting certificate issuance on every periodic job
	// when DNS, ports, or certificate preconditions cannot be satisfied.
	if time.Since(mtprotoWebLastError) < time.Minute {
		return nil
	}
	settings := SettingService{}
	cert, _ := settings.GetCertFile()
	key, _ := settings.GetKeyFile()
	previousState := state
	state.ManagedInboundID = ib.Id
	if state.BackendPort == 0 {
		port, portErr := mtproto.FreeLocalPort()
		if portErr != nil {
			return portErr
		}
		state.BackendPort = port
	}
	state.UserFingerprint = fingerprint
	if err := writeTelemtWebState(state); err != nil {
		return err
	}
	if _, err := svc.EnableWebProxy(ctx, domain, cert, key, normalized); err != nil {
		mtprotoWebLastError = time.Now()
		_ = writeTelemtWebState(previousState)
		return err
	}
	state, err = readTelemtWebState()
	if err != nil {
		return err
	}
	state.ManagedInboundID = ib.Id
	state.UserFingerprint = fingerprint
	if err := writeTelemtWebState(state); err != nil {
		return err
	}
	cfg, err := svc.GetConfig()
	if err != nil {
		return err
	}
	return svc.SaveConfig(cfg)
}

func mtprotoWebHasUsers(ib *model.Inbound) bool {
	inst, ok := mtproto.InstanceFromInbound(ib)
	return ok && len(inst.Secrets) > 0
}

// mtprotoWebUsername is stable across editing or reordering clients. It also
// lets the normal traffic collector resolve the corresponding panel email.
func mtprotoWebUsername(inboundID int, email string) string {
	digest := sha256.Sum256([]byte(email))
	return fmt.Sprintf("mtweb_%d_%x", inboundID, digest[:8])
}

var mtprotoWebTraffic struct {
	sync.Mutex
	inboundID int
	last map[string]int64
}

// CollectMtprotoWebTraffic maps cumulative WEB backend counters back onto the
// usual MTProto clients. Telemt currently reports aggregate octets, not reliable
// direction-specific counters, so they are recorded as Down, just like raw
// MTProto sidecars. A failed poll never advances the previous snapshot.
func CollectMtprotoWebTraffic(ctx context.Context) ([]mtproto.Traffic, []string, string, error) {
	mtprotoWebTraffic.Lock()
	defer mtprotoWebTraffic.Unlock()

	state, err := readTelemtWebState()
	if err != nil { return nil, nil, "", err }
	if !state.Enabled || state.ManagedInboundID == 0 {
		mtprotoWebTraffic.inboundID = 0
		mtprotoWebTraffic.last = nil
		return nil, nil, "", nil
	}

	var inbound model.Inbound
	if err := database.GetDB().First(&inbound, state.ManagedInboundID).Error; err != nil { return nil, nil, "", err }
	inst, ok := mtproto.InstanceFromInbound(&inbound)
	if !ok { return nil, nil, "", nil }

	usersByName := make(map[string]string, len(inst.Secrets))
	for _, sec := range inst.Secrets {
		usersByName[mtprotoWebUsername(inbound.Id, sec.Name)] = sec.Name
	}
	if mtprotoWebTraffic.inboundID != inbound.Id {
		mtprotoWebTraffic.inboundID = inbound.Id
		mtprotoWebTraffic.last = make(map[string]int64)
	}
	if systemctl("is-active", "--quiet", telemtServiceName) != nil { return nil, nil, "", nil }

	client := &http.Client{Timeout: 4 * time.Second, Transport: telemtMonitorTransport{ctx}}
	snapshot, err := fetchTelemtUserStats(client)
	if err != nil { return nil, nil, "", err }

	var deltas []mtproto.Traffic
	var online []string
	for _, user := range snapshot {
		email, wanted := usersByName[user.Username]
		if !wanted { continue }
		if user.CurrentConnections > 0 { online = append(online, email) }
		current := max(user.TotalOctets, 0)
		last, had := mtprotoWebTraffic.last[user.Username]
		if had {
			delta := current - last
			if delta < 0 { delta = current } // Telemt restarted/reset its counter.
			if delta > 0 {
				deltas = append(deltas, mtproto.Traffic{Tag: inbound.Tag, Email: email, Down: delta})
			}
		}
		mtprotoWebTraffic.last[user.Username] = current
	}
	return deltas, online, inbound.Tag, nil
}

func mtprotoWebFingerprint(ib *model.Inbound) string {
	inst, ok := mtproto.InstanceFromInbound(ib)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(inst.Secrets))
	for _, user := range inst.Secrets {
		parts = append(parts, user.Name+":"+user.Secret+":"+fmt.Sprint(user.QuotaBytes)+":"+fmt.Sprint(user.ExpiresUnix)+":"+fmt.Sprint(user.LimitIP))
	}
	fingerprint := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", fingerprint)
}
