package tgbot

import (
	"fmt"
	"io"
	"net/http"
)

const maxClientSubscriptionBytes = 2 << 20

// Both text and QR actions use this bound; remote error pages must not become
// proxy links, and a large response must not exhaust the panel's memory.
func readClientSubscriptionResponse(resp *http.Response) ([]byte, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxClientSubscriptionBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxClientSubscriptionBytes {
		return nil, fmt.Errorf("subscription exceeds %d bytes", maxClientSubscriptionBytes)
	}
	return raw, nil
}
