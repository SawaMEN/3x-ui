package network

import (
	"errors"
	"net"
	"net/http"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

// ServeHTTP runs a panel HTTP server and records unexpected listener failures.
// Unexpected failures are returned to the caller. A normal Shutdown is silent
// and returns nil.
func ServeHTTP(server *http.Server, listener net.Listener, name string) error {
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error(name, " stopped unexpectedly: ", err)
		return err
	}
	return nil
}
