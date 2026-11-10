package domainfront

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// CloudflareTXTProvider is a deliberately restricted ACME DNS-01 adapter.
// It can create and delete only the TXT records that it created, in one
// administrator-selected zone. It never lists or alters unrelated DNS records.
type CloudflareTXTProvider struct {
	zone, zoneID, token string
	endpoint string // fixed in production; overridden only in package tests
	client *http.Client
}

type cloudflareTXTRecord struct {
	rr libdns.RR
	id string
}

func (r cloudflareTXTRecord) RR() libdns.RR { return r.rr }

var cloudflareID = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
var cloudflareDNSValue = regexp.MustCompile(`^[A-Za-z0-9_-]{32,200}$`)

func NewCloudflareTXTProvider(zone, zoneID, token string) (*CloudflareTXTProvider, error) {
	zone = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone), "."))
	token = strings.TrimSpace(token)
	if !validPublicDNSName(zone) || !cloudflareID.MatchString(zoneID) ||
		token == "" || len(token) > 512 || strings.ContainsAny(token, "\r\n\t ") {
		return nil, errors.New("invalid Cloudflare DNS zone, zone identifier or API token")
	}
	return &CloudflareTXTProvider{
		zone: zone, zoneID: strings.ToLower(zoneID), token: token,
		endpoint: "https://api.cloudflare.com/client/v4",
		client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("Cloudflare API redirect refused")
		}},
	}, nil
}

func validPublicDNSName(name string) bool {
	if len(name) < 4 || len(name) > 253 || !strings.Contains(name, ".") {
		return false
	}
	for _, part := range strings.Split(name, ".") {
		if len(part) == 0 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, ch := range part {
			if ch != '-' && (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
				return false
			}
		}
	}
	return true
}

func (p *CloudflareTXTProvider) validateTXT(zone string, rr libdns.RR) (string, error) {
	if strings.ToLower(strings.TrimSuffix(zone, ".")) != p.zone ||
		(rr.Name != "_acme-challenge" && !strings.HasPrefix(rr.Name, "_acme-challenge.")) ||
		rr.Type != "TXT" || !cloudflareDNSValue.MatchString(rr.Data) {
		return "", errors.New("DNS-01 provider accepts only challenge TXT records in its configured zone")
	}
	name := strings.ToLower(libdns.AbsoluteName(rr.Name, p.zone))
	if !strings.HasSuffix(name, "."+p.zone) || !validPublicDNSName(strings.TrimSuffix(name, ".")) {
		return "", errors.New("invalid ACME TXT record name")
	}
	return strings.TrimSuffix(name, "."), nil
}

type cloudflareEnvelope struct {
	Success bool `json:"success"`
	Result struct {
		ID string `json:"id"`
	} `json:"result"`
}

func (p *CloudflareTXTProvider) request(ctx context.Context, method, suffix string, body any) (string, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return "", err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(1<<uint(attempt-1)) * time.Second)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, p.endpoint+"/zones/"+p.zoneID+"/dns_records"+suffix, bytes.NewReader(payload))
		if err != nil {
			return "", errors.New("cannot build DNS provider request")
		}
		req.Header.Set("Authorization", "Bearer "+p.token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if attempt < 2 {
				continue
			}
			return "", errors.New("Cloudflare DNS request failed")
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < 2 {
				continue
			}
			return "", fmt.Errorf("Cloudflare DNS API temporarily unavailable (HTTP %d)", resp.StatusCode)
		}
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("Cloudflare DNS API rejected request (HTTP %d); check scoped token and zone", resp.StatusCode)
		}
		var reply cloudflareEnvelope
		if json.Unmarshal(raw, &reply) != nil || !reply.Success || !cloudflareID.MatchString(reply.Result.ID) {
			return "", errors.New("Cloudflare DNS API returned an unsuccessful or invalid result")
		}
		return reply.Result.ID, nil
	}
	return "", errors.New("Cloudflare DNS request retry limit reached")
}

func (p *CloudflareTXTProvider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	result := make([]libdns.Record, 0, len(records))
	for _, record := range records {
		rr := record.RR()
		name, err := p.validateTXT(zone, rr)
		if err != nil {
			return result, err
		}
		id, err := p.request(ctx, http.MethodPost, "", struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Content string `json:"content"`
			TTL int `json:"ttl"`
		}{Type: "TXT", Name: name, Content: rr.Data, TTL: 120})
		if err != nil {
			return result, err
		}
		result = append(result, cloudflareTXTRecord{rr: rr, id: id})
	}
	return result, nil
}

func (p *CloudflareTXTProvider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	deleted := make([]libdns.Record, 0, len(records))
	for _, record := range records {
		saved, ok := record.(cloudflareTXTRecord)
		if !ok || !cloudflareID.MatchString(saved.id) {
			return deleted, errors.New("refusing to delete a DNS record not created by this ACME session")
		}
		if _, err := p.validateTXT(zone, saved.rr); err != nil {
			return deleted, err
		}
		id, err := p.request(ctx, http.MethodDelete, "/"+saved.id, nil)
		if err != nil {
			return deleted, err
		}
		if id != saved.id {
			return deleted, errors.New("Cloudflare deletion returned another record identifier")
		}
		deleted = append(deleted, saved)
	}
	return deleted, nil
}
