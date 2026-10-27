package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

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
