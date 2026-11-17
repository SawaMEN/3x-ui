package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TelemtWebAvailability is a manual, remote HTTPS probe of the WEB decoy.
// It is NOT a check of MTProto credentials or Russian cellular allowlists.
type TelemtWebAvailability struct {
	Domain      string    `json:"domain"`
	Probes      int       `json:"probes"`
	Reachable   int       `json:"reachable"`
	CheckedAt   time.Time `json:"checkedAt"`
	Description string    `json:"description"`
}

var telemtWebAvailabilityCache struct {
	sync.Mutex
	lastAttempt time.Time
	cached      TelemtWebAvailability
}

const globalpingMeasurementsAPI = "https://api.globalping.io/v1/measurements"

func (TelemtService) CheckWebAvailabilityFromRussia(ctx context.Context) (TelemtWebAvailability, error) {
	state, err := readTelemtWebState()
	if err != nil {
		return TelemtWebAvailability{}, err
	}
	if !state.Enabled || !telemtWebFrontendActive(state) {
		return TelemtWebAvailability{}, errors.New("WEB Proxy HTTPS frontend must be active before an external check")
	}
	domain := normalizeTelemtWebDomain(state.Domain)
	if !telemtWebDomainPattern.MatchString(domain) {
		return TelemtWebAvailability{}, errors.New("WEB Proxy has no valid public domain")
	}

	telemtWebAvailabilityCache.Lock()
	if telemtWebAvailabilityCache.cached.Domain == domain &&
		time.Since(telemtWebAvailabilityCache.cached.CheckedAt) < 15*time.Minute {
		result := telemtWebAvailabilityCache.cached
		telemtWebAvailabilityCache.Unlock()
		return result, nil
	}
	if time.Since(telemtWebAvailabilityCache.lastAttempt) < 15*time.Minute {
		telemtWebAvailabilityCache.Unlock()
		return TelemtWebAvailability{}, errors.New("Globalping checks are limited to once per 15 minutes")
	}
	telemtWebAvailabilityCache.lastAttempt = time.Now()
	telemtWebAvailabilityCache.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 12 * time.Second}
	payload := map[string]any{
		"type":               "http",
		"target":             domain,
		"locations":          []map[string]any{{"country": "RU", "limit": 3}},
		"measurementOptions": map[string]any{"protocol": "HTTPS", "port": 443, "request": map[string]string{"method": "HEAD"}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TelemtWebAvailability{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, globalpingMeasurementsAPI, bytes.NewReader(body))
	if err != nil {
		return TelemtWebAvailability{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "3x-ui-telemt-availability/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return TelemtWebAvailability{}, fmt.Errorf("Globalping request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		return TelemtWebAvailability{}, fmt.Errorf("Globalping returned HTTP %d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&created); err != nil {
		return TelemtWebAvailability{}, err
	}
	if len(created.ID) == 0 || len(created.ID) > 128 ||
		strings.ContainsAny(created.ID, "/?#") {
		return TelemtWebAvailability{}, errors.New("Globalping returned an invalid measurement ID")
	}
	for {
		select {
		case <-ctx.Done():
			return TelemtWebAvailability{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
		getReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, globalpingMeasurementsAPI+"/"+created.ID, nil)
		if reqErr != nil {
			return TelemtWebAvailability{}, reqErr
		}
		getReq.Header.Set("User-Agent", "3x-ui-telemt-availability/1.0")
		response, reqErr := client.Do(getReq)
		if reqErr != nil {
			return TelemtWebAvailability{}, reqErr
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return TelemtWebAvailability{}, fmt.Errorf("Globalping measurement returned HTTP %d", response.StatusCode)
		}
		var measurement struct {
			Status  string `json:"status"`
			Results []struct {
				Result struct {
					Status     string `json:"status"`
					StatusCode int    `json:"statusCode"`
				} `json:"result"`
			} `json:"results"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&measurement)
		response.Body.Close()
		if decodeErr != nil {
			return TelemtWebAvailability{}, decodeErr
		}
		if measurement.Status != "finished" {
			continue
		}
		if len(measurement.Results) == 0 {
			return TelemtWebAvailability{}, errors.New("Globalping returned no Russian probes")
		}
		result := TelemtWebAvailability{
			Domain: domain, Probes: len(measurement.Results), CheckedAt: time.Now().UTC(),
			Description: "External HTTPS decoy check; does not test MTProto or mobile allowlists",
		}
		for _, probe := range measurement.Results {
			if probe.Result.Status == "finished" && probe.Result.StatusCode >= 100 && probe.Result.StatusCode <= 599 {
				result.Reachable++
			}
		}
		telemtWebAvailabilityCache.Lock()
		telemtWebAvailabilityCache.cached = result
		telemtWebAvailabilityCache.Unlock()
		return result, nil
	}
}
