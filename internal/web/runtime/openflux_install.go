package runtime

import (
	"context"
	"encoding/json"
	"net/http"
)

// Optional so older runtimes remain usable and cannot accidentally install on
// the master when a selected node does not implement component installation.
type OpenFluxInstaller interface {
	StartOpenFluxInstall(context.Context) (json.RawMessage, error)
	ReadOpenFluxInstall(context.Context) (json.RawMessage, error)
	ReadOpenFluxStatus(context.Context) (json.RawMessage, error)
}

func (r *Remote) StartOpenFluxInstall(ctx context.Context) (json.RawMessage, error) {
	response, err := r.do(ctx, http.MethodPost, "panel/api/server/openflux/install", nil)
	if err != nil {
		return nil, err
	}
	return response.Obj, nil
}

func (r *Remote) ReadOpenFluxInstall(ctx context.Context) (json.RawMessage, error) {
	response, err := r.do(ctx, http.MethodGet, "panel/api/server/openflux/install", nil)
	if err != nil {
		return nil, err
	}
	return response.Obj, nil
}

func (r *Remote) ReadOpenFluxStatus(ctx context.Context) (json.RawMessage, error) {
	response, err := r.do(ctx, http.MethodGet, "panel/api/server/openflux/status", nil)
	if err != nil {
		return nil, err
	}
	return response.Obj, nil
}
