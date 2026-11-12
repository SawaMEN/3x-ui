package job

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// MtprotoJob reconciles the running Telemt sidecar processes against the enabled
// MTProto inbounds in the database, restarts any that crashed, and folds the
// per-client traffic scraped from each Telemt API into the usual client and
// inbound traffic accounting.
type MtprotoJob struct {
	ctx            context.Context
	inboundService service.InboundService
	journal        sidecarTrafficJournal
}

// NewMtprotoJob creates a new MTProto reconcile/traffic job instance.
func NewMtprotoJob(parents ...context.Context) *MtprotoJob {
	ctx := context.Background()
	if len(parents) > 0 && parents[0] != nil {
		ctx = parents[0]
	}
	return &MtprotoJob{ctx: ctx}
}

// Run reconciles desired MTProto inbounds with running Telemt processes and
// records per-client traffic deltas and online status.
func (j *MtprotoJob) Run() {
	ctx := j.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	// CollectTrafficConsistent advances Telemt's per-client snapshot baseline.
	// Serialize the poll so two scheduler runs cannot consume overlapping snapshots.
	mtprotoTrafficMu.Lock()
	defer mtprotoTrafficMu.Unlock()

	_, _, retryErr := j.journal.replay("mtproto", &j.inboundService)
	if retryErr != nil {
		logger.Warning("mtproto job: replay traffic journal:", retryErr)
		return
	}
	owners, ownerErr := service.CaptureSidecarTrafficOwners([]model.Protocol{model.MTProto})
	if ownerErr != nil {
		logger.Warning("mtproto job: capture traffic owners:", ownerErr)
		return
	}

	desired, err := j.inboundService.DesiredMtprotoInstances()
	if err != nil {
		logger.Warning("mtproto job: get desired instances failed:", err)
		return
	}

	routedTags := make(map[string]bool)
	activeTags := make([]string, 0, len(desired))
	for _, inst := range desired {
		activeTags = append(activeTags, inst.Tag)
		if inst.RouteThroughXray {
			routedTags[inst.Tag] = true
		}
	}

	mgr := mtproto.GetManager()
	mgr.Reconcile(desired)
	if err := (service.TelemtService{}).RefreshMekoFix(); err != nil {
		logger.Warning("mtproto job: reconcile MEKO V3 rules failed:", err)
	}

	deltas, onlineEmails := mgr.CollectTrafficConsistent()
	webDeltas, webOnline, webTag, webErr := service.CollectMtprotoWebTraffic(ctx)
	if webErr != nil {
		logger.Warning("mtproto job: WEB traffic poll failed:", webErr)
	} else {
		deltas = append(deltas, webDeltas...)
		onlineEmails = append(onlineEmails, webOnline...)
		if webTag != "" {
			activeTags = append(activeTags, webTag)
		}
	}

	// A routed inbound's total is already metered through the Xray bridge by
	// xray_traffic_job, so only non-routed inbounds are rolled up here; per-client
	// deltas are always kept, since the bridge cannot tell MTProto users apart.
	clientTraffics := make([]*coremodel.ClientTraffic, 0, len(deltas))
	inboundUp := make(map[string]int64)
	inboundDown := make(map[string]int64)
	for _, d := range deltas {
		clientTraffics = append(clientTraffics, &coremodel.ClientTraffic{
			Email: d.Email,
			Up:    d.Up,
			Down:  d.Down,
		})
		if !routedTags[d.Tag] {
			inboundUp[d.Tag] = accumulateTrafficDelta(inboundUp[d.Tag], d.Up)
			inboundDown[d.Tag] = accumulateTrafficDelta(inboundDown[d.Tag], d.Down)
		}
	}

	traffics := make([]*coremodel.Traffic, 0, len(inboundUp))
	for tag, up := range inboundUp {
		traffics = append(traffics, &coremodel.Traffic{
			IsInbound: true,
			Tag:       tag,
			Up:        up,
			Down:      inboundDown[tag],
		})
	}

	if len(traffics) > 0 || len(clientTraffics) > 0 {
		if _, _, err := j.journal.commit("mtproto", owners, &j.inboundService, traffics, clientTraffics); err != nil {
			// The collector has already moved Telemt's baseline. Preserve the exact
			// delta batch and retry it before collecting a newer snapshot.
			logger.Warning("mtproto job: add traffic failed; batch queued for retry:", err)
		}
	}

	j.inboundService.RefreshLocalOnlineClients(onlineEmails, activeTags)
}
