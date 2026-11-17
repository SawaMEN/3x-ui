package job

import (
	"context"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/eventbus"

	"github.com/shirou/gopsutil/v4/cpu"
)

// CheckCpuJob monitors CPU usage and publishes events when threshold is exceeded.
type CheckCpuJob struct{ ctx context.Context }

// NewCheckCpuJob creates a new CPU monitoring job instance.
func NewCheckCpuJob() *CheckCpuJob {
	return NewCheckCpuJobWithContext(context.Background())
}

func NewCheckCpuJobWithContext(ctx context.Context) *CheckCpuJob {
	return &CheckCpuJob{ctx: ctx}
}

// Run checks CPU usage and publishes a cpu.high event with raw metric data.
func (j *CheckCpuJob) Run() {
	ctx := j.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	percent, err := cpu.PercentWithContext(ctx, time.Minute, false)
	if err != nil || ctx.Err() != nil || len(percent) == 0 {
		return
	}

	if EventBus != nil {
		EventBus.Publish(eventbus.Event{
			Type: eventbus.EventCPUHigh,
			Data: &eventbus.SystemMetricData{
				Percent: percent[0],
			},
		})
	}
}
