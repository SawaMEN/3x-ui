package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// DomainActivityReader is optional: old runtimes do not need to implement it.
type DomainActivityReader interface {
	DomainActivity(ctx context.Context) (json.RawMessage, error)
}

func (m *Manager) DomainActivityReaderFor(nodeID *int) (DomainActivityReader, error) {
	rt, err := m.RuntimeFor(nodeID)
	if err != nil {
		return nil, err
	}
	reader, ok := rt.(DomainActivityReader)
	if !ok {
		return nil, fmt.Errorf("runtime %s does not support domain activity", rt.Name())
	}
	return reader, nil
}

// DomainActivity reads the remote node's own opted-in, bounded snapshot.
// Node-sync credentials receive GET-only scope; no remote settings are changed.
func (r *Remote) DomainActivity(ctx context.Context) (json.RawMessage, error) {
	env, err := r.do(ctx, http.MethodGet, "panel/api/server/domainActivity", nil)
	if err != nil {
		return nil, err
	}
	if len(env.Obj) == 0 || string(env.Obj) == "null" {
		return nil, errors.New("node returned no domain activity state")
	}
	if len(env.Obj) > 256<<10 {
		return nil, errors.New("node domain activity response is too large")
	}
	return append(json.RawMessage(nil), env.Obj...), nil
}
