package job

import (
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/psiphon"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type PsiphonJob struct{ inboundService service.InboundService }

func NewPsiphonJob() *PsiphonJob { return new(PsiphonJob) }

func (j *PsiphonJob) Run() {
	desired, err := j.inboundService.DesiredPsiphonInstances()
	if err != nil {
		logger.Warning("psiphon job: get desired instances failed:", err)
		return
	}
	psiphon.GetManager().Reconcile(desired)
}
