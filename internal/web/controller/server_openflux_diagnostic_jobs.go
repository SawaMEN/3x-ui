package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"
)

// The panel's HTTP WriteTimeout is 30 seconds. Negotiated OpenFlux sessions
// can require longer, so diagnostics must run as bounded asynchronous jobs.
// Job status is readable only by the authenticated owner who started it.
type openFluxDiagnosticJob struct {
	RunID   string                                `json:"runId"`
	State   string                                `json:"state"`
	Result  *externalvpn.OpenFluxDiagnosticResult `json:"result,omitempty"`
	OwnerID int                                   `json:"-"`
	Until   time.Time                             `json:"-"`
}

var openFluxDiagnosticJobs = struct {
	sync.Mutex
	entries map[string]*openFluxDiagnosticJob
}{entries: map[string]*openFluxDiagnosticJob{}}

func startOpenFluxDiagnosticJob(ownerID int, inst externalvpn.Instance, typ, clientToken string) (*openFluxDiagnosticJob, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("cannot allocate diagnostic job: %w", err)
	}
	select {
	case openFluxDiagnosticSlot <- struct{}{}:
	default:
		return nil, fmt.Errorf("another OpenFlux channel diagnostic is running")
	}
	job := &openFluxDiagnosticJob{
		RunID: hex.EncodeToString(nonce[:]),
		State: "running",
		OwnerID: ownerID,
		Until: time.Now().Add(6 * time.Minute),
	}
	openFluxDiagnosticJobs.Lock()
	for id, previous := range openFluxDiagnosticJobs.entries {
		if time.Now().After(previous.Until) {
			delete(openFluxDiagnosticJobs.entries, id)
		}
	}
	openFluxDiagnosticJobs.entries[job.RunID] = job
	openFluxDiagnosticJobs.Unlock()
	go func() {
		defer func() { <-openFluxDiagnosticSlot }()
		// Independent of browser disconnect; always bounded and cleaned up.
		ctx, cancel := context.WithTimeout(context.Background(), 58*time.Second)
		defer cancel()
		result := externalvpn.DiagnoseOpenFluxChannel(ctx, inst, typ, clientToken)
		openFluxDiagnosticJobs.Lock()
		job.Result = &result
		if result.Passed {
			job.State = "success"
		} else {
			job.State = "failed"
		}
		openFluxDiagnosticJobs.Unlock()
	}()
	return &openFluxDiagnosticJob{RunID: job.RunID, State: job.State}, nil
}

func (a *ServerController) openFluxDiagnosticStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user := session.GetLoginUser(c)
	if user == nil {
		jsonObj(c, nil, fmt.Errorf("authentication required"))
		return
	}
	runID := c.Query("runId")
	openFluxDiagnosticJobs.Lock()
	job := openFluxDiagnosticJobs.entries[runID]
	if job == nil || job.OwnerID != user.Id || time.Now().After(job.Until) {
		openFluxDiagnosticJobs.Unlock()
		jsonObj(c, nil, fmt.Errorf("OpenFlux diagnostic job not found"))
		return
	}
	result := openFluxDiagnosticJob{RunID: job.RunID, State: job.State, Result: job.Result}
	openFluxDiagnosticJobs.Unlock()
	jsonObj(c, result, nil)
}
