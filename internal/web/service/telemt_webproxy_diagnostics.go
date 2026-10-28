package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TelemtWebRuntimeDiagnostics is a bounded, read-only view of Telemt's
// authenticated runtime endpoint. Session IDs, user secrets and request bodies
// are deliberately excluded from this panel API response.
type TelemtWebRuntimeDiagnostics struct {
	Available          bool            `json:"available"`
	Lifecycle          string          `json:"lifecycle"`
	Listeners          []string        `json:"listeners"`
	Capacity           json.RawMessage `json:"capacity,omitempty"`
	CarrierNegotiation json.RawMessage `json:"carrierNegotiation,omitempty"`
	ActiveSessions      *int            `json:"activeSessions,omitempty"`
	CheckedAt          time.Time       `json:"checkedAt"`
}

func (TelemtService) WebProxyRuntimeDiagnostics(ctx context.Context) (TelemtWebRuntimeDiagnostics, error) {
	state, err := readTelemtWebState()
	if err != nil {
		return TelemtWebRuntimeDiagnostics{}, err
	}
	if !state.Enabled {
		return TelemtWebRuntimeDiagnostics{}, errors.New("WEB Proxy не включён")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := telemtAPIRequest(http.MethodGet, "/v1/runtime/web/status")
	if err != nil {
		return TelemtWebRuntimeDiagnostics{}, err
	}
	request = request.WithContext(ctx)
	client := &http.Client{Timeout: 5*time.Second, Transport: telemtMonitorTransport{ctx}}
	response, err := client.Do(request)
	if err != nil {
		return TelemtWebRuntimeDiagnostics{}, fmt.Errorf("Telemt WEB runtime API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed {
		return TelemtWebRuntimeDiagnostics{}, errors.New("эта версия Telemt не поддерживает диагностику WEB runtime")
	}
	if response.StatusCode != http.StatusOK {
		return TelemtWebRuntimeDiagnostics{}, fmt.Errorf("Telemt WEB runtime API вернул HTTP %d", response.StatusCode)
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Available          bool            `json:"available"`
			Lifecycle          string          `json:"lifecycle"`
			Listeners          []string        `json:"listeners"`
			Capacity           json.RawMessage `json:"capacity"`
			CarrierNegotiation json.RawMessage `json:"carrier_negotiation"`
			Runtime *struct {
				Manager *struct {
					Sessions int `json:"sessions"`
				} `json:"manager"`
			} `json:"runtime"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&payload); err != nil {
		return TelemtWebRuntimeDiagnostics{}, fmt.Errorf("ошибка чтения WEB runtime: %w", err)
	}
	if !payload.OK {
		return TelemtWebRuntimeDiagnostics{}, errors.New("Telemt отклонил запрос диагностики WEB")
	}
	var sessions *int
	if payload.Data.Runtime != nil && payload.Data.Runtime.Manager != nil {
		count := payload.Data.Runtime.Manager.Sessions
		sessions = &count
	}
	return TelemtWebRuntimeDiagnostics{
		ActiveSessions: sessions,
		Available: payload.Data.Available,
		Lifecycle: payload.Data.Lifecycle,
		Listeners: payload.Data.Listeners,
		Capacity: payload.Data.Capacity,
		CarrierNegotiation: payload.Data.CarrierNegotiation,
		CheckedAt: time.Now().UTC(),
	}, nil
}
