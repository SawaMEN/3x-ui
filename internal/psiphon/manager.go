package psiphon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type Instance struct {
	Id                  int
	Tag                 string
	ServerAddress       string
	Protocol            string
	Port                int
	ServerEntry         string
	AdditionalArguments []string
}

func InstanceFromInbound(ib *model.Inbound) (Instance, bool) {
	if ib == nil || ib.Protocol != model.Psiphon {
		return Instance{}, false
	}

	var raw map[string]any
	if json.Unmarshal([]byte(ib.Settings), &raw) != nil {
		return Instance{}, false
	}

	addr, _ := raw["serverAddress"].(string)
	addr = strings.TrimSpace(addr)

	proto, _ := raw["tunnelProtocol"].(string)
	proto = strings.ToUpper(strings.TrimSpace(proto))
	if proto == "" {
		proto = "OSSH"
	}
	switch proto {
	case "OSSH", "SSH", "TLS-OSSH", "QUIC-OSSH":
	default:
		return Instance{}, false
	}

	entry, _ := raw["serverEntry"].(string)
	entry = strings.TrimSpace(entry)

	args := []string{}
	if a, ok := raw["additionalArguments"].([]any); ok {
		for _, v := range a {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				args = append(args, s)
			}
		}
	}

	// server-entry.dat is client-facing Psiphon metadata, not the server's
	// private runtime configuration. A reusable entry therefore never replaces
	// the need for an advertised server address when generating psiphond.conf.
	if addr == "" {
		addr = detectPublicAddress()
	}

	if ib.Port < 1 || ib.Port > 65535 || addr == "" {
		return Instance{}, false
	}

	return Instance{
		Id:                  ib.Id,
		Tag:                 ib.Tag,
		ServerAddress:       addr,
		Protocol:            proto,
		Port:                ib.Port,
		ServerEntry:         entry,
		AdditionalArguments: args,
	}, true
}

func detectPublicAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	var publicIPv6 string

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ip := parseInterfaceIP(addr)
			if ip == nil || !ip.IsGlobalUnicast() {
				continue
			}

			if ip.To4() != nil {
				if !ip.IsPrivate() {
					return ip.String()
				}
				continue
			}

			if !ip.IsPrivate() && publicIPv6 == "" {
				publicIPv6 = ip.String()
			}
		}
	}

	// Prefer IPv4 for broad VPS/client compatibility, but keep a public IPv6
	// fallback for IPv6-only servers.
	return publicIPv6
}

func parseInterfaceIP(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		host, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			return nil
		}
		return net.ParseIP(host)
	}
}

func (inst Instance) fingerprint() string {
	// ServerEntry is generated client metadata and must not cause a psiphond
	// restart when it changes or is refreshed in the database.
	return fmt.Sprintf("%s|%s|%d|%s", inst.ServerAddress, inst.Protocol, inst.Port, strings.Join(inst.AdditionalArguments, "\x00"))
}

type managed struct {
	proc *Process
	tag  string
	fp   string
}

type Manager struct {
	mu      sync.Mutex
	procs   map[int]*managed
	lastErr map[int]string
}

var (
	once      sync.Once
	singleton *Manager
)

func GetManager() *Manager {
	once.Do(func() {
		singleton = &Manager{procs: map[int]*managed{}, lastErr: map[int]string{}}
	})
	return singleton
}

func configDir() string {
	return filepath.Join(config.GetBinFolderPath(), "psiphon")
}

func configPathForID(id int) string {
	return filepath.Join(configDir(), fmt.Sprintf("inbound-%d", id))
}

func persistServerEntry(id int, entry string) {
	if strings.TrimSpace(entry) == "" {
		return
	}
	db := database.GetDB()
	if db == nil {
		return
	}

	var ib model.Inbound
	if db.Where("id = ?", id).First(&ib).Error != nil {
		return
	}

	var raw map[string]any
	if json.Unmarshal([]byte(ib.Settings), &raw) != nil {
		return
	}
	raw["serverEntry"] = entry
	b, err := json.Marshal(raw)
	if err != nil {
		return
	}
	_ = db.Model(&ib).Update("settings", string(b)).Error
}

func (m *Manager) ensureLocked(inst Instance) error {
	fp := inst.fingerprint()
	if cur := m.procs[inst.Id]; cur != nil && cur.proc != nil && cur.proc.IsRunning() && cur.fp == fp {
		cur.tag = inst.Tag
		return nil
	}

	if cur := m.procs[inst.Id]; cur != nil {
		_ = cur.proc.Stop()
		delete(m.procs, inst.Id)
	}
	_ = os.RemoveAll(configPathForID(inst.Id))

	if err := os.MkdirAll(configPathForID(inst.Id), 0o750); err != nil {
		return err
	}

	proc := newProcess(configPathForID(inst.Id), inst.Tag, inst)
	if err := proc.Start(); err != nil {
		return err
	}

	if entry := proc.ServerEntry(); entry != "" {
		// Keep the DB copy synchronized with the actual generated server config.
		persistServerEntry(inst.Id, entry)
	}

	m.procs[inst.Id] = &managed{proc: proc, tag: inst.Tag, fp: fp}
	delete(m.lastErr, inst.Id)
	logger.Infof("psiphon: started psiphond for inbound %d (%s)", inst.Id, inst.Tag)
	return nil
}

func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureLocked(inst)
}

func (m *Manager) Reconcile(desired []Instance) {
	m.mu.Lock()
	defer m.mu.Unlock()

	want := map[int]Instance{}
	for _, i := range desired {
		want[i.Id] = i
	}

	for id, cur := range m.procs {
		if _, ok := want[id]; !ok {
			_ = cur.proc.Stop()
			delete(m.procs, id)
			_ = os.RemoveAll(configPathForID(id))
		}
	}

	for _, i := range desired {
		if err := m.ensureLocked(i); err != nil && m.lastErr[i.Id] != err.Error() {
			m.lastErr[i.Id] = err.Error()
			logger.Warningf("psiphon: failed to start inbound %d (%s): %v", i.Id, i.Tag, err)
		}
	}
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cur := m.procs[id]; cur != nil {
		_ = cur.proc.Stop()
		delete(m.procs, id)
	}
	_ = os.RemoveAll(configPathForID(id))
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, cur := range m.procs {
		_ = cur.proc.Stop()
		_ = os.RemoveAll(configPathForID(id))
		delete(m.procs, id)
	}
}

func (m *Manager) HasRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cur := range m.procs {
		if cur.proc != nil && cur.proc.IsRunning() {
			return true
		}
	}
	return false
}
