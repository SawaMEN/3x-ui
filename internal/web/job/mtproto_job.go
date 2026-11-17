package job

import (
	"context"

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

	mgr := mtproto.GetManager()
	desired, err := mgr.ReconcileCurrent(ctx, j.inboundService.DesiredMtprotoInstancesContext)
	if err != nil {
		logger.Warning("mtproto job: get desired instances failed:", err)
		return
	}

	activeTags := make([]string, 0, len(desired))
	for _, inst := range desired {
		activeTags = append(activeTags, inst.Tag)
	}

	if err := (service.TelemtService{}).RefreshMekoFix(); err != nil {
		logger.Warning("mtproto job: reconcile MEKO V3 rules failed:", err)
	}

	_, onlineEmails, collectErr := mgr.CollectTrafficPersisted()
	if collectErr != nil {
		logger.Warning("mtproto job: traffic poll incomplete:", collectErr)
	}
	webOnline, webTag, webErr := service.CollectMtprotoWebTrafficPersisted(ctx)
	if webErr != nil {
		logger.Warning("mtproto job: WEB traffic poll failed:", webErr)
	}
	onlineEmails = append(onlineEmails, webOnline...)
	if webTag != "" {
		activeTags = append(activeTags, webTag)
	}
	if _, _, err := j.journal.replay("mtproto", &j.inboundService); err != nil {
		logger.Warning("mtproto job: replay owned traffic:", err)
	}

	j.inboundService.RefreshLocalOnlineClients(onlineEmails, activeTags)
}
