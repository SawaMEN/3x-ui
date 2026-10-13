#!/usr/bin/env python3
"""Run management failures against stubs, never host services or installers."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


def function(file, name):
    source = (ROOT / file).read_text()
    start = source.index(name + "() {")
    line_end = source.index("\n", start)
    if source[line_end - 1] == "}":
        return source[start:line_end] + "\n"
    return source[start:source.index("\n}\n", start) + 3]


class ManagementTests(unittest.TestCase):
    def run_script(self, script, env=None):
        result = subprocess.run(["bash", "-c", script], capture_output=True,
                                text=True, env=dict(os.environ, **(env or {})))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

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
                        (base / "x-ui.service").write_text("[Service]\\n")
                    log_path = base / "systemctl.log"
                    self.run_script(function("install.sh", "_stop_existing_xui_for_install") + r'''
release=arch
xui_service="$TEST_ROOT"
systemctl() {
    printf '%s\\n' "$*" >> "$TEST_LOG"
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
