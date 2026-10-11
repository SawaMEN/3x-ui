package hiddify

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/core"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Runtime adapts the supervised raw-config process to the shared lifecycle
// contract. The service layer owns DB configuration and watchdog scheduling.
type Runtime struct {
	Process *singbox.Process
}

var _ core.Runtime = (*Runtime)(nil)

func NewRuntime(path string, isolated bool) *Runtime {
	return &Runtime{Process: NewProcess(path, isolated)}
}

func (r *Runtime) Type() core.Type                             { return core.Hiddify }
func (r *Runtime) Version(ctx context.Context) (string, error) { return r.Process.Version(ctx) }
func (r *Runtime) Start(ctx context.Context) error             { return r.Process.Start(ctx) }
func (r *Runtime) Stop(_ context.Context) error {
	// Panel shutdown cancels its context before stopping runtimes. Cleanup must
	// still terminate the child; Process.Stop has bounded graceful/force waits.
	return r.Process.Stop()
}
func (r *Runtime) Restart(ctx context.Context) error        { return r.Process.Restart(ctx) }
func (r *Runtime) IsRunning() bool                          { return r.Process.IsRunning() }
func (r *Runtime) ValidateConfig(ctx context.Context) error { return r.Process.Validate(ctx) }

func NewStatsClient() *singbox.V2RayStatsClient {
	return singbox.NewV2RayStatsClient("127.0.0.1:10086", StatsQueryMethod)
}
