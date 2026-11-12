package job

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type shutdownTrafficSample struct {
	tag, email string
	up, down   int64
	routed     bool
}

type shutdownTrafficSource struct {
	name      string
	mu        *sync.Mutex
	protocols []model.Protocol
	collect   func() ([]shutdownTrafficSample, error)
	stop      func()
}

// StopSidecarsWithTraffic must run after all panel producers have joined,
// while the running sidecars and traffic writer are still available. It samples
// then stops each manager immediately: no desired-config queries, reconciliation,
// quota or runtime maintenance.
func StopSidecarsWithTraffic() error {
	sources := []shutdownTrafficSource{
		{name: "mieru", stop: mieru.GetManager().StopAll, mu: &mieruTrafficMu, protocols: []model.Protocol{model.Mieru}, collect: func() ([]shutdownTrafficSample, error) {
			deltas, _, err := mieru.GetManager().CollectRunningTrafficChecked()
			samples := make([]shutdownTrafficSample, 0, len(deltas))
			for _, d := range deltas {
				samples = append(samples, shutdownTrafficSample{tag: d.Tag, email: d.Email, up: d.Up, down: d.Down})
			}
			return samples, err
		}},
		{name: "mtproto", stop: mtproto.GetManager().StopAll, mu: &mtprotoTrafficMu, protocols: []model.Protocol{model.MTProto}, collect: func() ([]shutdownTrafficSample, error) {
			deltas, _, rawErr := mtproto.GetManager().CollectTrafficConsistentChecked()
			// The panel context is already canceled. Use an independent bounded poll
			// for WEB's system-managed backend, without changing its service state.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			web, _, _, err := service.CollectMtprotoWebTraffic(ctx)
			deltas = append(deltas, web...)
			samples := make([]shutdownTrafficSample, 0, len(deltas))
			for _, d := range deltas {
				samples = append(samples, shutdownTrafficSample{tag: d.Tag, email: d.Email, up: d.Up, down: d.Down, routed: d.Routed})
			}
			return samples, errors.Join(rawErr, err)
		}},
		{name: "externalvpn", stop: externalvpn.GetManager().StopAll, mu: &externalTrafficMu, protocols: []model.Protocol{model.Pingtunnel, model.TrustTunnel, model.FPTN, model.VetroffProxy, model.OpenFlux}, collect: func() ([]shutdownTrafficSample, error) {
			deltas, err := externalvpn.GetManager().CollectTrafficChecked()
			samples := make([]shutdownTrafficSample, 0, len(deltas))
			for _, d := range deltas {
				samples = append(samples, shutdownTrafficSample{tag: d.Tag, email: d.Email, up: d.Up, down: d.Down})
			}
			return samples, err
		}},
	}
	var result error
	for _, source := range sources {
		if err := flushShutdownTrafficSource(source); err != nil {
			result = errors.Join(result, fmt.Errorf("%s final traffic: %w", source.name, err))
		}
	}
	return result
}

func flushShutdownTrafficSource(source shutdownTrafficSource) error {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.stop != nil {
		defer source.stop()
	}
	svc := &service.InboundService{}
	apply := func(batch service.SidecarTrafficBatch) (bool, bool, error) {
		return false, false, svc.PersistSidecarTrafficBatch(batch)
	}
	var journal sidecarTrafficJournal
	_, _, priorErr := journal.replayWith(source.name, apply)
	// A failed DB write leaves the old WAL durable, so another final batch can
	// still be saved. An unstored batch must be preserved before consuming more.
	if _, pending := unstoredSidecarTraffic.Load(sidecarJournalDir(source.name)); pending {
		return priorErr
	}
	owners, err := service.CaptureSidecarTrafficOwners(source.protocols)
	if err != nil {
		return errors.Join(priorErr, err)
	}
	samples, collectErr := source.collect()
	inbounds := make([]*coremodel.Traffic, 0, len(samples))
	clients := make([]*coremodel.ClientTraffic, 0, len(samples))
	for _, s := range samples {
		if s.up <= 0 && s.down <= 0 {
			continue
		}
		clients = append(clients, &coremodel.ClientTraffic{Email: s.email, Up: s.up, Down: s.down})
		if !s.routed {
			inbounds = append(inbounds, &coremodel.Traffic{IsInbound: true, Tag: s.tag, Up: s.up, Down: s.down})
		}
	}
	_, _, commitErr := journal.commitWith(source.name, owners, apply, inbounds, clients)
	return errors.Join(priorErr, collectErr, commitErr)
}
