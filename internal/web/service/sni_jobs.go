package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// Shared by every synchronous and asynchronous scanner on this panel.
var sniProbeSlots = make(chan struct{}, realityScanConcurrency)
var sniProbeLimiter = rate.NewLimiter(16, 8)

type sniHTTPCheckKey struct{}
type sniProgressKey struct{}
type sniProgress func(int, int, *RealityScanResult)

func emitSNIProgress(ctx context.Context, total, completed int, result *RealityScanResult) {
	if callback, ok := ctx.Value(sniProgressKey{}).(sniProgress); ok {
		callback(total, completed, result)
	}
}

type SNISavedTarget struct {
	IP   string `json:"ip"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

type SNIScanRequest struct {
	Mode    string `json:"mode,omitempty" form:"mode"`
	Targets string `json:"targets,omitempty" form:"targets"`
	Purpose string `json:"purpose,omitempty" form:"purpose"`
	// Recheck pins both IP and SNI from a saved result; never resolves it to
	// another destination. The ordinary SSRF guard still applies to every IP.
	Recheck []*SNISavedTarget `json:"recheck,omitempty"`
	NodeID  *int              `json:"nodeId,omitempty" form:"nodeId"`
}

type SNIJobSnapshot struct {
	ID          string               `json:"id"`
	State       string               `json:"state"`
	Phase       string               `json:"phase"`
	Source      string               `json:"source"`
	CreatedAt   string               `json:"createdAt"`
	FinishedAt  string               `json:"finishedAt,omitempty"`
	Total       int                  `json:"total"`
	Completed   int                  `json:"completed"`
	Verified    int                  `json:"verified"`
	VerifyTotal int                  `json:"verifyTotal"`
	Results     []*RealityScanResult `json:"results"`
	Error       string               `json:"error,omitempty"`
}

type sniJob struct {
	ctx      context.Context
	mu       sync.Mutex
	view     SNIJobSnapshot
	request  SNIScanRequest
	cancel   context.CancelFunc
	lastSeen time.Time
}

func (j *sniJob) snapshot() SNIJobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.lastSeen = time.Now()
	view := j.view
	view.Results = append([]*RealityScanResult{}, view.Results...)
	return view
}
func (j *sniJob) publish(total, completed int, result *RealityScanResult) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.view.Total += total
	j.view.Completed += completed
	if result != nil && result.CertValid {
		j.view.Results = dedupSNIJobResults(append(j.view.Results, pinSNIJobResult(result)))
		rankSNIResults(j.view.Results, j.request.Purpose)
		if len(j.view.Results) > 100 {
			j.view.Results = j.view.Results[:100]
		}
	}
}

type sniJobManager struct {
	mu   sync.Mutex
	jobs map[string]*sniJob
	// One executing scan and at most four accepted jobs. FIFO admission is
	// provided by the queue; no unbounded goroutine or network fan-out.
	queue chan *sniJob
	count int
	run   func(context.Context, *sniJob) error
	save  func(SNIJobSnapshot) error
}

func newSNIJobManager(run func(context.Context, *sniJob) error, save func(SNIJobSnapshot) error) *sniJobManager {
	m := &sniJobManager{jobs: make(map[string]*sniJob), queue: make(chan *sniJob, 4), run: run, save: save}
	go m.worker()
	return m
}

var sniJobsOnce sync.Once
var panelSNIJobs *sniJobManager

func sniJobs() *sniJobManager {
	sniJobsOnce.Do(func() { panelSNIJobs = newSNIJobManager(runSNIJob, saveSNIHistory) })
	return panelSNIJobs
}
func (m *sniJobManager) start(request SNIScanRequest) (SNIJobSnapshot, error) {
	if request.Mode == "" {
		request.Mode = "combined"
	}
	if request.Mode != "candidates" && request.Mode != "nearby" && request.Mode != "combined" && request.Mode != "recheck" {
		return SNIJobSnapshot{}, errors.New("unknown SNI scan mode")
	}
	if request.Purpose == "" {
		request.Purpose = "reality"
	}
	if !slices.Contains([]string{"reality", "tls", "shadowtls", "fakeTls"}, request.Purpose) {
		return SNIJobSnapshot{}, errors.New("unknown SNI scan purpose")
	}
	if len(request.Targets) > 16384 || len(request.Recheck) > 20 || request.Mode == "recheck" && len(request.Recheck) == 0 {
		return SNIJobSnapshot{}, errors.New("invalid or oversized SNI scan request")
	}
	for _, row := range request.Recheck {
		if row == nil || net.ParseIP(row.IP) == nil || row.Host == "" || row.Port < 1 || row.Port > 65535 {
			return SNIJobSnapshot{}, errors.New("recheck requires an IP, SNI and valid port")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.count >= 4 {
		return SNIJobSnapshot{}, errors.New("SNI scan queue is full; try again later")
	}
	for id, j := range m.jobs {
		j.mu.Lock()
		expired := j.view.FinishedAt != "" && time.Since(j.lastSeen) > 15*time.Minute
		j.mu.Unlock()
		if expired {
			delete(m.jobs, id)
		}
	}
	if len(m.jobs) >= 64 {
		return SNIJobSnapshot{}, errors.New("too many retained SNI jobs; try again later")
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()
	j := &sniJob{request: request, cancel: cancel, lastSeen: now, ctx: ctx, view: SNIJobSnapshot{ID: uuid.NewString(), State: "queued", Phase: "queued", Source: "local", CreatedAt: now.UTC().Format(time.RFC3339), Results: []*RealityScanResult{}}}
	m.jobs[j.view.ID] = j
	m.count++
	// A disconnected browser cannot leave queued scans consuming resources.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				j.mu.Lock()
				idle := time.Since(j.lastSeen) > 30*time.Second
				j.mu.Unlock()
				if idle {
					cancel()
					return
				}
			}
		}
	}()
	m.queue <- j
	return j.snapshot(), nil
}

func (m *sniJobManager) worker() {
	for j := range m.queue {
		ctx := j.ctx
		ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		j.mu.Lock()
		j.view.State = "running"
		j.view.Phase = "discovering"
		j.mu.Unlock()
		err := ctx.Err()
		if err == nil {
			err = m.run(ctx, j)
		}
		j.mu.Lock()
		view := j.view
		view.Results = append([]*RealityScanResult{}, view.Results...)
		if errors.Is(err, context.Canceled) {
			view.State = "canceled"
		} else if err != nil {
			view.State = "failed"
			view.Error = err.Error()
		} else {
			view.State = "done"
		}
		view.Phase = view.State
		view.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		j.view.State = "saving"
		j.view.Phase = "saving"
		j.mu.Unlock()
		if view.State == "done" && m.save != nil {
			if err := m.save(view); err != nil {
				view.Error = "history save failed: " + err.Error()
			}
		}
		j.mu.Lock()
		j.view = view
		j.mu.Unlock()

		cancel()
		j.cancel()

		m.mu.Lock()
		m.count--
		m.mu.Unlock()
	}
}
func (m *sniJobManager) get(id string, cancel bool) (SNIJobSnapshot, error) {
	m.mu.Lock()
	j := m.jobs[id]
	m.mu.Unlock()
	if j == nil {
		return SNIJobSnapshot{}, errors.New("SNI scan not found or expired")
	}
	if cancel {
		j.cancel()
	}
	return j.snapshot(), nil
}

func pinSNIJobResult(row *RealityScanResult) *RealityScanResult {
	if row == nil || !row.CertValid || net.ParseIP(row.IP) == nil {
		return row
	}
	out := *row
	out.Target = net.JoinHostPort(row.IP, fmt.Sprint(row.Port))
	out.ServerNames = []string{row.Host}
	return &out
}
func dedupSNIJobResults(rows []*RealityScanResult) []*RealityScanResult {
	out := make([]*RealityScanResult, 0, len(rows))
	seen := make(map[string]int)
	for _, row := range rows {
		if row == nil {
			continue
		}
		key := row.Target + "|" + row.Host
		if i, ok := seen[key]; ok {
			if betterRealityResult(row, out[i]) {
				out[i] = row
			}
		} else {
			seen[key] = len(out)
			out = append(out, row)
		}
	}
	return out
}

func sniUsable(r *RealityScanResult, purpose string) bool {
	return r != nil && (r.Attempts == 0 || r.Successes*5 >= r.Attempts*4) && r.CertValid && (purpose != "reality" || r.Feasible) && (purpose != "shadowtls" && purpose != "fakeTls" || r.TLS13)
}
func rankSNIResults(rows []*RealityScanResult, purpose string) {
	slices.SortStableFunc(rows, func(a, b *RealityScanResult) int {
		if sniUsable(a, purpose) != sniUsable(b, purpose) {
			if sniUsable(a, purpose) {
				return -1
			}
			return 1
		}
		aStable, bStable := a.Attempts > 0 && a.Successes >= 4, b.Attempts > 0 && b.Successes >= 4
		if aStable != bStable {
			if aStable {
				return -1
			}
			return 1
		}
		if a.Attempts > 0 && b.Attempts > 0 && a.Successes != b.Successes {
			return b.Successes - a.Successes
		}
		av, bv := a.MedianMs, b.MedianMs
		if a.Attempts == 0 {
			av = float64(a.LatencyMs)
		}
		if b.Attempts == 0 {
			bv = float64(b.LatencyMs)
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
		if a.JitterMs < b.JitterMs {
			return -1
		}
		if a.JitterMs > b.JitterMs {
			return 1
		}
		return strings.Compare(a.Target, b.Target)
	})
}

var sniJobServer ServerService

func runSNIJob(ctx context.Context, j *sniJob) error {
	svc := &sniJobServer
	ctx = context.WithValue(ctx, sniProgressKey{}, sniProgress(j.publish))
	var rows []*RealityScanResult
	var err error
	req := j.request
	switch req.Mode {
	case "nearby":
		rows, err = svc.ScanNearbySNIs(ctx, req.Targets)
	case "candidates":
		rows, err = svc.ScanRealityTargetsContext(ctx, req.Targets)
	case "combined":
		rows, err = svc.ScanNearbySNIs(ctx, req.Targets)
		// Lack of a public-IP answer must not prevent configured candidates.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		candidates, candidateErr := svc.ScanRealityTargetsContext(ctx, "")
		if candidateErr != nil {
			return candidateErr
		}
		if err != nil {
			j.mu.Lock()
			j.view.Error = "neighbor discovery: " + err.Error()
			j.mu.Unlock()
		}
		rows = dedupRealityResults(append(rows, candidates...))
		err = nil
	case "recheck":
		emitSNIProgress(ctx, len(req.Recheck), 0, nil)
		for _, saved := range req.Recheck {
			r := svc.probeRealityAddrContext(ctx, saved.IP, saved.Port, saved.Host, realityDiscoverTimeout, 0, false)
			r.Source = "saved"
			rows = append(rows, r)
			emitSNIProgress(ctx, 0, 1, r)
		}
	}
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for i, row := range rows {
		rows[i] = pinSNIJobResult(row)
	}
	rows = dedupSNIJobResults(rows)
	rankSNIResults(rows, req.Purpose)
	j.mu.Lock()
	if len(rows) > 100 {
		rows = rows[:100]
	}
	j.view.Results = rows
	j.view.Phase = "verifying"
	j.mu.Unlock()
	var finalists []*RealityScanResult
	for _, r := range rows {
		if sniUsable(r, req.Purpose) {
			finalists = append(finalists, r)
			if len(finalists) == 10 {
				break
			}
		}
	}
	j.mu.Lock()
	j.view.VerifyTotal = len(finalists)
	j.mu.Unlock()
	// Stability checks are sequential per host and bounded across hosts. The
	// global probe limiter is shared with legacy scanners as well.
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, r := range finalists {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		}
		wg.Add(1)
		go func(row *RealityScanResult) {
			defer wg.Done()
			defer func() { <-sem }()
			updated := verifySNICandidate(ctx, svc, row, req.Purpose)
			j.mu.Lock()
			for i, existing := range j.view.Results {
				if existing.Target == row.Target && existing.Host == row.Host {
					j.view.Results[i] = updated
					break
				}
			}
			j.view.Verified++
			rankSNIResults(j.view.Results, req.Purpose)
			j.mu.Unlock()
		}(r)
	}
	wg.Wait()
	return ctx.Err()
}
func verifySNICandidate(ctx context.Context, svc *ServerService, row *RealityScanResult, purpose string) *RealityScanResult {
	return measureSNICandidate(ctx, row, purpose, func(probeCtx context.Context, ip string, port int, sni string) *RealityScanResult {
		return svc.probeRealityAddrContext(probeCtx, ip, port, sni, realityDiscoverTimeout, 0, false)
	}, net.DefaultResolver.LookupIPAddr)
}

func measureSNICandidate(ctx context.Context, row *RealityScanResult, purpose string, probe func(context.Context, string, int, string) *RealityScanResult, lookup func(context.Context, string) ([]net.IPAddr, error)) *RealityScanResult {
	out := *row
	var samples []float64
	out.Attempts = 5
	out.Successes = 0
	for i := 0; i < 5; i++ {
		if ctx.Err() != nil {
			break
		}
		probeCtx := ctx
		if i == 4 {
			probeCtx = context.WithValue(ctx, sniHTTPCheckKey{}, true)
		}
		r := probe(probeCtx, row.IP, row.Port, row.Host)
		if sniUsable(r, purpose) {
			out.Successes++
			samples = append(samples, r.TCPMs+r.TLSMs)
			out.TCPMs = r.TCPMs
			out.TLSMs = r.TLSMs
			if r.HTTP2OK != nil {
				out.HTTP2OK = r.HTTP2OK
			}
		}
	}
	slices.Sort(samples)
	if len(samples) > 0 {
		out.MedianMs = samples[len(samples)/2]
		if len(samples)%2 == 0 {
			out.MedianMs = (samples[len(samples)/2-1] + samples[len(samples)/2]) / 2
		}
		out.JitterMs = samples[len(samples)-1] - samples[0]
		out.LatencyMs = int(out.MedianMs)
	} else {
		out.LatencyMs = 0
		out.MedianMs = 0
		out.JitterMs = 0
	}
	out.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	dnsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ips, err := lookup(dnsCtx, row.Host)
	if err != nil {
		out.DNSLookupError = err.Error()
	} else {
		match := false
		for _, ip := range ips {
			if ip.IP.Equal(net.ParseIP(row.IP)) {
				match = true
				break
			}
		}
		out.DNSMatchesIP = &match
	}
	return &out
}

var sniHistoryMu sync.Mutex

const sniHistoryKey = "sniScanHistory"

func loadSNIHistory() ([]SNIJobSnapshot, error) {
	var setting model.Setting
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("database not initialized")
	}
	err := db.Where("key = ?", sniHistoryKey).First(&setting).Error
	if database.IsNotFound(err) {
		return []SNIJobSnapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	var history []SNIJobSnapshot
	if err := json.Unmarshal([]byte(setting.Value), &history); err != nil {
		return nil, fmt.Errorf("decode SNI history: %w", err)
	}
	return history, nil
}
func saveSNIHistory(view SNIJobSnapshot) error {
	sniHistoryMu.Lock()
	defer sniHistoryMu.Unlock()
	history, err := loadSNIHistory()
	if err != nil {
		return err
	}
	saved := make([]*RealityScanResult, 0, 20)
	for _, row := range view.Results {
		if row != nil && row.CertValid && net.ParseIP(row.IP) != nil && row.Host != "" {
			saved = append(saved, row)
			if len(saved) == 20 {
				break
			}
		}
	}
	view.Results = saved
	history = append([]SNIJobSnapshot{view}, history...)
	if len(history) > 20 {
		history = history[:20]
	}
	data, err := json.Marshal(history)
	if err != nil {
		return err
	}
	return (&SettingService{}).saveSetting(sniHistoryKey, string(data))
}

func StartSNIScan(request SNIScanRequest) (SNIJobSnapshot, error)  { return sniJobs().start(request) }
func SNIScanStatus(id string, cancel bool) (SNIJobSnapshot, error) { return sniJobs().get(id, cancel) }
func SNIScanHistory() ([]SNIJobSnapshot, error) {
	sniHistoryMu.Lock()
	defer sniHistoryMu.Unlock()
	return loadSNIHistory()
}
