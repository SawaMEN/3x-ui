package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// SNIJobRuntime is optional, so older runtimes report a clear unsupported API
// error instead of silently scanning from the central panel.
type SNIJobRuntime interface {
	RequestSNIScan(context.Context, string, string, any) (json.RawMessage, error)
}

func (r *Remote) RequestSNIScan(ctx context.Context, method, resource string, body any) (json.RawMessage, error) {
	valid := resource == "jobs" && method == http.MethodPost || resource == "history" && method == http.MethodGet
	if strings.HasPrefix(resource, "jobs/") && !strings.Contains(strings.TrimPrefix(resource, "jobs/"), "/") && (method == http.MethodGet || method == http.MethodDelete) {
		valid = true
	}
	if !valid {
		return nil, errors.New("unsupported SNI job operation")
	}
	parts := strings.Split(resource, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	response, err := r.do(ctx, method, "panel/api/server/sni/"+strings.Join(parts, "/"), body)
	if err != nil {
		return nil, err
	}
	return response.Obj, nil
}
