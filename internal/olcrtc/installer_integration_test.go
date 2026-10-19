package olcrtc

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Run this against the exact downloaded script, including its global variable
// initializers. Syntax-only tests missed the inetutils hostname -I failure.
func TestPinnedInstallerInitializersWithoutHostnameI(t *testing.T) {
	path := os.Getenv("OLCRTC_PINNED_INSTALLER_PATH")
	t.Setenv("PANEL_CERT_IP", "")
	t.Setenv("PANEL_PUBLIC_HOST", "")
	if path == "" {
		t.Skip("set OLCRTC_PINNED_INSTALLER_PATH")
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	script, err := managedInstaller(original, "pacman")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-n")
	cmd.Stdin = bytes.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("syntax: %v: %s", err, out)
	}
	// Drop only the final invocation; all production global initializers run.
	end := bytes.LastIndex(script, []byte(`main "$@"`))
	if end < 0 {
		t.Fatal("main invocation missing")
	}
	prefix := "hostname() { return 64; }\nexport -f hostname\n"
	script = append([]byte(prefix), script[:end]...)
	script = append(script, []byte("printf 'initializers-ok\\n'\n")...)
	for _, production := range []bool{false, true} {
		cmd := exec.Command("bash")
		cmd.Stdin = bytes.NewReader(script)
		cmd.Env = append(os.Environ(), "PANEL_PORT=23456", "PANEL_ADMIN_PATH=/admin")
		if production {
			cmd.Env = append(cmd.Env, installerEnvironment(filepath.Join(t.TempDir(), "revisions"))...)
		}
		out, err := cmd.CombinedOutput()
		if !production {
			var exitErr *exec.ExitError
			if err == nil {
				t.Fatal("original initializer unexpectedly survived hostname exit 64")
			}
			if e, ok := err.(*exec.ExitError); ok {
				exitErr = e
			}
			if exitErr == nil || exitErr.ExitCode() != 64 {
				t.Fatalf("expected 64, got %v: %s", err, out)
			}
		} else if err != nil || !bytes.Contains(out, []byte("initializers-ok")) {
			t.Fatalf("production initializer: %v: %s", err, out)
		}
	}
}

func TestAdapterBuildFailureDoesNotDeployAndCleansWorkspace(t *testing.T) {
	root := t.TempDir()
	// Host commands are replaced with shell functions: this exercises the real
	// adapter's ordering and EXIT trap without installing anything on the host.
	script := `set -euo pipefail
need_root() { :; }
install_packages() { :; }
install_go() { :; }
ensure_build_memory() { :; }
clone_repo() { mkdir -p "$3/cmd/olcrtc-manager/web/dist"; touch "$3/cmd/olcrtc-manager/web/dist/index.html"; }
mktemp() { command mktemp -d "$TEST_ROOT/work.XXXXXX"; }
go() { case "$PWD" in */panel) return 64;; esac; }
install() { echo unexpected-deployment; return 99; }
OLCRTC_REPO=server
OLCRTC_REF=master
PANEL_REPO=manager
PANEL_REF=main
` + string(installerAdapter)
	cmd := exec.Command("bash")
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = append(os.Environ(), "TEST_ROOT="+root)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("build failure was ignored")
	}
	if strings.Contains(string(out), "unexpected-deployment") {
		t.Fatalf("deployed after failed build: %s", out)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatalf("workspace leaked: %v, %v", files, err)
	}
	logPath := filepath.Join(root, "log")
	if err := os.WriteFile(logPath, out, 0600); err != nil {
		t.Fatal(err)
	}
	got := installerFailure(exec.Command("bash", "-c", "exit 64").Run(), logPath).Error()
	if !strings.Contains(got, "build-manager") || !strings.Contains(got, "64") {
		t.Fatalf("bad diagnostic: %s", got)
	}
}

func TestInstallerStageRedactsArbitraryOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	for _, line := range []string{"Password: secret", "3XUI_OLCRTC_STAGE=secret"} {
		_ = os.WriteFile(path, []byte("3XUI_OLCRTC_STAGE=sources\n"+line+"\n"), 0600)
		got := installerFailure(fmt.Errorf("failed"), path).Error()
		if strings.Contains(got, "secret") || !strings.Contains(got, "sources") {
			t.Fatalf("unsafe error: %s", got)
		}
	}
}

func TestBuiltRevisionPairValidation(t *testing.T) {
	for value, want := range map[string]bool{
		strings.Repeat("a", 40) + "|" + strings.Repeat("b", 40): true,
		"unknown|unknown": false, "": false, strings.Repeat("a", 40): false,
	} {
		if got := validRevisions(value); got != want {
			t.Fatalf("%q: %v", value, got)
		}
	}
}

func TestManagerClientDoesNotForwardAuthenticationOnRedirect(t *testing.T) {
	called := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer source.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, source.URL, nil)
	req.SetBasicAuth("admin", "secret")
	res, err := managerHTTPClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if called || res.StatusCode != http.StatusFound {
		t.Fatal("manager redirect was followed")
	}
}

// Only enabled on disposable systemd CI VMs. Exercises the same install()
// called by the panel, including environment, adapter and authenticated health.
func TestManagedInstallationSmoke(t *testing.T) {
	if os.Getenv("OLCRTC_RUN_INSTALL_SMOKE") != "1" {
		t.Skip("requires disposable root systemd VM")
	}
	if yes, reason := Supported(); !yes {
		t.Fatal(reason)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	if err := install(ctx); err != nil {
		t.Fatal(err)
	}
	if !installed() || !validRevisions(readRevisions()) {
		t.Fatal("installation or built revisions missing")
	}
}
