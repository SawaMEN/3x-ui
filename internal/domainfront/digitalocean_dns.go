package domainfront

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// DigitalOceanTXTProvider is limited to one configured DNS zone and the
// ACME _acme-challenge TXT namespace. Credentials should have domain-record
// permissions only; creation IDs never leave this provider.
type DigitalOceanTXTProvider struct {
	zone, token string
	endpoint    string
	client      *http.Client
}

type digitalOceanTXTRecord struct {
	rr libdns.RR
	id int64
}

func (r digitalOceanTXTRecord) RR() libdns.RR { return r.rr }

func NewDigitalOceanTXTProvider(zone, token string) (*DigitalOceanTXTProvider, error) {
	zone = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone), "."))
	if !validPublicDNSName(zone) || len(token) < 20 || len(token) > 512 ||
		strings.TrimSpace(token) != token || strings.ContainsAny(token, "\r\n\t ") {
		return nil, errors.New("invalid DigitalOcean DNS zone or API token")
	}
	return &DigitalOceanTXTProvider{
		zone: zone, token: token, endpoint: "https://api.digitalocean.com/v2",
		client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("DigitalOcean API redirect refused")
		}},
	}, nil
}

func (p *DigitalOceanTXTProvider) request(ctx context.Context, method, suffix string, body any) (int64, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return 0, errors.New("could not encode DNS record")
		}
	}
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			wait := time.NewTimer(time.Duration(1<<uint(retry-1)) * time.Second)
			select {
			case <-wait.C:
			case <-ctx.Done():
				wait.Stop()
				return 0, ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, method,
			p.endpoint+"/domains/"+p.zone+"/records"+suffix, bytes.NewReader(payload))
		if err != nil {
			return 0, errors.New("cannot build DNS provider request")
		}
		req.Header.Set("Authorization", "Bearer "+p.token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			if retry < 2 {
				continue
			}
			return 0, errors.New("DigitalOcean DNS API request failed")
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if retry < 2 {
				continue
			}
			return 0, fmt.Errorf("DigitalOcean DNS API temporarily unavailable (HTTP %d)", resp.StatusCode)
		}
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return 0, fmt.Errorf("DigitalOcean DNS API rejected request (HTTP %d); check scoped token and zone", resp.StatusCode)
		}
		if method == http.MethodDelete && resp.StatusCode == http.StatusNoContent {
			return 0, nil
		}
		var parsed struct {
			DomainRecord struct {
				ID int64 `json:"id"`
			} `json:"domain_record"`
		}
		if json.Unmarshal(data, &parsed) != nil || parsed.DomainRecord.ID <= 0 {
			return 0, errors.New("DigitalOcean DNS API returned an invalid record ID")
		}
		return parsed.DomainRecord.ID, nil
	}
	return 0, errors.New("DigitalOcean retry limit reached")
}

func (p *DigitalOceanTXTProvider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	out := make([]libdns.Record, 0, len(records))
	for _, record := range records {
		rr := record.RR()
		name, err := validateDNS01TXTName(zone, rr, p.zone)
		if err != nil {
			return out, err
		}
		name = strings.TrimSuffix(name, "."+p.zone)
		id, err := p.request(ctx, http.MethodPost, "", struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Data string `json:"data"`
			TTL  int    `json:"ttl"`
		}{Type: "TXT", Name: name, Data: rr.Data, TTL: 120})
		if err != nil {
			return out, err
		}
		out = append(out, digitalOceanTXTRecord{rr: rr, id: id})
	}
	return out, nil
}

func (p *DigitalOceanTXTProvider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	out := make([]libdns.Record, 0, len(records))
	for _, record := range records {
		owned, ok := record.(digitalOceanTXTRecord)
		if !ok || owned.id <= 0 {
			return out, errors.New("refusing to delete a DNS record not created by this session")
		}
		if _, err := validateDNS01TXTName(zone, owned.rr, p.zone); err != nil {
			return out, err
		}
		_, err := p.request(ctx, http.MethodDelete, fmt.Sprintf("/%d", owned.id), nil)
		if err != nil {
			return out, err
		}
		out = append(out, owned)
	}
	return out, nil
}
