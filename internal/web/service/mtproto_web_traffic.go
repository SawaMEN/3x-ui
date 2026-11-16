package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

type mtprotoWebTrafficBinding struct {
	tag   string
	users map[string]string
	owner *sidecartraffic.Owner
}

// The binding belongs to installed runtime configuration. Hold this lock through
// the source poll, WAL acceptance and activation so old responses cannot cross it.
type mtprotoWebTrafficTracker struct {
	sync.Mutex
	binding *mtprotoWebTrafficBinding
	last    map[string]int64
}

func captureMtprotoWebTrafficBinding(state TelemtWebProxyState, rendered string) (*mtprotoWebTrafficBinding, error) {
	if !state.Enabled || state.ManagedInboundID == 0 {
		return nil, nil
	}
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("mtproto WEB traffic: database unavailable")
	}
	var cfg struct {
		Access struct {
			Users map[string]string `toml:"users"`
		} `toml:"access"`
	}
	if err := toml.Unmarshal([]byte(rendered), &cfg); err != nil {
		return nil, fmt.Errorf("mtproto WEB traffic: decode installed users: %w", err)
	}
	var inbound model.Inbound
	if err := db.First(&inbound, state.ManagedInboundID).Error; err != nil {
		return nil, err
	}
	inst, ok := mtproto.InstanceFromInbound(&inbound)
	if !ok {
		return nil, errors.New("mtproto WEB traffic: managed users unavailable")
	}
	b := &mtprotoWebTrafficBinding{tag: inbound.Tag, users: map[string]string{}}
	var emails []string
	for _, user := range inst.Secrets {
		name := mtprotoWebUsername(inbound.Id, user.Name)
		if _, installed := cfg.Access.Users[name]; installed {
			b.users[name] = user.Name
			emails = append(emails, user.Name)
		}
	}
	var err error
	b.owner, err = sidecartraffic.Capture("mtproto", inbound.Id, emails, false)
	if err != nil {
		return nil, err
	}
	if b.owner == nil {
		return nil, errors.New("mtproto WEB traffic: managed owner unavailable")
	}
	return b, nil
}

func fetchMtprotoWebTraffic(ctx context.Context) ([]telemtUserStats, error) {
	return fetchTelemtUserStats(&http.Client{Timeout: 4 * time.Second, Transport: telemtMonitorTransport{ctx}})
}

func (t *mtprotoWebTrafficTracker) consume(snapshot []telemtUserStats, required []string) ([]string, bool, error) {
	b := t.binding
	seen := map[string]bool{}
	for _, user := range snapshot {
		if email, ok := b.users[user.Username]; ok {
			seen[email] = true
		}
	}
	for _, email := range required {
		if !seen[email] {
			return nil, false, errors.New("mtproto WEB traffic: incomplete ownership boundary")
		}
	}
	last := maps.Clone(t.last)
	if last == nil {
		last = map[string]int64{}
	}
	var samples []sidecartraffic.Sample
	var online []string
	for _, user := range snapshot {
		email, wanted := b.users[user.Username]
		if !wanted {
			continue
		}
		if user.CurrentConnections > 0 {
			online = append(online, email)
		}
		current := max(user.TotalOctets, 0)
		if previous, had := t.last[user.Username]; had {
			delta := current - previous
			if delta < 0 {
				delta = current
			}
			samples = append(samples, sidecartraffic.Sample{Email: email, Down: delta, Active: user.CurrentConnections > 0})
		}
		last[user.Username] = current
	}
	accepted, err := b.owner.Store(samples)
	if accepted {
		t.last = last
	}
	return online, accepted, err
}

func (t *mtprotoWebTrafficTracker) poll(ctx context.Context, bootstrap func() (*mtprotoWebTrafficBinding, error), active func() bool, fetch func() ([]telemtUserStats, error)) ([]string, string, error) {
	t.Lock()
	defer t.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if !active() {
		return nil, "", nil
	}
	if t.binding == nil {
		b, err := bootstrap()
		if err != nil {
			return nil, "", err
		}
		if b == nil {
			return nil, "", nil
		}
		t.binding = b
		t.last = nil
	}
	snapshot, err := fetch()
	if err != nil {
		return nil, t.binding.tag, err
	}
	if err := ctx.Err(); err != nil {
		return nil, t.binding.tag, err
	}
	online, _, err := t.consume(snapshot, nil)
	return online, t.binding.tag, err
}

// activate reports an actual restart (including successful rollback restart),
// or a source known to be stopped, so its next counter starts at zero.
func (t *mtprotoWebTrafficTracker) transition(ctx context.Context, next *mtprotoWebTrafficBinding, mutation, active bool, fetch func() ([]telemtUserStats, error), activate func() (bool, error)) error {
	t.Lock()
	defer t.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.binding == nil && next == nil {
		_, err := activate()
		return err
	}
	changed := t.binding == nil || next == nil
	if !changed {
		changed = !t.binding.owner.SameIdentities(next.owner) || !maps.Equal(t.binding.users, next.users)
	}
	var warm []telemtUserStats
	var uncertain map[string]bool
	if t.binding != nil && (changed || mutation) {
		var required []string
		if next != nil && changed {
			before, after := map[string]bool{}, map[string]bool{}
			for _, email := range t.binding.users {
				before[email] = true
			}
			for _, email := range next.users {
				after[email] = true
			}
			for _, email := range t.binding.owner.ChangedClients(next.owner) {
				if before[email] && after[email] {
					required = append(required, email)
				}
			}
		}
		snapshot, err := fetch()
		accepted := false
		if err == nil {
			_, accepted, err = t.consume(snapshot, required)
		}
		if err != nil {
			if len(required) > 0 && !accepted {
				if !mutation {
					return err
				}
				uncertain = map[string]bool{}
				for _, email := range required {
					uncertain[email] = true
				}
			}
			// Revocation cannot wait for metrics; accepted disk failures stay queued.
			logger.Warning("mtproto WEB traffic: configuration boundary:", err)
		}
	} else if t.binding == nil && active {
		var err error
		warm, err = fetch()
		if err != nil {
			if !mutation {
				return err
			}
			logger.Warning("mtproto WEB traffic: initial configuration boundary:", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reset, err := activate()
	if err != nil {
		if reset && t.binding != nil {
			t.last = webTrafficZeroBaseline(t.binding)
		}
		return err
	}
	if !changed && !mutation && !reset {
		t.binding = next
		return nil
	}
	previous, previousBinding := t.last, t.binding
	t.binding = next
	if next == nil {
		t.last = nil
		return nil
	}
	t.last = webTrafficZeroBaseline(next)
	if !reset {
		// Missing old counters remain unknown, never implicit zero/history charges.
		t.last = map[string]int64{}
		for name := range next.users {
			if value, ok := previous[name]; ok {
				t.last[name] = value
			} else if previousBinding != nil {
				if _, existed := previousBinding.users[name]; !existed {
					t.last[name] = 0
				}
			} else if !active {
				t.last[name] = 0
			}
		}
		for _, user := range warm {
			if _, ok := next.users[user.Username]; ok {
				t.last[user.Username] = max(user.TotalOctets, 0)
			}
		}
		// A real mutation may proceed without a strict boundary. Warm the ambiguous
		// interval rather than assigning old bytes to a replacement database row.
		for name, email := range next.users {
			if uncertain[email] {
				delete(t.last, name)
			}
		}
	}
	return nil
}

func webTrafficZeroBaseline(b *mtprotoWebTrafficBinding) map[string]int64 {
	last := make(map[string]int64, len(b.users))
	for name := range b.users {
		last[name] = 0
	}
	return last
}

func withMtprotoWebTrafficConfig(ctx context.Context, rendered string, mutation bool, activate func() (bool, error)) error {
	state, err := readTelemtWebState()
	if err != nil {
		return err
	}
	next, err := captureMtprotoWebTrafficBinding(state, rendered)
	if err != nil {
		return err
	}
	return mtprotoWebTraffic.transition(ctx, next, mutation, systemctl("is-active", "--quiet", telemtServiceName) == nil, func() ([]telemtUserStats, error) { return fetchMtprotoWebTraffic(ctx) }, activate)
}

func refreshMtprotoWebTrafficBinding(ctx context.Context) error {
	data, err := os.ReadFile(telemtConfigPath)
	if err != nil {
		return err
	}
	return withMtprotoWebTrafficConfig(ctx, string(data), false, func() (bool, error) { return false, nil })
}

// CollectMtprotoWebTrafficPersisted accepts immutable-ID samples into the shared
// MTProto WAL before advancing a cursor; callers replay it without re-batching.
func CollectMtprotoWebTrafficPersisted(ctx context.Context) ([]string, string, error) {
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	return mtprotoWebTraffic.poll(ctx, func() (*mtprotoWebTrafficBinding, error) {
		state, err := readTelemtWebState()
		if err != nil {
			return nil, err
		}
		if !state.Enabled || state.ManagedInboundID == 0 {
			return nil, nil
		}
		data, err := os.ReadFile(telemtConfigPath)
		if err != nil {
			return nil, err
		}
		return captureMtprotoWebTrafficBinding(state, string(data))
	}, func() bool { return systemctl("is-active", "--quiet", telemtServiceName) == nil }, func() ([]telemtUserStats, error) { return fetchMtprotoWebTraffic(ctx) })
}
