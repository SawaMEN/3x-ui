package tuic

import (
	"fmt"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type managed struct {
	server       *Server
	tag          string
	structuralFP string
	usersFP      string
}

type Manager struct {
<<<<<<< HEAD
	mu           sync.Mutex
	procs        map[int]*managed
	lastStartErr map[int]string
	// pending holds the final counters drained from sidecars that were stopped
	// by Reconcile/StopAll. The next traffic snapshot consumes them together
	// with counters from still-running instances.
	pending TrafficSnapshot
=======
	mu             sync.Mutex
	servers        map[int]*managed
	lastStartErr   map[int]string
	pendingTraffic map[string]ClientTrafficDelta
>>>>>>> 3985ba46a19406eec1a890e1842588d1956c5a10
}

var (
	managerInstance *Manager
	managerOnce     sync.Once
)

func GetManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{
			servers:        make(map[int]*managed),
			lastStartErr:   make(map[int]string),
			pendingTraffic: make(map[string]ClientTrafficDelta),
		}
	})
	return managerInstance
}

func (m *Manager) HasRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mg := range m.servers {
		if mg.server != nil && mg.server.IsRunning() {
			return true
		}
	}
	return false
}

func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureLocked(inst)
}

func (m *Manager) ensureLocked(inst Instance) error {
	if err := ValidateClients(inst.Clients); err != nil {
		return err
	}
	if len(inst.Clients) == 0 {
		m.removeLocked(inst.Id)
		return nil
	}

	structuralFP := inst.StructuralFingerprint()
	usersFP := inst.UsersFingerprint()

	if existing, ok := m.servers[inst.Id]; ok && existing != nil {
		if existing.server != nil && existing.server.IsRunning() && existing.structuralFP == structuralFP {
			existing.tag = inst.Tag
			existing.server.UpdateRuntimeSettings(inst.Tag, inst.CongestionControl, inst.LogLevel)
			if existing.usersFP != usersFP {
				existing.usersFP = usersFP
				existing.server.UpdateUsers(inst.Clients)
			}
			return nil
		}
<<<<<<< HEAD
		// Quiesce the old sidecar first, then drain its last relay counters before
		// closing the sockets. Use the new tag for a same-ID restart so a tag edit
		// cannot strand the final inbound delta under a stale database key.
		m.stopManagedAndCaptureLocked(existing, inst.Tag)
		delete(m.procs, inst.Id)
=======
		m.stopAndDrainLocked(existing)
		delete(m.servers, inst.Id)
>>>>>>> 3985ba46a19406eec1a890e1842588d1956c5a10
	}

	server, err := m.startLocked(inst)
	if err != nil {
		if m.lastStartErr[inst.Id] != err.Error() {
			m.lastStartErr[inst.Id] = err.Error()
			if tuicLogWarn >= parseLogLevel(inst.LogLevel) {
				logger.Warningf("tuic: inbound %d (%s): failed to start server: %v", inst.Id, inst.Tag, err)
			}
		}
		return err
	}
	delete(m.lastStartErr, inst.Id)

	m.servers[inst.Id] = &managed{
		server:       server,
		tag:          inst.Tag,
		structuralFP: structuralFP,
		usersFP:      usersFP,
	}
	return nil
}

func (m *Manager) startLocked(inst Instance) (*Server, error) {
	relay := &SocksRelay{
		Addr:     fmt.Sprintf("127.0.0.1:%d", SOCKSPortForInbound(inst.Id)),
		Password: SocksPassword(),
	}
	server, err := NewServer(inst, relay)
	if err != nil {
		return nil, fmt.Errorf("tuic: init server for %d: %w", inst.Id, err)
	}
	if err := server.Start(); err != nil {
		return nil, fmt.Errorf("tuic: start server on %s for %d: %w", inst.BindTo(), inst.Id, err)
	}
<<<<<<< HEAD
	configPath, err := WriteConfigFile(inst.Id, configBytes)
	if err != nil {
		return nil, nil, "", fmt.Errorf("tuic: write config for %d: %w", inst.Id, err)
	}
	relay, err := startUDPRelay(inst.BindTo(), upstream, relayFlowIdle)
	if err != nil {
		_ = RemoveConfigFile(inst.Id)
		return nil, nil, "", fmt.Errorf("tuic: listen on %s for %d: %w", inst.BindTo(), inst.Id, err)
	}
	proc := newProcess(configPath, inst.Tag, uuidToEmail, relay.bindPeer)
	if err := proc.Start(); err != nil {
		relay.Close()
		_ = RemoveConfigFile(inst.Id)
		return nil, nil, "", err
	}
	return proc, relay, configPath, nil
}

func (m *Manager) stopManagedAndCaptureLocked(mg *managed, tag string) {
	if mg == nil {
		return
	}
	if mg.proc != nil && mg.proc.IsRunning() {
		_ = mg.proc.Stop()
	}
	if mg.relay == nil {
		return
	}

	up, down := mg.relay.CollectTraffic()
	if up > 0 || down > 0 {
		m.pending.Inbounds = append(m.pending.Inbounds, InboundTrafficDelta{
			Tag:  tag,
			Up:   up,
			Down: down,
		})
	}
	for _, d := range mg.relay.CollectClientTraffic() {
		m.pending.Clients = append(m.pending.Clients, ClientTrafficDelta{
			Tag:   tag,
			Email: d.Email,
			Up:    d.Up,
			Down:  d.Down,
		})
	}
	mg.relay.Close()
=======
	return server, nil
}

func (m *Manager) stopAndDrainLocked(mg *managed) {
	if mg == nil || mg.server == nil {
		return
	}
	_ = mg.server.Close()
	m.appendPendingTrafficLocked(mg.server.CollectClientTraffic())
}

func (m *Manager) appendPendingTrafficLocked(deltas []ClientTrafficDelta) {
	if m.pendingTraffic == nil {
		m.pendingTraffic = make(map[string]ClientTrafficDelta)
	}
	for _, delta := range deltas {
		key := delta.Email
		if delta.TrafficID > 0 {
			key = fmt.Sprintf("traffic:%d", delta.TrafficID)
		}
		if delta.TrafficID == 0 && delta.InboundID > 0 && delta.UUID != "" {
			key = fmt.Sprintf("%d:%s", delta.InboundID, delta.UUID)
		}
		current := m.pendingTraffic[key]
		current.Email = delta.Email
		current.UUID = delta.UUID
		current.InboundID = delta.InboundID
		current.TrafficID = delta.TrafficID
		current.Up += delta.Up
		current.Down += delta.Down
		m.pendingTraffic[key] = current
	}
}

func (m *Manager) RequeueClientTraffic(deltas []ClientTrafficDelta) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendPendingTrafficLocked(deltas)
>>>>>>> 3985ba46a19406eec1a890e1842588d1956c5a10
}

func (m *Manager) GetActiveClients(window time.Duration) ([]string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var emails []string
	var tags []string
	for _, mg := range m.servers {
		if mg.server != nil && mg.server.IsRunning() {
			active := mg.server.GetActiveEmails(window)
			if len(active) > 0 {
				emails = append(emails, active...)
				tags = append(tags, mg.tag)
			}
		}
	}
	return emails, tags
}

type InboundTrafficDelta struct {
	Tag  string
	Up   int64
	Down int64
}

<<<<<<< HEAD
type ClientTrafficDelta struct {
	Tag   string
	Email string
	Up    int64
	Down  int64
}

type TrafficSnapshot struct {
	Inbounds []InboundTrafficDelta
	Clients  []ClientTrafficDelta
}

// CollectTrafficSnapshot atomically samples both the aggregate relay counters
// and the per-email counters attributed from authenticated tuic-server peers.
// The two counter sets have independent cursors: an unbound flow is retained on
// the per-client side until authentication identifies it, while the inbound
// aggregate can still be committed every poll. Final counters drained during a
// sidecar restart/removal are returned first from m.pending.
func (m *Manager) CollectTrafficSnapshot() TrafficSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.pending
	m.pending = TrafficSnapshot{}
	for _, mg := range m.procs {
		if mg.relay == nil || mg.proc == nil || !mg.proc.IsRunning() {
			continue
		}
		deltaUp, deltaDown := mg.relay.CollectTraffic()
		if deltaUp > 0 || deltaDown > 0 {
			out.Inbounds = append(out.Inbounds, InboundTrafficDelta{
				Tag:  mg.tag,
				Up:   deltaUp,
				Down: deltaDown,
			})
		}
		for _, d := range mg.relay.CollectClientTraffic() {
			out.Clients = append(out.Clients, ClientTrafficDelta{
				Tag:   mg.tag,
				Email: d.Email,
				Up:    d.Up,
				Down:  d.Down,
			})
		}
	}
	return out
}

// CollectTraffic is kept for package/API compatibility with callers that only
// need inbound totals. It intentionally leaves pending/current per-client
// cursors untouched.
func (m *Manager) CollectTraffic() []InboundTrafficDelta {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.pending.Inbounds
	m.pending.Inbounds = nil
	for _, mg := range m.procs {
		if mg.relay != nil && mg.proc != nil && mg.proc.IsRunning() {
			deltaUp, deltaDown := mg.relay.CollectTraffic()
			if deltaUp > 0 || deltaDown > 0 {
				out = append(out, InboundTrafficDelta{
=======
func (m *Manager) CollectClientTraffic() []ClientTrafficDelta {
	_, clients := m.CollectAllTraffic()
	return clients
}

func (m *Manager) CollectAllTraffic() ([]InboundTrafficDelta, []ClientTrafficDelta) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var inbounds []InboundTrafficDelta
	clients := make([]ClientTrafficDelta, 0, len(m.pendingTraffic))
	for email, delta := range m.pendingTraffic {
		clients = append(clients, delta)
		delete(m.pendingTraffic, email)
	}

	for _, mg := range m.servers {
		if mg.server != nil && mg.server.IsRunning() {
			up, down, cDeltas := mg.server.CollectAllTraffic()
			if up > 0 || down > 0 {
				inbounds = append(inbounds, InboundTrafficDelta{
>>>>>>> 3985ba46a19406eec1a890e1842588d1956c5a10
					Tag:  mg.tag,
					Up:   up,
					Down: down,
				})
			}
			clients = append(clients, cDeltas...)
		}
	}
	return inbounds, clients
}

func (m *Manager) AddTestTraffic(id int, email string, up, down int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mg, ok := m.servers[id]; ok && mg.server != nil {
		return mg.server.AddTestTraffic(email, up, down)
	}
	return false
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(id)
}

func (m *Manager) removeLocked(id int) {
<<<<<<< HEAD
	if existing, ok := m.procs[id]; ok && existing != nil {
		m.stopManagedAndCaptureLocked(existing, existing.tag)
		_ = RemoveConfigFile(id)
		delete(m.procs, id)
=======
	if existing, ok := m.servers[id]; ok && existing != nil {
		m.stopAndDrainLocked(existing)
		delete(m.servers, id)
>>>>>>> 3985ba46a19406eec1a890e1842588d1956c5a10
		delete(m.lastStartErr, id)
	}
}

func (m *Manager) Reconcile(desired []Instance) {
	m.mu.Lock()
	defer m.mu.Unlock()

	desiredMap := make(map[int]Instance, len(desired))
	for _, inst := range desired {
		desiredMap[inst.Id] = inst
	}

	for id := range m.servers {
		if _, ok := desiredMap[id]; !ok {
			m.removeLocked(id)
		}
	}

	for _, inst := range desired {
		_ = m.ensureLocked(inst)
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
<<<<<<< HEAD
	for id, mg := range m.procs {
		m.stopManagedAndCaptureLocked(mg, mg.tag)
		_ = RemoveConfigFile(id)
=======
	for _, mg := range m.servers {
		m.stopAndDrainLocked(mg)
>>>>>>> 3985ba46a19406eec1a890e1842588d1956c5a10
	}
	m.servers = make(map[int]*managed)
}
