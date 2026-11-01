#!/bin/bash

# Thin compatibility layer for the interactive `x-ui` management command.
# The full historical implementation lives in x-ui-core.sh; keeping the fixes
# here makes the user-facing menu easier to audit and prevents unrelated menu
# changes from rewriting a very large shell script.

_wrapper_dir="$(cd -P "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd || pwd -P)"
_wrapper_file="${_wrapper_dir}/$(basename "${BASH_SOURCE[0]}")"
_wrapper_xui_folder="${XUI_MAIN_FOLDER:-/usr/local/x-ui}"
_xui_core=""

for _candidate in \
    "${_wrapper_dir}/x-ui-core.sh" \
    "${_wrapper_xui_folder}/x-ui-core.sh"; do
    if [[ -r "${_candidate}" ]]; then
        _xui_core="${_candidate}"
        break
    fi
done

# Compatibility for an installation upgraded with only "Update Menu": older
# release archives still have the full implementation as /usr/local/x-ui/x-ui.sh.
if [[ -z "${_xui_core}" && -r "${_wrapper_xui_folder}/x-ui.sh" ]]; then
    _legacy_core="$(readlink -f "${_wrapper_xui_folder}/x-ui.sh" 2>/dev/null || printf '%s' "${_wrapper_xui_folder}/x-ui.sh")"
    _current_wrapper="$(readlink -f "${_wrapper_file}" 2>/dev/null || printf '%s' "${_wrapper_file}")"
    if [[ "${_legacy_core}" != "${_current_wrapper}" ]] && grep -q '^show_menu()' "${_legacy_core}" 2>/dev/null; then
        _xui_core="${_legacy_core}"
    fi
fi

if [[ -z "${_xui_core}" ]]; then
    echo "[ERR] x-ui-core.sh was not found. Reinstall or update the panel package." >&2
    exit 1
fi

# Passing an unknown argument makes the legacy script define all functions and
# return through show_usage instead of opening its interactive menu. Hide that
# one-time output; stderr remains visible for real startup errors.
# shellcheck disable=SC1090
source "${_xui_core}" __xui_core_load_only >/dev/null

function LOGW() {
    echo -e "${yellow}[WRN] $* ${plain}"
}

is_port_in_use() {
    local port="$1"
    if command -v ss >/dev/null 2>&1; then
        ss -ltnH 2>/dev/null | awk -v p=":${port}" '$4 ~ (p "$") {found=1; exit} END {exit !found}'
        return
    fi
    if command -v netstat >/dev/null 2>&1; then
        netstat -lnt 2>/dev/null | awk -v p=":${port}" '$4 ~ (p "$") {found=1; exit} END {exit !found}'
        return
    fi
    if command -v lsof >/dev/null 2>&1; then
        lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1
        return
    fi
    return 1
}

run_remote_script() {
    local url="$1"
    shift
    local temp_file rc
    temp_file=$(mktemp /tmp/x-ui-script.XXXXXX) || {
        LOGE "Failed to create a temporary file."
        return 1
    }
    if ! curl -fsSL --retry 3 --retry-delay 2 --connect-timeout 15 "$url" -o "$temp_file"; then
        LOGE "Failed to download: $url"
        rm -f "$temp_file"
        return 1
    fi
    bash "$temp_file" "$@"
    rc=$?
    rm -f "$temp_file"
    return "$rc"
}

install() {
    if run_remote_script "https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh"; then
        if [[ $# == 0 ]]; then
            start
        else
            start 0
        fi
        return $?
    else
        local rc=$?
    fi
    LOGE "Installation failed."
    [[ $# == 0 ]] && before_show_menu
    return "$rc"
}

update() {
    confirm "This function will update all x-ui components to the latest version, and the data will not be lost. Do you want to continue?" "y"
    if [[ $? != 0 ]]; then
        LOGE "Cancelled"
        [[ $# == 0 ]] && before_show_menu
        return 0
    fi
    if run_remote_script "https://raw.githubusercontent.com/SawaMEN/3x-ui/main/update.sh"; then
        LOGI "Update is complete."
        if [[ $# == 0 ]]; then
            exec /usr/bin/x-ui
        fi
        return 0
    else
        local rc=$?
    fi
    LOGE "Update failed. Review the errors above."
    [[ $# == 0 ]] && before_show_menu
    return "$rc"
}

update_dev() {
    confirm "This will update x-ui to the latest published DEV build (the rolling 'dev-latest' build, not a stable release). Your data is preserved. Continue?" "y"
    if [[ $? != 0 ]]; then
        LOGE "Cancelled"
        [[ $# == 0 ]] && before_show_menu
        return 0
    fi
    if XUI_UPDATE_TAG="dev-latest" run_remote_script "https://raw.githubusercontent.com/SawaMEN/3x-ui/main/update.sh"; then
        LOGI "Dev update is complete."
        if [[ $# == 0 ]]; then
            exec /usr/bin/x-ui
        fi
        return 0
    else
        local rc=$?
    fi
    LOGE "Dev update failed. Review the errors above."
    [[ $# == 0 ]] && before_show_menu
    return "$rc"
}

restore_packaged_menu() {
    local source="${xui_folder}/x-ui.sh" temp_file
    [[ -s "$source" ]] && bash -n "$source" || return 1
    if grep -q 'x-ui-core.sh' "$source"; then
        [[ -s "${xui_folder}/x-ui-core.sh" ]] && bash -n "${xui_folder}/x-ui-core.sh" || return 1
    fi
    temp_file=$(mktemp /usr/bin/x-ui-menu.XXXXXX) || return 1
    if cp "$source" "$temp_file" && chmod 0755 "$temp_file" && mv -f "$temp_file" /usr/bin/x-ui; then
        return 0
    fi
    rm -f "$temp_file"
    return 1
}

update_menu() {
    echo -e "${yellow}Updating Menu${plain}"
    confirm "Restore the management menu shipped with the installed panel version?" "y"
    if [[ $? != 0 ]]; then
        LOGE "Cancelled"
        [[ $# == 0 ]] && before_show_menu
        return 0
    fi
    if restore_packaged_menu; then
        LOGI "Menu updated successfully."
        if [[ $# == 0 ]]; then
            exec /usr/bin/x-ui
        fi
        LOGI "Run 'x-ui' again to use the updated menu."
        return 0
    fi
    LOGE "Failed to update the menu."
    [[ $# == 0 ]] && before_show_menu
    return 1
}

legacy_version() {
    local tag_version
    read -rp "Enter the panel version (like 2.4.0): " tag_version
    tag_version="${tag_version#v}"
    if [[ -z "$tag_version" ]]; then
        LOGE "Panel version cannot be empty."
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
    if [[ ! "$tag_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
        LOGE "Invalid panel version: $tag_version"
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
    LOGI "Downloading and installing panel version $tag_version..."
    if run_remote_script "https://raw.githubusercontent.com/SawaMEN/3x-ui/v${tag_version}/install.sh" "v${tag_version}"; then
        [[ $# == 0 ]] && exec /usr/bin/x-ui
        return 0
    else
        local rc=$?
    fi
    LOGE "Failed to install panel version $tag_version."
    [[ $# == 0 ]] && before_show_menu
    return "$rc"
}

reset_user() {
    confirm "Are you sure to reset the username and password of the panel?" "n"
    if [[ $? != 0 ]]; then
        [[ $# == 0 ]] && show_menu
        return 0
    fi
    local config_account config_password twoFactorConfirm
    read -rp "Please set the login username [default is a random username]: " config_account
    [[ -z $config_account ]] && config_account=$(gen_random_string 10)
    read -rp "Please set the login password [default is a random password]: " config_password
    [[ -z $config_password ]] && config_password=$(gen_random_string 18)
    read -rp "Do you want to disable currently configured two-factor authentication? (y/n): " twoFactorConfirm

    local args=(setting -username "$config_account" -password "$config_password")
    if [[ $twoFactorConfirm == "y" || $twoFactorConfirm == "Y" ]]; then
        args+=(-resetTwoFactor=true)
    fi
    if ! "${xui_folder}/x-ui" "${args[@]}" >/dev/null 2>&1; then
        LOGE "Failed to update panel credentials. Check the panel logs."
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
    [[ $twoFactorConfirm == "y" || $twoFactorConfirm == "Y" ]] && echo "Two factor authentication has been disabled."
    echo -e "Panel login username has been reset to: ${green}${config_account}${plain}"
    echo -e "Panel login password has been reset to: ${green}${config_password}${plain}"
    confirm_restart
}

reset_webbasepath() {
    echo -e "${yellow}Resetting Web Base Path${plain}"
    local answer config_webBasePath
    read -rp "Are you sure you want to reset the web base path? (y/n): " answer
    if [[ $answer != "y" && $answer != "Y" ]]; then
        echo -e "${yellow}Operation canceled.${plain}"
        return 0
    fi
    config_webBasePath=$(gen_random_string 18)
    if ! "${xui_folder}/x-ui" setting -webBasePath "$config_webBasePath" >/dev/null 2>&1; then
        LOGE "Failed to reset the web base path."
        return 1
    fi
    echo -e "Web base path has been reset to: ${green}${config_webBasePath}${plain}"
    restart
}

reset_config() {
    confirm "Are you sure you want to reset all panel settings, Account data will not be lost, Username and password will not change" "n"
    if [[ $? != 0 ]]; then
        [[ $# == 0 ]] && show_menu
        return 0
    fi
    if ! "${xui_folder}/x-ui" setting -reset; then
        LOGE "Failed to reset panel settings."
        [[ $# == 0 ]] && before_show_menu
        return 1
    fi
    echo -e "All panel settings have been reset to default."
    restart
}

set_port() {
    local port existing_port
    read -rp "Enter port number [1-65535]: " port
    if [[ -z "$port" ]]; then
        LOGD "Cancelled"
        before_show_menu
        return 0
    fi
    if [[ ! "$port" =~ ^[0-9]+$ ]] || ((port < 1 || port > 65535)); then
        LOGE "Invalid port: $port. Enter a number from 1 to 65535."
        before_show_menu
        return 1
    fi
    existing_port=$("${xui_folder}/x-ui" setting -show true 2>/dev/null | grep -Eo 'port: .+' | awk '{print $2}' | head -1)
    if [[ "$port" != "$existing_port" ]] && is_port_in_use "$port"; then
        LOGE "Port $port is already in use by another process."
        before_show_menu
        return 1
    fi
    if ! "${xui_folder}/x-ui" setting -port "$port"; then
        LOGE "Failed to change the panel port."
        before_show_menu
        return 1
    fi
    echo -e "The port is set. Restart the panel and use ${green}${port}${plain} to access the web panel."
    confirm_restart
}

# Return service-command failures to scripts instead of letting a later log
# message or menu condition replace the exit status with zero.
xui_service_action() {
    local action="$1"
    if [[ $release == alpine ]]; then
        rc-service x-ui "$action"
    else
        systemctl "$action" x-ui
    fi
}

finish_service_action() {
    local rc="$1" argc="$2"
    [[ "$argc" != 0 ]] || before_show_menu
    return "$rc"
}

start() {
    local rc=0
    if check_status; then
        LOGI "Panel is already running."
    elif [[ "$running_in_docker" == true ]]; then
        LOGE "Panel is not running. Restart the container from the host."
        rc=1
    elif xui_service_action start; then
        sleep 2
        if check_status; then LOGI "Panel started."; else LOGE "Panel failed to start."; rc=1; fi
    else
        rc=$?
        LOGE "Panel start failed."
    fi
    finish_service_action "$rc" "$#"
}

stop() {
    local rc=0 state
    check_status; state=$?
    if [[ "$state" == 1 ]]; then
        LOGI "Panel is already stopped."
    elif [[ "$state" == 2 && "$running_in_docker" != true ]]; then
        LOGE "x-ui.service is not installed or not loaded. Check systemctl status x-ui; repair with the installer."
        rc=1
    elif [[ "$running_in_docker" == true ]]; then
        LOGE "Stop the container from the host."
        rc=1
    elif xui_service_action stop; then
        sleep 2
        check_status; state=$?
        if [[ "$state" == 1 ]]; then LOGI "Panel stopped."; else LOGE "Panel failed to stop."; rc=1; fi
    else
        rc=$?
        LOGE "Panel stop failed."
    fi
    finish_service_action "$rc" "$#"
}

restart() {
    local rc=0
    if [[ "$running_in_docker" == true ]]; then
        if signal_xui HUP && signal_xui USR1; then LOGI "Restart signals sent."; else rc=$?; LOGE "Could not signal panel."; fi
    elif xui_service_action restart; then
        sleep 2
        if check_status; then LOGI "Panel restarted."; else LOGE "Panel failed to restart."; rc=1; fi
    else
        rc=$?
        LOGE "Panel restart failed."
    fi
    finish_service_action "$rc" "$#"
}

restart_xray() {
    local rc=0
    if [[ "$running_in_docker" == true ]]; then
        signal_xui USR1 || rc=$?
    else
        xui_service_action reload || rc=$?
    fi
    if [[ "$rc" == 0 ]]; then LOGI "Core reload requested."; else LOGE "Core reload failed."; fi
    finish_service_action "$rc" "$#"
}

status() {
    local rc=0
    if [[ "$running_in_docker" == true ]]; then
        show_status
        check_status || rc=$?
    else
        xui_service_action status || rc=$?
    fi
    finish_service_action "$rc" "$#"
}

set_autostart() {
    local action="$1" argc="$2" rc=0
    if [[ "$running_in_docker" == true ]]; then
        LOGI "Autostart is controlled by the Docker restart policy."
    elif [[ $release == alpine ]]; then
        if [[ "$action" == enable ]]; then rc-update add x-ui default || rc=$?; else rc-update del x-ui || rc=$?; fi
    else
        systemctl "$action" x-ui || rc=$?
    fi
    if [[ "$rc" == 0 ]]; then LOGI "Autostart setting applied."; else LOGE "Autostart setting failed."; fi
    finish_service_action "$rc" "$argc"
}

enable() { set_autostart enable "$#"; }
disable() { set_autostart disable "$#"; }

xui_pid() {
    ps -ef 2>/dev/null | grep -F "${xui_folder}/x-ui" | grep -v grep | awk 'NR==1 {print $2}'
}

check_status() {
    if [[ "${running_in_docker}" == "true" ]]; then
        [[ -x "${xui_folder}/x-ui" ]] || return 2
        [[ -n "$(xui_pid)" ]] && return 0
        return 1
    fi
    if [[ $release == "alpine" ]]; then
        [[ -f /etc/init.d/x-ui ]] || return 2
        rc-service x-ui status >/dev/null 2>&1 && return 0
        return 1
    fi
    command -v systemctl >/dev/null 2>&1 || return 2
    systemctl cat x-ui >/dev/null 2>&1 || return 2
    systemctl is-active --quiet x-ui && return 0
    return 1
}

check_enabled() {
    if [[ "${running_in_docker}" == "true" ]]; then
        return 1
    fi
    if [[ $release == "alpine" ]]; then
        rc-update show default 2>/dev/null | grep -Eq '^[[:space:]]*x-ui[[:space:]]*\|'
        return $?
    fi
    systemctl is-enabled --quiet x-ui 2>/dev/null
}

check_xray_status() {
    local bin_folder="${XUI_BIN_FOLDER:-${xui_folder}/bin}" proc exe
    [[ "$bin_folder" == /* ]] || bin_folder="${xui_folder}/${bin_folder}"
    bin_folder=$(readlink -f "$bin_folder" 2>/dev/null) || return 1
    for proc in /proc/[0-9]*/exe; do
        exe=$(readlink "$proc" 2>/dev/null) || continue
        case "$exe" in
            "$bin_folder"/hiddify-core-linux-*|"$bin_folder"/sing-box-linux-*) return 0 ;;
        esac
    done
    return 1
}

show_status() {
    check_status
    case $? in
        0)
            echo -e "Panel state: ${green}Running${plain}"
            show_enable_status
            ;;
        1)
            echo -e "Panel state: ${yellow}Not Running${plain}"
            show_enable_status
            ;;
        2)
            echo -e "Panel state: ${red}Not Installed${plain}"
            ;;
    esac
    show_xray_status
    show_mtproto_status
    show_telemt_status
}

show_log() {
    if [[ $release == "alpine" ]]; then
        echo -e "${green}\t1.${plain} Debug Log"
        echo -e "${green}\t0.${plain} Back to Main Menu"
        read -rp "Choose an option: " choice
        case "$choice" in
            0) return 0 ;;
            1) grep -F 'x-ui[' /var/log/messages 2>/dev/null || true ;;
            *) LOGE "Invalid option." ;;
        esac
        [[ $# == 0 ]] && before_show_menu
        return 0
    fi

    echo -e "${green}\t1.${plain} Debug Log"
    echo -e "${green}\t2.${plain} Recent panel errors"
    echo -e "${green}\t3.${plain} Panel update log"
    echo -e "${green}\t0.${plain} Back to Main Menu"
    read -rp "Choose an option: " choice
    case "$choice" in
        0) return 0 ;;
        1)
            journalctl -u x-ui -e --no-pager -f -p debug
            ;;
        2)
            journalctl -u x-ui -n 100 --no-pager -p err
            ;;
        3)
            if [[ -r /var/log/x-ui/update.log ]]; then
                tail -n 100 /var/log/x-ui/update.log
            else
                LOGW "No panel update log exists yet."
            fi
            ;;
        *)
            LOGE "Invalid option."
            ;;
    esac
    [[ $# == 0 ]] && before_show_menu
}

require_ufw() {
    if command -v ufw >/dev/null 2>&1; then
        return 0
    fi
    LOGE "UFW is not installed. Choose 'Install Firewall' first."
    return 1
}

install_firewall() {
    if ! command -v ufw >/dev/null 2>&1; then
        echo "UFW firewall is not installed. Installing now..."
        case "${release}" in
            ubuntu | debian | armbian)
                apt-get update && apt-get install -y ufw
                ;;
            fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol)
                dnf install -y ufw
                ;;
            centos)
                if command -v dnf >/dev/null 2>&1; then dnf install -y ufw; else yum install -y ufw; fi
                ;;
            arch | manjaro | parch)
                pacman -S --noconfirm --needed ufw
                ;;
            opensuse-tumbleweed | opensuse-leap)
                zypper -q install -y ufw
                ;;
            alpine)
                apk add --no-cache ufw
                ;;
            *)
                LOGE "Unsupported OS for automatic UFW installation: ${release}"
                return 1
                ;;
        esac
    fi
    command -v ufw >/dev/null 2>&1 || {
        LOGE "UFW installation failed."
        return 1
    }

    local ssh_ports panel_info port
    ssh_ports=$(grep -E '^[[:space:]]*Port[[:space:]]+[0-9]+' /etc/ssh/sshd_config 2>/dev/null | awk '{print $2}' | sort -nu)
    [[ -n "$ssh_ports" ]] || ssh_ports="22"
    while read -r port; do
        [[ -n "$port" ]] && ufw allow "${port}/tcp"
    done <<< "$ssh_ports"
    ufw allow 80/tcp
    ufw allow 443/tcp

    if [[ -x "${xui_folder}/x-ui" ]]; then
        panel_info=$("${xui_folder}/x-ui" setting -show true 2>/dev/null)
        while read -r port; do
            if [[ "$port" =~ ^[0-9]+$ ]] && ((port >= 1 && port <= 65535)); then
                ufw allow "${port}/tcp"
            fi
        done < <(printf '%s\n' "$panel_info" | awk -F': ' '/^(port|subPort): / {print $2}' | sort -nu)
    fi

    ufw --force enable || return 1
    if [[ $release != "alpine" ]] && command -v systemctl >/dev/null 2>&1 && systemctl cat ufw.service >/dev/null 2>&1; then
        systemctl enable ufw.service >/dev/null 2>&1 || true
    fi
    LOGI "Firewall is installed and enabled."
}

open_ports() {
    local ports port start_port end_port
    read -rp "Enter the ports you want to open (e.g. 80,443,2053 or range 400-500): " ports
    if ! [[ $ports =~ ^([0-9]+|[0-9]+-[0-9]+)(,([0-9]+|[0-9]+-[0-9]+))*$ ]]; then
        LOGE "Invalid input. Use comma-separated ports or ranges."
        return 1
    fi
    IFS=',' read -ra PORT_LIST <<< "$ports"
    for port in "${PORT_LIST[@]}"; do
        if [[ $port == *-* ]]; then
            start_port=${port%-*}
            end_port=${port#*-}
            if ((start_port < 1 || start_port > 65535 || end_port < 1 || end_port > 65535 || start_port > end_port)); then
                LOGE "Invalid port range: $port"
                return 1
            fi
            ufw allow "${start_port}:${end_port}/tcp" || return 1
            ufw allow "${start_port}:${end_port}/udp" || return 1
        else
            if ((port < 1 || port > 65535)); then
                LOGE "Invalid port: $port"
                return 1
            fi
            ufw allow "$port" || return 1
        fi
    done
    LOGI "Requested ports were added to UFW."
}

delete_ports() {
    local choice rule_numbers ports port start_port end_port rule_number
    echo "Current UFW rules:"
    ufw status numbered
    echo "Do you want to delete rules by:"
    echo "1) Rule numbers"
    echo "2) Ports"
    read -rp "Enter your choice (1 or 2): " choice

    if [[ $choice == "1" ]]; then
        read -rp "Enter the rule numbers you want to delete (1,2,...): " rule_numbers
        if ! [[ $rule_numbers =~ ^([0-9]+)(,[0-9]+)*$ ]]; then
            LOGE "Invalid rule-number list."
            return 1
        fi
        IFS=',' read -ra RULE_NUMBERS <<< "$rule_numbers"
        mapfile -t RULE_NUMBERS < <(printf '%s\n' "${RULE_NUMBERS[@]}" | sort -rn)
        for rule_number in "${RULE_NUMBERS[@]}"; do
            ufw --force delete "$rule_number" || return 1
        done
        LOGI "Selected rules have been deleted."
        return 0
    fi

    if [[ $choice == "2" ]]; then
        read -rp "Enter the ports you want to delete (e.g. 80,443,2053 or range 400-500): " ports
        if ! [[ $ports =~ ^([0-9]+|[0-9]+-[0-9]+)(,([0-9]+|[0-9]+-[0-9]+))*$ ]]; then
            LOGE "Invalid port list."
            return 1
        fi
        IFS=',' read -ra PORT_LIST <<< "$ports"
        for port in "${PORT_LIST[@]}"; do
            if [[ $port == *-* ]]; then
                start_port=${port%-*}
                end_port=${port#*-}
                if ((start_port < 1 || start_port > 65535 || end_port < 1 || end_port > 65535 || start_port > end_port)); then
                    LOGE "Invalid port range: $port"
                    return 1
                fi
                ufw --force delete allow "${start_port}:${end_port}/tcp" || true
                ufw --force delete allow "${start_port}:${end_port}/udp" || true
            else
                if ((port < 1 || port > 65535)); then
                    LOGE "Invalid port: $port"
                    return 1
                fi
                ufw --force delete allow "$port" || true
            fi
        done
        LOGI "Requested port rules have been removed where present."
        return 0
    fi

    LOGE "Invalid choice. Please enter 1 or 2."
    return 1
}

firewall_menu() {
    while true; do
        echo -e "${green}\t1.${plain} ${green}Install${plain} Firewall"
        echo -e "${green}\t2.${plain} Port List [numbered]"
        echo -e "${green}\t3.${plain} ${green}Open${plain} Ports"
        echo -e "${green}\t4.${plain} ${red}Delete${plain} Ports from List"
        echo -e "${green}\t5.${plain} ${green}Enable${plain} Firewall"
        echo -e "${green}\t6.${plain} ${red}Disable${plain} Firewall"
        echo -e "${green}\t7.${plain} Firewall Status"
        echo -e "${green}\t0.${plain} Back to Main Menu"
        read -rp "Choose an option: " choice
        case "$choice" in
            0) return 0 ;;
            1) install_firewall ;;
            2) require_ufw && ufw status numbered ;;
            3) require_ufw && open_ports ;;
            4) require_ufw && delete_ports ;;
            5) require_ufw && ufw --force enable ;;
            6) require_ufw && ufw disable ;;
            7) require_ufw && ufw status verbose ;;
            *) LOGE "Invalid option. Please select a valid number." ;;
        esac
    done
}

run_speedtest() {
    if ! command -v speedtest >/dev/null 2>&1 && ! command -v speedtest-cli >/dev/null 2>&1; then
        echo "Installing a Speedtest CLI..."
        case "${release}" in
            arch | manjaro | parch)
                pacman -S --noconfirm --needed speedtest-cli || return 1
                ;;
            ubuntu | debian | armbian)
                curl -fsSL https://packagecloud.io/install/repositories/ookla/speedtest-cli/script.deb.sh | bash || return 1
                apt-get install -y speedtest || return 1
                ;;
            fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol | centos)
                curl -fsSL https://packagecloud.io/install/repositories/ookla/speedtest-cli/script.rpm.sh | bash || return 1
                if command -v dnf >/dev/null 2>&1; then dnf install -y speedtest; else yum install -y speedtest; fi || return 1
                ;;
            opensuse-tumbleweed | opensuse-leap)
                zypper -q install -y speedtest-cli || return 1
                ;;
            alpine)
                apk add --no-cache speedtest-cli || apk add --no-cache py3-speedtest-cli || return 1
                ;;
            *)
                LOGE "No supported automatic Speedtest installation method for ${release}."
                return 1
                ;;
        esac
    fi
    if command -v speedtest >/dev/null 2>&1; then
        speedtest
    elif command -v speedtest-cli >/dev/null 2>&1; then
        speedtest-cli
    else
        LOGE "Speedtest installation completed but no CLI command was found."
        return 1
    fi
}

# Gateway networking and recovery are owned by the Go service shared with the web UI.
gateway_enable() {
    local lan_if lan_ip prefix wan_if
    read -rp "LAN interface: " lan_if
    read -rp "LAN IPv4 address: " lan_ip
    read -rp "LAN prefix [24]: " prefix
    read -rp "WAN interface for NAT [none]: " wan_if
    "${xui_folder}/x-ui" gateway enable --lan-interface "${lan_if}" --lan-ip "${lan_ip}" --lan-prefix "${prefix:-24}" --wan-interface "${wan_if}"
}

gateway_disable() {
    "${xui_folder}/x-ui" gateway disable
}

gateway_status() {
    "${xui_folder}/x-ui" gateway status
}

gateway_menu() {
    echo ""
    echo -e "${green}\t1.${plain} Enable Gateway Mode"
    echo -e "${green}\t2.${plain} Disable Gateway Mode"
    echo -e "${green}\t3.${plain} Gateway Status"
    echo -e "${green}\t0.${plain} Back to Main Menu"

    read -rp "Choose an option: " choice

    case "${choice}" in
        0)
            show_menu
            ;;
        1)
            gateway_enable
            gateway_menu
            ;;
        2)
            gateway_disable
            gateway_menu
            ;;
        3)
            gateway_status
            gateway_menu
            ;;
        *)
            LOGE "Invalid option."
            gateway_menu
            ;;
    esac
}

print_console_box() {
    local row width=48 border
    for row in "$@"; do
        (( ${#row} <= width )) || width=${#row}
    done
    printf -v border '%*s' "$((width + 2))" ''
    border=${border// /─}
    printf '┌%s┐\n' "$border"
    for row in "$@"; do
        if [[ "$row" == -- ]]; then
            printf '├%s┤\n' "$border"
        else
            printf '│ %-*s │\n' "$width" "$row"
        fi
    done
    printf '└%s┘\n' "$border"
}

show_usage() {
    print_console_box \
        "x-ui control menu usages (subcommands):" \
        "" \
        "x-ui                       - Admin Management Script" \
        "x-ui start                 - Start" \
        "x-ui stop                  - Stop" \
        "x-ui restart               - Restart" \
        "x-ui restart-core          - Restart Sing-box" \
        "x-ui status                - Current Status" \
        "x-ui settings              - Current Settings" \
        "x-ui enable                - Enable Autostart on OS Startup" \
        "x-ui disable               - Disable Autostart on OS Startup" \
        "x-ui log                   - Check logs" \
        "x-ui banlog                - Check Fail2ban ban logs" \
        "x-ui update                - Update" \
        "x-ui update-dev            - Update to Dev channel (latest)" \
        "x-ui update-menu           - Update management menu" \
        "x-ui update-all-geofiles   - Update all geo files" \
        "x-ui migrateDB [file]      - Convert .db <-> .dump (SQLite)" \
        "x-ui pgclient [ver]        - Upgrade pg_dump/pg_restore tools" \
        "x-ui legacy                - Legacy version" \
        "x-ui install               - Install" \
        "x-ui uninstall             - Uninstall"
}

before_show_menu() {
    read -rp "Press enter to return to the main menu: " _ || true
}

show_menu() {
    [[ "${_xui_menu_active:-0}" == 0 ]] || return 0
    local _xui_menu_active=1
    while true; do
        print_console_box \
            "3X-UI Panel Management Script" \
            "0. Exit Script" \
            "--" \
            "1. Install" \
            "2. Update" \
            "3. Update to DEV (published build)" \
            "4. Restore Installed Menu" \
            "5. Legacy Version" \
            "6. Uninstall" \
            "--" \
            "7. Reset Username & Password" \
            "8. Reset Web Base Path" \
            "9. Reset Settings" \
            "10. Change Port" \
            "11. View Current Settings" \
            "--" \
            "12. Start" \
            "13. Stop" \
            "14. Restart" \
            "15. Restart Sing-box" \
            "16. Check Status" \
            "17. Logs Management" \
            "--" \
            "18. Enable Autostart" \
            "19. Disable Autostart" \
            "--" \
            "20. SSL Certificate Management" \
            "21. Cloudflare SSL Certificate" \
            "22. IP Limit Management" \
            "23. Firewall Management" \
            "24. SSH Port Forwarding Management" \
            "25. PostgreSQL Management" \
            "--" \
            "26. Enable BBR" \
            "27. Update Geo Files" \
            "28. Speedtest" \
            "29. Gateway Mode"
        show_status
        echo
        read -rp "Please enter your selection [0-29]: " num || return 0
        case "${num}" in
            0) exit 0 ;;
            1) check_uninstall && install ;;
            2) check_install && update ;;
            3) check_install && update_dev ;;
            4) check_install && update_menu ;;
            5) check_install && legacy_version ;;
            6) check_install && uninstall ;;
            7) check_install && reset_user ;;
            8) check_install && reset_webbasepath ;;
            9) check_install && reset_config ;;
            10) check_install && set_port ;;
            11) check_install && check_config ;;
            12) check_install && start ;;
            13) check_install && stop ;;
            14) check_install && restart ;;
            15) check_install && restart_xray ;;
            16) check_install && status ;;
            17) check_install && show_log ;;
            18) check_install && enable ;;
            19) check_install && disable ;;
            20) check_install && ssl_cert_issue_main ;;
            21) check_install && ssl_cert_issue_CF ;;
            22) check_install && iplimit_main ;;
            23) firewall_menu ;;
            24) check_install && SSH_port_forwarding ;;
            25) check_install && postgresql_menu ;;
            26) bbr_menu ;;
            27) check_install && update_geo ;;
            28) run_speedtest ;;
            29) check_install && gateway_menu ;;
            *) LOGE "Please enter the correct number [0-29]" ;;
        esac
    done
}

if [[ $# -gt 0 ]]; then
    case "$1" in
        start) check_install 0 && start 0 ;;
        stop) check_install 0 && stop 0 ;;
        restart) check_install 0 && restart 0 ;;
        restart-core|restart-xray) check_install 0 && restart_xray 0 ;;
        status) check_install 0 && status 0 ;;
        settings) check_install 0 && check_config 0 ;;
        enable) check_install 0 && enable 0 ;;
        disable) check_install 0 && disable 0 ;;
        log) check_install 0 && show_log 0 ;;
        banlog) check_install 0 && show_banlog 0 ;;
        setup-fail2ban) setup_fail2ban_iplimit ;;
        update) check_install 0 && update 0 ;;
        update-dev) check_install 0 && update_dev 0 ;;
        update-menu) check_install 0 && update_menu 0 ;;
        legacy) check_install 0 && legacy_version 0 ;;
        install) check_uninstall 0 && install 0 ;;
        uninstall) check_install 0 && uninstall 0 ;;
        update-all-geofiles)
            geo_updated=0
            if check_install 0 && update_all_geofiles 0; then
                [[ $geo_updated -eq 0 ]] || restart 0
            fi
            ;;
        migrateDB) migrate_db "$2" "$3" ;;
        pgclient) pg_upgrade_client "$2" ;;
        gateway)
            check_install 0 || exit 1
            "${xui_folder}/x-ui" gateway "${@:2}"
            ;;
        help|--help|-h) show_usage ;;
        *) show_usage; exit 1 ;;
    esac
else
    show_menu
fi
