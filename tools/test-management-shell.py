#!/usr/bin/env python3
"""Run management failures against stubs, never host services or installers."""
import os
import re
import hashlib
import runpy
import shutil
import tarfile
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


def function(file, name):
    source = (ROOT / file).read_text()
    match = re.search(r"^(?:function )?" + re.escape(name) + r"\(\) \{", source, re.M)
    if match is None:
        raise ValueError(f"Function {name} not found in {file}")
    start = match.start()
    line_end = source.index("\n", start)
    if source[line_end - 1] == "}":
        return source[start:line_end] + "\n"
    return source[start:source.index("\n}\n", start) + 3]


class ManagementTests(unittest.TestCase):
    def run_script(self, script, env=None):
        result = subprocess.run(["bash", "-c", script], capture_output=True,
                                text=True, env=dict(os.environ, **(env or {})))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_install_smoke_verifies_stable_version_and_dev_commit(self):
        source = (ROOT / "deploy/test/smoke-noninteractive.sh").read_text()
        start = source.index('        if [ -n "${XUI_SMOKE_VERSION:-}" ]; then\n            installed=')
        end = source.index("\n\n        RESULT=", start)
        check = source[start:end].replace("/usr/local/x-ui/x-ui", '"$TEST_PANEL"')
        commit = "0123456789abcdef0123456789abcdef01234567"
        for tag, installed, expected in (("v3.9.8", "3.9.8", 0),
                                         ("v3.9.8", "3.9.7", 1),
                                         ("dev-" + commit, "dev+01234567", 0),
                                         ("dev-" + commit, "dev+ffffffff", 1),
                                         ("dev-latest", "dev+01234567", 0),
                                         ("dev-latest", "3.9.8", 1)):
            with self.subTest(tag=tag, installed=installed), tempfile.TemporaryDirectory() as raw:
                panel = Path(raw) / "panel"
                panel.write_text("#!/bin/bash\nprintf '%s\\n' \"$TEST_INSTALLED\"\n")
                panel.chmod(0o700)
                result = subprocess.run(["bash", "-c", check], capture_output=True, text=True,
                                        env=dict(os.environ, XUI_SMOKE_VERSION=tag,
                                                 TEST_INSTALLED=installed, TEST_PANEL=str(panel)))
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)

    def test_post_update_stops_after_failed_database_migration(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            panel = base / "x-ui"
            panel.write_text('''#!/bin/bash
printf '%s\n' "$*" >> "$TEST_ROOT/calls"
[[ "$1" != migrate ]] || exit 23
printf 'port: 2053\nwebBasePath: /unchanged-path/\ncert: /cert/panel.pem\n'
''')
            panel.chmod(0o700)
            self.run_script(function("update.sh", "config_after_update") + r'''
xui_folder="$TEST_ROOT"
curl() { printf '127.0.0.1\n200\n'; }
config_after_update >/dev/null
[[ $? == 1 ]] || exit 1
[[ "$(wc -l < "$TEST_ROOT/calls")" == 2 ]]
''', {"TEST_ROOT": raw})

    def test_remote_script_failure_is_returned_by_all_update_commands(self):
        for name in ("install", "update", "update_dev", "legacy_version"):
            with self.subTest(command=name):
                self.run_script(function("x-ui.sh", name) + r'''
LOGE() { :; }; LOGI() { :; }; confirm() { return 0; }
before_show_menu() { :; }; start() { :; }
read() { printf -v "${@: -1}" '%s' '2.4.0'; }
run_remote_script() { return 42; }
''' + f"\n{name} 0\nrc=$?\n[[ $rc == 42 ]]\n")

    def service_functions(self):
        names = ("xui_service_action", "finish_service_action", "start", "stop",
                 "restart", "restart_xray", "status", "set_autostart", "enable", "disable")
        return "".join(function("x-ui.sh", name) for name in names)

    def test_install_returns_failure_of_followup_start(self):
        self.run_script(function("x-ui.sh", "install") + r'''
run_remote_script() { return 0; }
start() { return 37; }
install 0
[[ $? == 37 ]]
''')

    def test_service_failures_reach_cli_exit_status(self):
        for release in ("debian", "alpine"):
            for name in ("start", "stop", "restart", "restart_xray", "status", "enable", "disable"):
                with self.subTest(release=release, command=name):
                    state = 1 if name == "start" else 0
                    script = self.service_functions() + r'''
LOGE() { :; }; LOGI() { :; }; before_show_menu() { :; }
sleep() { :; }; systemctl() { return 37; }
rc-service() { return 37; }; rc-update() { return 37; }
running_in_docker=false
''' + f"\nrelease={release}\ncheck_status() {{ return {state}; }}\n{name} 0\nrc=$?\n[[ $rc == 37 ]]\n"
                    self.run_script(script)

    def test_stop_missing_service_does_not_invoke_systemctl_stop(self):
        with tempfile.TemporaryDirectory() as raw:
            self.run_script(self.service_functions() + r'''
LOGE() { :; }; LOGI() { :; }; before_show_menu() { :; }
running_in_docker=false; release=arch
check_status() { return 2; }
systemctl() { echo "$*" >> "$TEST_LOG"; return 37; }
stop 0
rc=$?
[[ "$rc" == 1 && ! -e "$TEST_LOG" ]]
''', {"TEST_LOG": str(Path(raw) / "calls.log")})

    def test_service_success_is_not_reported_as_failure(self):
        for name in ("start", "stop", "restart", "restart_xray", "status", "enable", "disable"):
            with self.subTest(command=name):
                script = self.service_functions() + r'''
LOGE() { :; }; LOGI() { :; }; before_show_menu() { :; }; sleep() { :; }
running_in_docker=false; release=debian; mock_state=1
check_status() { return "$mock_state"; }
systemctl() {
    case "$1" in start|restart) mock_state=0;; stop) mock_state=1;; esac
    return 0
}
''' + f"\n{name} 0\nrc=$?\n[[ $rc == 0 ]]\n"
                self.run_script(script)

    def test_docker_restart_checks_both_signals(self):
        self.run_script(self.service_functions() + r'''
LOGE() { :; }; LOGI() { :; }; before_show_menu() { :; }
running_in_docker=true
signal_xui() { [[ "$1" == HUP ]] || return 37; }
restart 0
[[ $? == 37 ]]
''')

    def test_settings_error_survives_local_declaration(self):
        with tempfile.TemporaryDirectory(prefix="xui space [test] ") as raw:
            executable = Path(raw) / "x-ui"
            executable.write_text("#!/bin/bash\nexit 23\n")
            executable.chmod(0o700)
            self.run_script(function("x-ui-core.sh", "check_config") + r'''
LOGE() { :; }; LOGI() { :; }; show_menu() { :; }
xui_folder="$TEST_ROOT"
check_config 0
[[ $? == 1 ]]
''', {"TEST_ROOT": raw})

    def release_functions(self, file):
        return "".join(function(file, name) for name in
                       ("release_curl", "resolve_latest_tag", "select_release_tag"))

    def test_install_and_update_share_release_selection(self):
        self.assertEqual(self.release_functions("install.sh"), self.release_functions("update.sh"))
        self.assertEqual(function("install.sh", "download_release_asset"),
                         function("update.sh", "download_release_asset"))

    def test_default_release_avoids_legacy_stable_but_explicit_tag_is_preserved(self):
        for file in ("install.sh", "update.sh"):
            for requested, stable_status, expected, rc in (
                    ("", "200", "v4.0.0", 0),
                    ("", "404", "dev-latest", 0),
                    ("v3.9.8", "404", "v3.9.8", 0),
                    ("", "503", "", 1)):
                with self.subTest(file=file, requested=requested, status=stable_status):
                    self.run_script(self.release_functions(file) + r'''
resolve_latest_tag() { printf v4.0.0; }
curl() {
    case "$*" in
        *core-migration.sh*) printf '%s' "$STATUS";;
        *api.github.com*) printf '{"body":"legacy release"}';;
        *) return 99;;
    esac
}
value=$(select_release_tag "$REQUESTED")
rc=$?
[[ "$rc" == "$EXPECTED_RC" && "$value" == "$EXPECTED" ]]
''', {"STATUS": stable_status, "REQUESTED": requested,
      "EXPECTED": expected, "EXPECTED_RC": str(rc)})

    def test_dev_selection_keeps_rolling_release(self):
        for file in ("install.sh", "update.sh"):
            self.run_script(self.release_functions(file) + r'''
arch() { printf amd64; }
curl() {
    case "$*" in
        *api.github.com*) printf '{"body":"commit=0123456789012345678901234567890123456789"}';;
        *dev-0123456789012345678901234567890123456789/x-ui-linux-amd64.tar.gz*) printf 200;;
        *) return 99;;
    esac
}
[[ "$(select_release_tag dev)" == dev-latest ]]
[[ "$(select_release_tag dev-latest)" == dev-latest ]]
''')

    def test_dev_publisher_only_updates_rolling_release(self):
        source = (ROOT / ".github/workflows/release.yml").read_text()
        block = source.split("      - name: Publish dev-latest\n", 1)[1].split("        run: |\n", 1)[1]
        script = "\n".join(line[10:] for line in block.splitlines())
        commit = "0123456789012345678901234567890123456789"
        for stale in (False, True):
            with self.subTest(stale=stale), tempfile.TemporaryDirectory() as raw:
                self.run_script(r'''
git() {
    printf 'git %s\n' "$*" >> "$TEST_LOG"
    if [[ "$1" == rev-parse ]]; then printf '%s' "$REMOTE_COMMIT"; fi
}
gh() { printf 'gh %s\n' "$*" >> "$TEST_LOG"; }
''' + script, {"COMMIT": commit, "REMOTE_COMMIT": "f" * 40 if stale else commit,
                 "TEST_LOG": str(Path(raw) / "calls")})
                calls = (Path(raw) / "calls").read_text()
                self.assertNotIn("dev-" + commit, calls)
                if stale:
                    self.assertNotIn("gh release", calls)
                    self.assertNotIn("git tag", calls)
                else:
                    self.assertIn("gh release upload dev-latest ", calls)
                    self.assertNotIn("gh release create", calls)

    def test_failed_transport_retries_ipv4_without_partial_stdout(self):
        for file in ("install.sh", "update.sh"):
            self.run_script(function(file, "release_curl") + r'''
curl() {
    if [[ "$1" == -4 ]]; then printf success; else printf partial; return 7; fi
}
[[ "$(release_curl -fsSL test)" == success ]]
''')

    def test_port_detection_does_not_override_match_in_awk_end(self):
        for file in ("install.sh", "x-ui.sh", "x-ui-core.sh"):
            self.run_script(function(file, "is_port_in_use") + r'''
ss() { printf 'LISTEN 0 128 127.0.0.1:2053 0.0.0.0:*\n'; }
is_port_in_use 2053 || exit 1
is_port_in_use 2054 && exit 1
exit 0
''')

    def test_update_exit_status_includes_rollback_failure(self):
        for initial, rollback, expected in ((0, 0, 0), (0, 1, 1), (42, 0, 42), (42, 1, 42)):
            script = function("update.sh", "_report_update_exit") + f'''
core_migration_exit() {{ return {rollback}; }}
_write_update_status() {{ :; }}
trap _report_update_exit EXIT
exit {initial}
'''
            result = subprocess.run(["bash", "-c", script], capture_output=True, text=True)
            self.assertEqual(result.returncode, expected, result.stderr)

    def test_console_menu_and_usage_have_aligned_borders(self):
        for name in ("show_menu", "show_usage"):
            with self.subTest(command=name):
                script = function("x-ui.sh", "print_console_box") + function("x-ui.sh", name)
                script += "\nshow_status() { :; }\n" + name + " </dev/null\n"
                result = subprocess.run(["bash", "-c", script], text=True, capture_output=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                lines = [line for line in result.stdout.splitlines() if line.startswith(("┌", "│", "├", "└"))]
                self.assertGreater(len(lines), 20)
                self.assertEqual(len({len(line) for line in lines}), 1, "\n".join(lines))

    def test_openrc_autostart_matches_exact_service(self):
        for service, expected in (("x-ui", 0), ("other-x-ui", 1), ("x-ui-test", 1)):
            with self.subTest(service=service):
                script = function("x-ui.sh", "check_enabled") + "\n"
                script += f"running_in_docker=false; release=alpine\nrc-update() {{ printf '{service} | default\\n'; }}\ncheck_enabled\n"
                result = subprocess.run(["bash", "-c", script], text=True, capture_output=True)
                self.assertEqual(result.returncode, expected, result.stderr)

    def test_main_menu_returns_on_eof_and_nested_navigation(self):
        script = function("x-ui.sh", "print_console_box") + function("x-ui.sh", "show_menu") + r'''
show_status() { :; }
show_menu </dev/null >/dev/null || exit 1
_xui_menu_active=1
read() { return 99; }
show_menu || exit 1
'''
        self.run_script(script)

    def test_asset_download_keeps_http_failure_for_diagnostics(self):
        for file in ("install.sh", "update.sh"):
            with tempfile.TemporaryDirectory() as raw:
                self.run_script(function(file, "download_release_asset") + r'''
curl() { printf 404; return 22; }
download_release_asset https://example.invalid/missing "$TEST_ROOT/archive" >/dev/null
rc=$?
[[ "$rc" == 22 && "$download_http_status" == 404 ]]
''', {"TEST_ROOT": raw})

    def test_modern_release_cannot_skip_missing_checksum(self):
        with tempfile.TemporaryDirectory() as raw:
            self.run_script(function("install.sh", "verify_release_checksum") + r'''
release_curl() { printf 404; }
(verify_release_checksum https://github.com/SawaMEN/3x-ui/releases/download/dev-latest/archive "$TEST_ROOT/archive") >/dev/null
[[ $? == 1 ]]
''', {"TEST_ROOT": raw})

    def test_unsupported_architecture_is_failure_without_deleting_script(self):
        for file in ("install.sh", "update.sh"):
            self.run_script(function(file, "arch") + r'''
uname() { printf mips; }
rm() { exit 99; }
arch >/dev/null 2>&1
[[ $? == 1 ]]
''')

    def test_environment_loader_preserves_explicit_update_settings(self):
        self.assertEqual(function("install.sh", "load_xui_env"), function("update.sh", "load_xui_env"))
        with tempfile.TemporaryDirectory() as raw:
            Path(raw, "env").write_text('XUI_UPDATE_TAG=stable\nXUI_DB_FOLDER=database\nXUI_BIN_FOLDER=custom-bin\n')
            for file in ("install.sh", "update.sh"):
                self.run_script(function(file, "load_xui_env") + r'''
xui_env_file_path() { printf '%s/env' "$TEST_ROOT"; }
export XUI_UPDATE_TAG=dev-latest
unset XUI_DB_FOLDER XUI_BIN_FOLDER
load_xui_env
[[ "$XUI_UPDATE_TAG" == dev-latest && "$XUI_DB_FOLDER" == database && "$XUI_BIN_FOLDER" == custom-bin ]]
[[ "$-" != *a* ]]
set -a
load_xui_env
[[ "$-" == *a* ]]
''', {"TEST_ROOT": raw})

    def test_service_recovery_is_armed_before_stop(self):
        install = (ROOT / "install.sh").read_text()
        update = function("update.sh", "update_x-ui")
        self.assertLess(install.index('|| cm_stopped=1'), install.index('_stop_existing_xui_for_install || exit 1'))
        self.assertLess(update.index('        cm_stopped=1'), update.index('rc-service x-ui stop > /dev/null'))
        for source in (install, update):
            self.assertLess(source.index('prepare_release_host_files "${script_ref}"'), source.index('core_migration_begin ||'))

    def host_file_functions(self, file):
        return "".join(function(file, name) for name in
                       ("rewrite_release_service_paths", "prepare_release_host_files"))

    def test_both_installers_share_host_file_preflight(self):
        self.assertEqual(self.host_file_functions("install.sh"), self.host_file_functions("update.sh"))

    def test_host_files_are_prepared_before_service_stop(self):
        for release, unit in (("debian", "x-ui.service.debian"), ("arch", "x-ui.service.arch"),
                              ("rocky", "x-ui.service.rhel"), ("alpine", "x-ui.rc")):
            for bundled in (True, False):
                with self.subTest(release=release, bundled=bundled), tempfile.TemporaryDirectory() as raw:
                    base = Path(raw)
                    stage = base / "stage" / "x-ui"
                    stage.mkdir(parents=True)
                    for file in ("x-ui.sh", "x-ui-core.sh"):
                        (stage / file).write_text("#!/bin/bash\nexit 0\n")
                    payload = (ROOT / unit).read_text()
                    (base / "source").write_text(payload)
                    if bundled: (stage / unit).write_text(payload)
                    self.run_script(self.host_file_functions("update.sh") + r'''
cm_stage="$TEST_ROOT/stage"
xui_folder=/usr/local/x-ui
systemctl() { exit 99; }; rc-service() { exit 99; }
download_release_asset() { cp "$TEST_ROOT/source" "$2"; touch "$TEST_ROOT/downloaded"; }
prepare_release_host_files test-ref
''', {"TEST_ROOT": raw, "release": release})
                    self.assertEqual((stage / unit).read_text(), payload)
                    self.assertEqual((base / "downloaded").exists(), not bundled)

    def test_invalid_release_menu_fails_without_stopping_or_downloading(self):
        with tempfile.TemporaryDirectory() as raw:
            stage = Path(raw) / "stage" / "x-ui"
            stage.mkdir(parents=True)
            (stage / "x-ui.sh").write_text("if then\n")
            self.run_script(self.host_file_functions("install.sh") + r'''
cm_stage="$TEST_ROOT/stage"
systemctl() { exit 99; }; download_release_asset() { exit 99; }
prepare_release_host_files test-ref >/dev/null 2>&1
[[ $? == 1 ]]
''', {"TEST_ROOT": raw})

    def test_custom_install_path_is_applied_to_systemd_and_openrc(self):
        with tempfile.TemporaryDirectory() as raw:
            base = Path(raw)
            install = base / 'install space $cash % & "quote"' / 'x-ui'
            install.mkdir(parents=True)
            for kind, unit in (("systemd", "x-ui.service.debian"), ("openrc", "x-ui.rc")):
                with self.subTest(kind=kind):
                    target = base / unit
                    target.write_text((ROOT / unit).read_text())
                    self.run_script(function("install.sh", "rewrite_release_service_paths") + r'''
xui_folder="$TEST_INSTALL"
rewrite_release_service_paths "$TEST_FILE" "$TEST_KIND" || exit 1
if [[ "$TEST_KIND" == openrc ]]; then
    sh -n "$TEST_FILE" || exit 1
    source "$TEST_FILE"
    [[ "$command" == "$TEST_INSTALL/x-ui" ]] || exit 1
    setup_swap() { :; }
    start_pre || exit 1
    [[ "$PWD" == "$TEST_INSTALL" ]] || exit 1
fi
''', {"TEST_INSTALL": str(install), "TEST_FILE": str(target), "TEST_KIND": kind})
                    if kind == 'systemd':
                        escaped = str(install).replace('\\', '\\\\').replace('"', '\\"').replace('%', '%%')
                        self.assertIn('WorkingDirectory=' + str(install).replace('%', '%%') + '/', target.read_text())
                        self.assertIn('ExecStart=/usr/bin/env -- "' + escaped.replace('$', '$$') + '/x-ui"', target.read_text())
                        if shutil.which("systemd-analyze"):
                            validation = base / "validation.service"
                            validation.write_text(target.read_text())
                            result = subprocess.run(["systemd-analyze", "verify", str(validation)], capture_output=True, text=True)
                            self.assertEqual(result.returncode, 0, result.stderr)

    def test_full_update_success_and_failed_start_with_sandboxed_services(self):
        panel = runpy.run_path(str(ROOT / "tools/test-core-migration-shell.py"))["PANEL"]
        panel = panel.replace('set -eu\n', 'set -eu\nif [[ "$1" == -v ]]; then echo test-version; exit 0; fi\n', 1)
        for failure in (False, True):
            with self.subTest(failed_start=failure), tempfile.TemporaryDirectory() as raw:
                base = Path(raw)
                old = base / "x-ui"
                (old / "bin").mkdir(parents=True)
                (old / "x-ui").write_text(panel + "\n# original\n")
                (old / "x-ui").chmod(0o700)
                (base / "database").write_text("original-data")
                (base / "host-cli").write_text("original-menu")
                (base / "service").mkdir()
                (base / "service/x-ui.service").write_text("original-service")
                (base / "running").touch()
                bundle = base / "bundle/x-ui"
                (bundle / "bin").mkdir(parents=True)
                (bundle / "x-ui").write_text(panel + "\n# upgraded\n")
                (bundle / "x-ui").chmod(0o700)
                (bundle / "bin/hiddify-core-linux-amd64").write_text("#!/bin/bash\nexit 1\n")
                (bundle / "bin/hiddify-core-linux-amd64").chmod(0o700)
                for name in ("x-ui.sh", "x-ui-core.sh"):
                    (bundle / name).write_text("#!/bin/bash\n# upgraded-menu\n")
                (bundle / "x-ui.service.debian").write_text((ROOT / "x-ui.service.debian").read_text())
                (bundle / "core-migration.sh").write_text((ROOT / "core-migration.sh").read_text() + r'''
core_migration_host_paths() { cm_host_paths=("$TEST_ROOT/host-cli" "$TEST_ROOT/service/x-ui.service"); }
core_migration_resume_gateway() { :; }
''')
                archive = base / "release.tar.gz"
                with tarfile.open(archive, "w:gz") as target: target.add(bundle, arcname="x-ui")
                script = "".join(function("update.sh", name) for name in
                    ("rewrite_release_service_paths", "prepare_release_host_files", "_install_xui_service_unit", "update_x-ui")).replace("/usr/bin/x-ui", "${TEST_ROOT}/host-cli") + r'''
_fail() { echo "$*" >&2; exit 2; }
load_xui_env() { :; }
select_release_tag() { printf dev-latest; }
arch() { printf amd64; }
download_release_asset() { command cp "$ARCHIVE" "$2"; }
curl() {
    local dest=""
    while (( $# )); do
        if [[ "$1" == -o ]]; then dest="$2"; shift; fi
        shift
    done
    printf '%s  release.tar.gz\n' "$TEST_SHA" > "$dest"
    printf 200
}
mkdir() { [[ "${@: -1}" == /var/log/x-ui ]] || command mkdir "$@"; }
chown() { :; }; pkill() { :; }; sleep() { :; }; setup_fail2ban() { :; }
systemctl() {
    printf '%s\n' "$*" >> "$TEST_LOG"
    case "$1" in
        is-active) [[ -e "$TEST_ROOT/running" ]];;
        is-enabled) return 0;;
        show) printf loaded;;
        stop) command rm -f "$TEST_ROOT/running";;
        start)
            # Only the new release fails; rollback must start the original.
            if [[ "$FAIL_START" == 1 ]] && grep -q '# upgraded' "$TEST_ROOT/x-ui/x-ui"; then return 37; fi
            touch "$TEST_ROOT/running";;
        *) return 0;;
    esac
}
release=debian
curl_bin=curl
xui_folder="$TEST_ROOT/x-ui"
xui_service="$TEST_ROOT/service"
trap 'code=$?; if declare -F core_migration_exit >/dev/null; then core_migration_exit "$code" || code=1; fi; exit "$code"' EXIT
update_x-ui
'''
                env = dict(os.environ, TEST_ROOT=raw, ARCHIVE=str(archive), TEST_DB=str(base / "database"),
                           TEST_LOG=str(base / "calls"), TEST_SHA=hashlib.sha256(archive.read_bytes()).hexdigest(),
                           XUI_MAIN_FOLDER=str(old), XUI_SERVICE=str(base / "service"),
                           XUI_DB_FOLDER=str(base / "db-dir"), FAIL_START=str(int(failure)))
                result = subprocess.run(["bash", "-c", script], env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 2 if failure else 0, result.stdout + result.stderr)
                self.assertEqual((base / "database").read_text(), "original-data" if failure else "new-core\n")
                self.assertTrue((base / "running").exists())
                if failure:
                    self.assertEqual((old / "x-ui").read_text(), panel + "\n# original\n")
                    self.assertEqual((base / "host-cli").read_text(), "original-menu")
                    self.assertEqual((base / "service/x-ui.service").read_text(), "original-service")
                else:
                    self.assertIn(str(old), (base / "service/x-ui.service").read_text())
                    self.assertIn('upgraded-menu', (base / "host-cli").read_text())

    def test_installer_aborts_if_parent_directory_is_missing(self):
        with tempfile.TemporaryDirectory() as raw:
            self.run_script(function("install.sh", "install_x-ui") + r'''
xui_folder="$TEST_ROOT/missing/x-ui"
curl() { touch "$TEST_ROOT/download-started"; return 1; }
install_x-ui
rc=$?
[[ $rc == 1 && ! -e "$TEST_ROOT/download-started" ]]
''', {"TEST_ROOT": raw})

    def test_installer_repairs_leftover_folder_without_systemd_unit(self):
        # A failed first install may leave /usr/local/x-ui but no x-ui.service.
        # Reinstall must skip the nonexistent service instead of failing on stop.
        cases = (
            # unit_file, loaded, active, stop_success, binary_running, rc, stops
            (False, False, False, True, False, 0, 0),
            (True, False, False, True, False, 0, 0),
            (True, True, False, True, False, 0, 0),
            (True, True, True, True, False, 0, 1),
            (True, True, True, False, False, 1, 1),
            (False, False, False, True, True, 1, 0),
        )
        for unit_file, loaded, active, stop_success, running, expected_rc, expected_stop in cases:
            with self.subTest(unit_file=unit_file, loaded=loaded, active=active,
                              stop_success=stop_success, running=running):
                with tempfile.TemporaryDirectory() as raw:
                    base = Path(raw)
                    if unit_file:
                        (base / "x-ui.service").write_text("[Service]\n")
                    log_path = base / "systemctl.log"
                    self.run_script(function("install.sh", "_stop_existing_xui_for_install") + r'''
release=arch
xui_service="$TEST_ROOT"
systemctl() {
    printf '%s\n' "$*" >> "$TEST_LOG"
    case "$1" in
        cat) [[ "$MOCK_LOADED" == 1 ]];;
        is-active) [[ "$MOCK_ACTIVE" == 1 ]];;
        stop) [[ "$MOCK_STOP_SUCCESS" == 1 ]];;
        *) return 0;;
    esac
}
_xui_binary_running() { [[ "$MOCK_BINARY_RUNNING" == 1 ]]; }
_stop_existing_xui_for_install
rc=$?
stops=$(grep -c '^stop ' "$TEST_LOG" || true)
[[ "$rc" == "$EXPECTED_RC" && "$stops" == "$EXPECTED_STOPS" ]]
''', {
                        "TEST_ROOT": raw,
                        "TEST_LOG": str(log_path),
                        "MOCK_LOADED": str(int(loaded)),
                        "MOCK_ACTIVE": str(int(active)),
                        "MOCK_STOP_SUCCESS": str(int(stop_success)),
                        "MOCK_BINARY_RUNNING": str(int(running)),
                        "EXPECTED_RC": str(expected_rc),
                        "EXPECTED_STOPS": str(expected_stop),
                    })

    def test_installer_forwards_version_only_when_provided(self):
        source = (ROOT / "install.sh").read_text()
        marker = '\necho -e "${green}Running...${plain}"\n'
        self.assertIn(marker, source)
        dispatch = source.rsplit(marker, 1)[1]
        for args, expected in (((), "0:"), (("v3.9.8",), "1:v3.9.8"),
                               (("dev",), "1:dev")):
            with self.subTest(args=args):
                script = r'''
install_base() { :; }
install_x-ui() { printf '%s:%s' "$#" "${1:-}"; }
''' + dispatch
                result = subprocess.run(["bash", "-c", script, "install.sh", *args],
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, expected)

    def test_uninstall_rejects_root_and_unrelated_directories(self):
        for path in ("/", "/usr", "relative/x-ui", "/app"):
            with self.subTest(path=path), tempfile.TemporaryDirectory() as raw:
                self.run_script(function("x-ui-core.sh", "uninstall") + r'''
LOGE() { :; }
confirm() { touch "$TEST_ROOT/asked-confirmation"; return 0; }
rm() { touch "$TEST_ROOT/removal-started"; }
uninstall 0
rc=$?
[[ $rc == 1 && ! -e "$TEST_ROOT/removal-started" && ! -e "$TEST_ROOT/asked-confirmation" ]]
''', {"TEST_ROOT": raw, "xui_folder": path})


if __name__ == "__main__":
    unittest.main()
