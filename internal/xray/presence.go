package xray

import (
	"encoding/json"
	"errors"
	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"os"
	"sync"
)

// Presence tracks local and remote online identities; it cannot launch a core.
type Presence struct {
	onlineClients, localActiveInbounds      []string
	localLastOnline, localInboundLastActive map[string]int64
	nodeOnlineTrees, nodeActiveInboundTrees map[int]map[string][]string
	onlineMu                                sync.RWMutex
}

func NewPresence() *Presence { return &Presence{} }
func GetConfigPath() string {
	return config.GetBinFolderPath() + "/config.json"
}
func GetGeositePath() string {
	return config.GetBinFolderPath() + "/geosite.dat"
}
func GetGeoipPath() string {
	return config.GetBinFolderPath() + "/geoip.dat"
}
func GetIPLimitLogPath() string {
	return config.GetLogFolder() + "/3xipl.log"
}
func GetIPLimitBannedLogPath() string {
	return config.GetLogFolder() + "/3xipl-banned.log"
}
func GetIPLimitBannedPrevLogPath() string {
	return config.GetLogFolder() + "/3xipl-banned.prev.log"
}
func getLogPath(key string) (string, error) {
	config, err := os.ReadFile(GetConfigPath())
	if err != nil {
		// config.json is generated when Xray starts. It may legitimately be
		// absent after an install/update and before the first successful start.
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		logger.Warningf("Failed to read configuration file: %s", err)
		return "", err
	}

	jsonConfig := map[string]any{}
	err = json.Unmarshal(config, &jsonConfig)
	if err != nil {
		logger.Warningf("Failed to parse JSON configuration: %s", err)
		return "", err
	}

	if jsonLog, ok := jsonConfig["log"].(map[string]any); ok {
		if logPath, ok := jsonLog[key].(string); ok {
			return logPath, nil
		}
	}
	return "", nil
}
func GetAccessLogPath() (string, error) {
	return getLogPath("access")
}
func GetErrorLogPath() (string, error) {
	return getLogPath("error")
}
func (p *Presence) GetOnlineClients() []string {
	p.onlineMu.RLock()
	defer p.onlineMu.RUnlock()

	if len(p.nodeOnlineTrees) == 0 {
		// Hot path for single-panel deployments: avoid the map+dedupe
		// work entirely and return the local slice as-is.
		return p.onlineClients
	}

	seen := make(map[string]struct{}, len(p.onlineClients))
	out := make([]string, 0, len(p.onlineClients))
	add := func(emails []string) {
		for _, email := range emails {
			if _, dup := seen[email]; dup {
				continue
			}
			seen[email] = struct{}{}
			out = append(out, email)
		}
	}
	add(p.onlineClients)
	for _, tree := range p.nodeOnlineTrees {
		for _, emails := range tree {
			add(emails)
		}
	}
	return out
}
func (p *Presence) GetLocalOnlineClients() []string {
	p.onlineMu.RLock()
	defer p.onlineMu.RUnlock()
	if len(p.onlineClients) == 0 {
		return nil
	}
	out := make([]string, len(p.onlineClients))
	copy(out, p.onlineClients)
	return out
}
func (p *Presence) GetMergedNodeTrees() map[string][]string {
	p.onlineMu.RLock()
	defer p.onlineMu.RUnlock()
	if len(p.nodeOnlineTrees) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string)
	seen := make(map[string]map[string]struct{})
	for _, tree := range p.nodeOnlineTrees {
		for guid, emails := range tree {
			if guid == "" || len(emails) == 0 {
				continue
			}
			dedup := seen[guid]
			if dedup == nil {
				dedup = make(map[string]struct{}, len(emails))
				seen[guid] = dedup
			}
			for _, email := range emails {
				if _, ok := dedup[email]; ok {
					continue
				}
				dedup[email] = struct{}{}
				out[guid] = append(out[guid], email)
			}
		}
	}
	return out
}
func (p *Presence) GetLocalActiveInbounds() []string {
	p.onlineMu.RLock()
	defer p.onlineMu.RUnlock()
	if len(p.localActiveInbounds) == 0 {
		return nil
	}
	out := make([]string, len(p.localActiveInbounds))
	copy(out, p.localActiveInbounds)
	return out
}
func (p *Presence) GetMergedActiveInboundTrees() map[string][]string {
	p.onlineMu.RLock()
	defer p.onlineMu.RUnlock()
	if len(p.nodeActiveInboundTrees) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string)
	seen := make(map[string]map[string]struct{})
	for _, tree := range p.nodeActiveInboundTrees {
		for guid, tags := range tree {
			if guid == "" || len(tags) == 0 {
				continue
			}
			dedup := seen[guid]
			if dedup == nil {
				dedup = make(map[string]struct{}, len(tags))
				seen[guid] = dedup
			}
			for _, tag := range tags {
				if tag == "" {
					continue
				}
				if _, ok := dedup[tag]; ok {
					continue
				}
				dedup[tag] = struct{}{}
				out[guid] = append(out[guid], tag)
			}
		}
	}
	return out
}
func (p *Presence) RefreshLocalOnline(activeEmails, activeInboundTags []string, now, graceMs int64) {
	p.onlineMu.Lock()
	defer p.onlineMu.Unlock()
	if p.localLastOnline == nil {
		p.localLastOnline = make(map[string]int64, len(activeEmails))
	}
	for _, email := range activeEmails {
		p.localLastOnline[email] = now
	}
	online := make([]string, 0, len(p.localLastOnline))
	for email, ts := range p.localLastOnline {
		if now-ts < graceMs {
			online = append(online, email)
		} else {
			delete(p.localLastOnline, email)
		}
	}
	p.onlineClients = online

	if p.localInboundLastActive == nil {
		p.localInboundLastActive = make(map[string]int64, len(activeInboundTags))
	}
	for _, tag := range activeInboundTags {
		p.localInboundLastActive[tag] = now
	}
	activeInbounds := make([]string, 0, len(p.localInboundLastActive))
	for tag, ts := range p.localInboundLastActive {
		if now-ts < graceMs {
			activeInbounds = append(activeInbounds, tag)
		} else {
			delete(p.localInboundLastActive, tag)
		}
	}
	p.localActiveInbounds = activeInbounds
}
func (p *Presence) SetNodeOnlineTree(nodeID int, tree map[string][]string) {
	p.onlineMu.Lock()
	defer p.onlineMu.Unlock()
	if len(tree) == 0 {
		delete(p.nodeOnlineTrees, nodeID)
		return
	}
	if p.nodeOnlineTrees == nil {
		p.nodeOnlineTrees = map[int]map[string][]string{}
	}
	p.nodeOnlineTrees[nodeID] = tree
}
func (p *Presence) SetNodeActiveInboundTree(nodeID int, tree map[string][]string) {
	p.onlineMu.Lock()
	defer p.onlineMu.Unlock()
	if len(tree) == 0 {
		delete(p.nodeActiveInboundTrees, nodeID)
		return
	}
	if p.nodeActiveInboundTrees == nil {
		p.nodeActiveInboundTrees = map[int]map[string][]string{}
	}
	p.nodeActiveInboundTrees[nodeID] = tree
}
func (p *Presence) ClearNodeOnlineClients(nodeID int) {
	p.onlineMu.Lock()
	defer p.onlineMu.Unlock()
	delete(p.nodeOnlineTrees, nodeID)
	delete(p.nodeActiveInboundTrees, nodeID)
}
func (p *Presence) RetainNodeOnlineClients(keep func(nodeID int) bool) {
	p.onlineMu.Lock()
	defer p.onlineMu.Unlock()
	for nodeID := range p.nodeOnlineTrees {
		if !keep(nodeID) {
			delete(p.nodeOnlineTrees, nodeID)
		}
	}
	for nodeID := range p.nodeActiveInboundTrees {
		if !keep(nodeID) {
			delete(p.nodeActiveInboundTrees, nodeID)
		}
	}
}
