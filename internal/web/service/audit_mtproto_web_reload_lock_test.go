package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// Use a child test process so a regression in the non-reentrant mutex cannot
// strand the rest of the package tests or require forcibly unlocking a live lock.
func TestMtprotoWebReconcileReloadDoesNotDeadlock(t *testing.T) {
	const childEnv = "XUI_TEST_WEB_RECONCILE_LOCK_CHILD"
	if os.Getenv(childEnv) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMtprotoWebReconcileReloadDoesNotDeadlock$", "-test.count=1")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		output, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("WEB reconcile deadlocked while updating users: %v\n%s", ctx.Err(), output)
		}
		if err != nil {
			t.Fatalf("WEB reconcile child failed: %v\n%s", err, output)
		}
		return
	}

	setupConflictDB(t)
	settings := `{"webProxy":{"enabled":true,"domain":"web.example.com"},"clients":[{"email":"alice","enable":true,"secret":"00112233445566778899aabbccddeeff"}]}`
	seedInboundConflict(t, "web-reload-lock", "", 443, model.MTProto, "", settings)
	ib := loadInboundByTag(t, "web-reload-lock")
	opts, err := mtprotoWebForInbound(ib)
	if err != nil {
		t.Fatal(err)
	}
	opts.Site = &TelemtWebSiteOptions{Mode: "auto"}
	normalized, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{Frontend: opts.Frontend, Carrier: opts.Carrier, Carriers: opts.Carriers, Site: opts.Site, BasePath: &opts.BasePath, Aliases: &opts.Aliases, RetryJitter: opts.RetryJitter})
	if err != nil {
		t.Fatal(err)
	}
	state := TelemtWebProxyState{
		Enabled: true, ManagedInboundID: ib.Id, Domain: opts.Domain,
		BasePath: *normalized.BasePath, Aliases: *normalized.Aliases,
		Frontend: normalized.Frontend, Carrier: normalized.Carrier,
		Carriers: normalized.Carriers, Site: normalized.Site,
		UserFingerprint: "previous-user-list",
	}
	configRead := false
	err = reconcileMtprotoWebProxy(context.Background(), func() (TelemtWebProxyState, error) { return state, nil }, func() (TelemtConfig, error) {
		configRead = true
		// Invalid port makes rendering fail before host files, systemctl, API
		// or certificate operations. It still traverses the real reload lock.
		return TelemtConfig{}, nil
	})
	if !configRead || err == nil || !strings.Contains(err.Error(), "invalid port") {
		t.Fatalf("did not return from locked user reload: configRead=%v err=%v", configRead, err)
	}
	if !telemtWebOperationMu.TryLock() {
		t.Fatal("operation lock leaked")
	}
	telemtWebOperationMu.Unlock()
	if !mtprotoWebSyncMu.TryLock() {
		t.Fatal("reconcile lock leaked")
	}
	mtprotoWebSyncMu.Unlock()
}

func TestMtprotoWebPublicReloadSerializesWriters(t *testing.T) {
	telemtWebOperationMu.Lock()
	held := true
	defer func() {
		if held {
			telemtWebOperationMu.Unlock()
		}
	}()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- (TelemtService{}).SaveWebUsersHotReload(TelemtConfig{})
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("reload bypassed operation lock: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	telemtWebOperationMu.Unlock()
	held = false
	select {
	case err := <-done:
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unexpected validation result: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reload remained blocked after operation lock was released")
	}
}

func TestMtprotoWebCanceledReconcileDoesNotReadOrWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	readState := func() (TelemtWebProxyState, error) {
		t.Error("canceled reconcile read state")
		return TelemtWebProxyState{}, nil
	}
	getConfig := func() (TelemtConfig, error) {
		t.Error("canceled reconcile read config")
		return TelemtConfig{}, nil
	}
	if err := reconcileMtprotoWebProxy(ctx, readState, getConfig); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reconcile returned %v", err)
	}
	if err := (TelemtService{}).saveWebUsersHotReloadLocked(ctx, TelemtConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reload returned %v", err)
	}
}
