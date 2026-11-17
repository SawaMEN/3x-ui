package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// ReadDomainFrontStatus retrieves the node's current domain config, preserving
// unrelated domains and listener options before an explicit change.
func (r *Remote) ReadDomainFrontStatus(ctx context.Context) (json.RawMessage, error) {
	response, err := r.do(ctx, http.MethodGet, "panel/api/domains/status", nil)
	if err != nil {
		return nil, err
	}
	return response.Obj, nil
}

// ApplyDomainFrontConfig uses the node's authenticated panel API and its own
// CertMagic manager. Never copy certificate/key paths from the master panel.
func (r *Remote) ApplyDomainFrontConfig(ctx context.Context, cfg any) (json.RawMessage, error) {
	response, err := r.doWithTimeout(ctx, http.MethodPost, "panel/api/domains/apply", cfg, 90*time.Second)
	if err != nil {
		return nil, err
	}
	return response.Obj, nil
}
