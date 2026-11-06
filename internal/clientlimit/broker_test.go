package clientlimit

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit/brokerclient"
)

func brokerConfig(t *testing.T, s *Runtime) brokerclient.Config {
	t.Helper()
	env := s.Environment([]string{BrokerEnvironment + "=untrusted", "KEEP=yes"})
	if len(env) != 2 || env[0] != "KEEP=yes" {
		t.Fatalf("unsanitized environment: %d entries", len(env))
	}
	path := strings.TrimPrefix(env[1], BrokerEnvironment+"=")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credential file permissions: %v %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg brokerclient.Config
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestNativeBrokerSharesExistingClientBudget(t *testing.T) {
	s := start(t, []Route{{ID: "native", Email: "alice", UploadMbps: 1, Aliases: []string{"mieru:alice", "sudoku:hash", "masque:in:alice"}}, {ID: "tcp", Email: "alice", UploadMbps: 1}, {ID: "bob", Email: "bob", Aliases: []string{"mieru:bob"}}})
	cfg := brokerConfig(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	started := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for _, alias := range []string{"mieru:alice", "sudoku:hash", "masque:in:alice"} {
		wg.Go(func() {
			client, err := brokerclient.NewWithConfig(cfg, alias)
			if err != nil {
				errs <- err
				return
			}
			defer client.Close()
			errs <- client.Wait(ctx, 0, 128<<10)
		})
	}
	c := tcp(t, s, "tcp")
	wg.Go(func() { _, err := c.Write(make([]byte, 128<<10)); errs <- err })
	bob, err := brokerclient.NewWithConfig(cfg, "mieru:bob")
	if err != nil {
		t.Fatal(err)
	}
	defer bob.Close()
	if err = bob.Wait(ctx, 0, 128<<10); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("another client shares the restricted budget: %v", elapsed)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(started); elapsed < 3*time.Second || elapsed > 8*time.Second {
		t.Fatalf("protocols bypassed shared 1 Mbps budget: %v", elapsed)
	}
}

func TestNativeBrokerRevocationAndRollback(t *testing.T) {
	s := start(t, []Route{{ID: "native", Email: "alice", UploadMbps: 1, Aliases: []string{"mieru:alice"}}})
	cfg := brokerConfig(t, s)
	client, err := brokerclient.NewWithConfig(cfg, "mieru:alice")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err = client.Wait(context.Background(), 0, 65536); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- client.Wait(context.Background(), 0, 65536) }()
	finish, err := s.Apply(Plan{})
	if err != nil {
		t.Fatal(err)
	}
	finish(false)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("revoked native lane obtained more budget")
		}
	case <-time.After(time.Second):
		t.Fatal("revocation did not interrupt budget wait")
	}
	replacement, err := brokerclient.NewWithConfig(brokerConfig(t, s), "mieru:alice")
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if err = replacement.Wait(context.Background(), 1, 1); err != nil {
		t.Fatal("rollback did not restore native access", err)
	}
	cfg.Password = "wrong"
	wrong, _ := brokerclient.NewWithConfig(cfg, "mieru:alice")
	defer wrong.Close()
	if err = wrong.Wait(context.Background(), 0, 1); err == nil {
		t.Fatal("unauthenticated budget accepted")
	}
}

func TestNativeBrokerCancellationAndDisconnect(t *testing.T) {
	s := start(t, []Route{{ID: "native", Email: "alice", UploadMbps: 1, Aliases: []string{"mieru:alice"}}})
	client, err := brokerclient.NewWithConfig(brokerConfig(t, s), "mieru:alice")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	if err = client.Wait(ctx, 0, 65536); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err = client.Wait(ctx, 0, 65536); err == nil {
		t.Fatal("cancelled budget request succeeded")
	}
	client.Close()
	if err = client.Wait(context.Background(), 0, 1); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed client: %v", err)
	}
}

func TestNativeBrokerKeepsUnlimitedUsersAvailableDuringEdits(t *testing.T) {
	s := start(t, []Route{{ID: "old", Email: "alice", UploadMbps: 1, Aliases: []string{"mieru:alice"}}})
	env := s.Environment(nil)
	path := strings.TrimPrefix(env[0], BrokerEnvironment+"=")
	finish, err := s.Apply(Plan{Routes: []Route{{ID: "new", Email: "alice", UploadMbps: 2, Aliases: []string{"mieru:alice"}}}})
	if err != nil {
		t.Fatal(err)
	}
	finish(true)
	if got := s.Environment(nil); len(got) != 1 || got[0] != env[0] {
		t.Fatal("old protocol processes lost their configuration path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg brokerclient.Config
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	bob, err := brokerclient.NewWithConfig(cfg, "mieru:bob")
	if err != nil || bob != nil {
		t.Fatalf("unlimited user interrupted by another client's edit: %v %v", bob, err)
	}
	if cfg.Users["mieru:alice"] != "new" {
		t.Fatal("existing protocol processes still use the old budget identity")
	}
}

func TestNativeBrokerCredentialFailureDoesNotKeepListener(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	s := New("127.0.0.1:0", "127.0.0.1:0")
	defer s.Close()
	_, err := s.Apply(Plan{Routes: []Route{{ID: "native", Email: "alice", UploadMbps: 1, Aliases: []string{"mieru:alice"}}}})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected credential-store failure: %v", err)
	}
	if s.Address() != "" {
		t.Fatal("failed plan retained its listener")
	}
}
