package job

import (
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type MieruJob struct{ inboundService service.InboundService }

func NewMieruJob() *MieruJob { return new(MieruJob) }

func (j *MieruJob) Run() {
	desired, err := j.inboundService.DesiredMieruInstances()
	if err != nil {
		logger.Warning("mieru job: get desired instances failed:", err)
		return
	}
	mieru.GetManager().Reconcile(desired)
}
