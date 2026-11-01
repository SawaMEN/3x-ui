package job

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"
	"github.com/SawaMEN/3x-ui/v3/internal/amneziawgnet"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// AmneziaWGJob converges embedded AmneziaWG interfaces (inbounds AND the
// template's "amneziawg" outbounds) every 10s. Payload accounting belongs
// to the core SOCKS relay; device diagnostics only supply handshake presence.
type AmneziaWGJob struct {
	inboundService service.InboundService
	settingService service.SettingService
	runMu          sync.Mutex
}

// NewAmneziaWGJob creates a new AmneziaWG reconcile job instance.
func NewAmneziaWGJob() *AmneziaWGJob {
	return &AmneziaWGJob{}
}

const amneziaWGOnlineWindow = 3 * time.Minute

// refreshOnline uses handshakes for peer presence, including idle tunnels.
// Do not add device byte counters: the core already counts TCP/UDP payload
// under the same email and inbound tag, so adding them would charge twice.
func (j *AmneziaWGJob) refreshOnline(coreType string, desired []amneziawg.Instance) {
	j.runMu.Lock()
	defer j.runMu.Unlock()
	if !service.IsNativeCore(coreType) || len(desired) == 0 {
		return
	}
	now := time.Now()
	var activeEmails, activeTags []string
	seenTags := map[string]bool{}
	for _, inst := range desired {
		diag, err := j.inboundService.GetAmneziaWGDiagnostics(inst.Id)
		if err != nil || !diag.Running {
			continue
		}
		for _, client := range diag.Clients {
			if !client.LastHandshake.IsZero() && now.Sub(client.LastHandshake) <= amneziaWGOnlineWindow {
				activeEmails = append(activeEmails, client.Email)
				if !seenTags[inst.Tag] {
					activeTags = append(activeTags, inst.Tag)
					seenTags[inst.Tag] = true
				}
			}
		}
	}
	if len(activeEmails) > 0 {
		if err := j.inboundService.BumpClientsLastOnline(activeEmails); err != nil {
			logger.Warning("amneziawg job: bump last online failed:", err)
		}
	}
	j.inboundService.RefreshLocalOnlineClients(activeEmails, activeTags)
}

// Run reconciles desired AmneziaWG inbounds with running embedded interfaces.
func (j *AmneziaWGJob) Run() {
	desired, err := j.inboundService.DesiredAmneziaWGInstances()
	if err != nil {
		logger.Warning("amneziawg job: get desired instances failed:", err)
		return
	}

	wanted := make([]amneziawgnet.Desired, 0, len(desired))
	for _, inst := range desired {
		wanted = append(wanted, amneziawgnet.Desired{
			Instance: inst,
			Options: amneziawgnet.DeviceOptions{
				HeaderProtectionKey:    inst.Obfuscation.HeaderProtectionKey,
				ContentPaddingAddition: inst.Obfuscation.ContentPaddingAddition,
				RekeyAfterTime:         inst.Obfuscation.RekeyAfterTime,
				RekeyTimeout:           inst.Obfuscation.RekeyTimeout,
				RejectAfterTime:        inst.Obfuscation.RejectAfterTime,
				KeepaliveTimeout:       inst.Obfuscation.KeepaliveTimeout,
				MaxHandshakeAttempts:   inst.Obfuscation.MaxHandshakeAttempts,
				RandomTrailers:         inst.Obfuscation.RandomTrailers,
				DisableCookies:         inst.Obfuscation.DisableCookies,
			},
		})
	}
	amneziawgnet.GetManager().Reconcile(wanted)

	coreType, _ := j.settingService.GetCoreType()
	j.refreshOnline(coreType, desired)

	outboundDesired, err := j.desiredOutboundInstances()
	if err != nil {
		logger.Warning("amneziawg job: get desired outbound instances failed:", err)
		return
	}
	amneziawgnet.GetOutboundManager().Reconcile(outboundDesired)
}

// desiredOutboundInstances derives client instances per template "amneziawg" outbound.
func (j *AmneziaWGJob) desiredOutboundInstances() ([]amneziawgnet.OutboundDesired, error) {
	template, err := j.settingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}
	if template == "" {
		return nil, nil
	}
	cfg := &xray.Config{}
	if err := json.Unmarshal([]byte(template), cfg); err != nil {
		return nil, err
	}
	if len(cfg.OutboundConfigs) == 0 {
		return nil, nil
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(cfg.OutboundConfigs, &raws); err != nil {
		return nil, err
	}
	out := make([]amneziawgnet.OutboundDesired, 0, len(raws))
	for _, raw := range raws {
		if !amneziawg.IsAmneziaWGOutbound(raw) {
			continue
		}
		var probe struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil || probe.Tag == "" {
			continue
		}
		if amneziawgnet.GetOutboundManager().HasNativeTag(probe.Tag) {
			continue
		}
		inst, ok := amneziawg.InstanceFromOutbound(probe.Tag, raw)
		if !ok {
			continue
		}
		out = append(out, amneziawgnet.OutboundDesired{
			Instance: inst,
			Options: amneziawgnet.DeviceOptions{
				HeaderProtectionKey:    inst.Obfuscation.HeaderProtectionKey,
				ContentPaddingAddition: inst.Obfuscation.ContentPaddingAddition,
				RekeyAfterTime:         inst.Obfuscation.RekeyAfterTime,
				RekeyTimeout:           inst.Obfuscation.RekeyTimeout,
				RejectAfterTime:        inst.Obfuscation.RejectAfterTime,
				KeepaliveTimeout:       inst.Obfuscation.KeepaliveTimeout,
				MaxHandshakeAttempts:   inst.Obfuscation.MaxHandshakeAttempts,
				RandomTrailers:         inst.Obfuscation.RandomTrailers,
				DisableCookies:         inst.Obfuscation.DisableCookies,
			},
		})
	}
	return out, nil
}
