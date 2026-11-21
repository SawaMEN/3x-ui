package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
)

func (r *Remote) FetchTelegramSubscription(ctx context.Context, subID string) (coremodel.TelegramSubscription, error) {
	var result coremodel.TelegramSubscription
	response, err := r.do(ctx, http.MethodGet, "/panel/api/telemt/subscription/"+url.PathEscape(subID), nil)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(response.Obj, &result)
	return result, err
}

func (r *Remote) FetchPublishedTelegramProxies(ctx context.Context) ([]coremodel.TelegramProxyProfile, error) {
	response, err := r.do(ctx, http.MethodGet, "/panel/api/telemt/published", nil)
	if err != nil {
		return nil, err
	}
	var result []coremodel.TelegramProxyProfile
	err = json.Unmarshal(response.Obj, &result)
	return result, err
}

func (r *Remote) TelemtWebStatus(ctx context.Context, tag string) (json.RawMessage, error) {
	return r.telemtWebInspect(ctx, http.MethodGet, "/panel/api/telemt/webproxy/status", tag, nil)
}

func (r *Remote) TelemtWebPreflight(ctx context.Context, tag string, settings any) (json.RawMessage, error) {
	return r.telemtWebInspect(ctx, http.MethodPost, "/panel/api/telemt/webproxy/preflight", tag, settings)
}

func (r *Remote) telemtWebInspect(ctx context.Context, method, path, tag string, settings any) (json.RawMessage, error) {
	if tag != "" {
		id, err := r.resolveRemoteID(ctx, tag)
		if err != nil && !errors.Is(err, errRemoteInboundNotFound) {
			return nil, err
		}
		// A saved central row may still be waiting for its first push to the node.
		if id > 0 {
			path += "?inboundId=" + strconv.Itoa(id)
		}
	}
	response, err := r.do(ctx, method, path, settings)
	if err != nil {
		return nil, err
	}
	return response.Obj, nil
}
