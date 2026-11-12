package web

import (
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
)

// Manually invoked cron wrappers are not counted by cron.Stop. Own them before
// launching so shutdown cannot return while they still use the database/runtime.
func (s *Server) runStartupTask(run func()) {
	s.startupTasks.Add(1)
	common.GoRecover("panel-startup", func() {
		defer s.startupTasks.Done()
		if s.ctx.Err() != nil {
			return
		}
		run()
	})
}

func (s *Server) runDelayedStartupTask(delay time.Duration, run func()) {
	s.runStartupTask(func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			if s.ctx.Err() == nil {
				run()
			}
		}
	})
}
