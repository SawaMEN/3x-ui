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
	script := "set -euo pipefail\n" + strings.TrimSuffix(string(installerAdapter), "main \"$@\"\n") + `
need_root() { :; }
install_packages() { :; }
managed_toolchain() { :; }
git() { :; }
cp() { :; }
ensure_build_memory() { :; }
managed_sources() { mkdir -p "$3/cmd/olcrtc-manager/web/dist"; touch "$3/cmd/olcrtc-manager/web/dist/index.html"; }
mktemp() { command mktemp -d "$TEST_ROOT/work.XXXXXX"; }
go() { case "$PWD" in */panel) return 64;; esac; }
install() { echo unexpected-deployment; return 99; }
OLCRTC_REPO=server
OLCRTC_REF=master
PANEL_REPO=manager
PANEL_REF=main
CONFIG_DIR="$TEST_ROOT/config"
MANAGER_PATCH_PATH=unused
MANAGER_SAFETY_PATH=unused
main
`
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

func TestMaintenanceRejectsClientOperations(t *testing.T) {
	operationMu.Lock()
	defer operationMu.Unlock()
	if _, err := Clients(); err == nil {
		t.Fatal("read allowed during deployment")
	}
	if _, err := CreateClient(context.Background(), ClientInput{}); err == nil {
		t.Fatal("mutation allowed during deployment")
	}
}

func TestDeploymentRollbackAndRecovery(t *testing.T) {
	for _, failure := range []string{"health", "deploy", "crash", "success"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "config")
			bin := filepath.Join(root, "bin")
			unit := filepath.Join(root, "unit")
			for _, p := range []string{config, bin, unit} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			files := []string{bin + "/olcrtc", bin + "/olcrtc-manager", config + "/config.json", config + "/panel.env", config + "/.3x-ui-revisions", unit + "/manager.service"}
			for i, p := range files {
				if err := os.WriteFile(p, []byte(fmt.Sprintf("old-%d", i)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			definitions := strings.TrimSuffix(string(installerAdapter), "main \"$@\"\n")
			mocks := `
need_root() { :; }
install_packages() { :; }
managed_toolchain() { :; }
ensure_build_memory() { :; }
managed_sources() { mkdir -p "$3/cmd/olcrtc-manager/web/dist"; touch "$3/cmd/olcrtc-manager/web/dist/index.html"; }
git() { :; }
go() { while [ "$#" -gt 0 ]; do if [ "$1" = '-o' ]; then printf 'new-binary' > "$2"; return; fi; shift; done; }
systemctl() { printf '%s\n' "$*" >> "$TEST_ROOT/systemctl.log"; }
detect_manager_features() { :; }
write_config_if_missing() { printf 'new-config' > "$CONFIG_PATH"; }
apply_config_port() { :; }
write_panel_env_if_missing() { printf 'new-env' > "$PANEL_ENV_PATH"; }
install_service() { printf 'new-unit' > "$MANAGED_SERVICE_PATH"; }
install() { if [ "$FAILURE" = deploy ] && [ "${@: -1}" = "$MANAGED_BIN_DIR/olcrtc-manager.new" ]; then return 64; fi; command install "$@"; }
manager_ready() { case "$FAILURE" in health) return 1;; crash) kill -KILL $$;; *) return 0;; esac; }
`
			env := append(os.Environ(), "TEST_ROOT="+root, "FAILURE="+failure, "CONFIG_DIR="+config, "CONFIG_PATH="+config+"/config.json", "PANEL_ENV_PATH="+config+"/panel.env", "MANAGED_BIN_DIR="+bin, "MANAGED_SERVICE_PATH="+files[5], "BUILT_REVISIONS_PATH="+root+"/built", "MANAGER_PATCH_PATH="+root+"/patch", "MANAGER_SAFETY_PATH="+root+"/safety", "PANEL_REF="+managerRevision, "OLCRTC_REF="+serverRevision, "PANEL_REPO=manager", "OLCRTC_REPO=server", "MANAGER_PATCH_VERSION="+managerPatchVersion)
			_ = os.WriteFile(root+"/safety", []byte("package main"), 0600)
			cmd := exec.Command("bash")
			cmd.Env = env
			cmd.Stdin = strings.NewReader("set -euo pipefail\n" + definitions + mocks + "\nmain\n")
			out, err := cmd.CombinedOutput()
			if failure == "success" {
				if err != nil {
					t.Fatalf("install: %v: %s", err, out)
				}
			} else if err == nil {
				t.Fatal("failure ignored")
			}
			if failure == "crash" {
				// Simulate restarting the panel after SIGKILL/power loss: recovery uses the
				// durable snapshot, not local variables in the terminated process.
				cmd = exec.Command("bash")
				cmd.Env = env
				cmd.Stdin = strings.NewReader("set -euo pipefail\n" + definitions + mocks + "\ntransaction=\"$CONFIG_DIR/.3x-ui-transaction\"\nrecover_install\n")
				if out, err = cmd.CombinedOutput(); err != nil {
					t.Fatalf("recovery: %v: %s", err, out)
				}
			}
			if failure != "success" {
				for i, p := range files {
					b, err := os.ReadFile(p)
					if err != nil || string(b) != fmt.Sprintf("old-%d", i) {
						t.Fatalf("%s was not restored: %q, %v", p, b, err)
					}
				}
			}
			if _, err := os.Stat(config + "/.3x-ui-transaction"); !os.IsNotExist(err) {
				t.Fatal("transaction not cleaned")
			}
		})
	}
}
