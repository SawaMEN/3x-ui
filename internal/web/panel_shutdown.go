package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/global"
	"github.com/SawaMEN/3x-ui/v3/internal/web/network"
)

// Retired workers must finish before process-wide shutdown closes the database.
var retiredPanelTasks sync.WaitGroup

// Close active keep-alive connections as well as the listening socket.
func (s *Server) closeHTTP() error {
	var result error
	if s.httpServer != nil {
		result = s.httpServer.Close()
	}
	if s.listener != nil {
		if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			result = errors.Join(result, err)
		}
	}
	return result
}

// Full shutdown still joins workers before the database closes; panel reloads have a deadline.
func waitPanelTask(ctx context.Context, name string, wait func()) error {
	if ctx.Done() == nil {
		wait()
		return nil
	}
	done := make(chan struct{})
	retiredPanelTasks.Add(1)
	go func() { defer retiredPanelTasks.Done(); wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		logger.Warningf("Panel restart: no longer waiting for %s: %v", name, ctx.Err())
		return fmt.Errorf("wait for %s: %w", name, ctx.Err())
	}
}

// A dead listener otherwise leaves a healthy-looking process with no panel.
// The existing restart hook rebuilds only the panel and subscription servers.
func (s *Server) serveHTTP(server *http.Server, listener net.Listener) {
	if err := network.ServeHTTP(server, listener, "Web server"); err != nil && s.ctx.Err() == nil {
		logger.Error("Panel HTTP listener failed; requesting panel restart")
		if !global.TriggerRestart() {
			logger.Error("Cannot restart panel: restart hook is not registered")
		}
	}
}
