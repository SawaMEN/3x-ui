# Embedded before the pinned upstream main invocation.
managed_toolchain() {
	local arch checksum cache archive
	arch="$(go_arch)"
	case "$arch" in
		amd64) checksum=2b2cfc7148493da5e73981bffbf3353af381d5f93e789c82c79aff64962eb556 ;;
		arm64) checksum=9d89a3ea57d141c2b22d70083f2c8459ba3890f2d9e818e7e933b75614936565 ;;
		*) die 'unsupported toolchain architecture' ;;
	esac
	cache="/opt/3x-ui/olcrtc/toolchains/go1.26.3-$arch"
	if [ ! -x "$cache/go/bin/go" ] || [ "$(cat "$cache/checksum" 2>/dev/null || true)" != "$checksum" ]; then
		archive="$work/go.tar.gz"
		curl -fsSL --retry 3 --connect-timeout 15 --max-time 300 "https://go.dev/dl/go1.26.3.linux-$arch.tar.gz" -o "$archive"
		printf '%s  %s\n' "$checksum" "$archive" | sha256sum -c -
		mkdir -p -m 0700 "$cache"
		tar -C "$cache" -xzf "$archive"
		printf '%s\n' "$checksum" > "$cache/checksum"
	fi
	unset GOROOT GOTOOLDIR
	export PATH="$cache/go/bin:$PATH" GOTOOLCHAIN=local GOENV=off GOWORK=off GOFLAGS=
}

managed_sources() {
	local repo="$1" revision="$2" dest="$3"
	git init -q "$dest"
	git -C "$dest" remote add origin "$repo"
	git -C "$dest" fetch -q --depth 1 origin "$revision"
	git -C "$dest" checkout -q --detach FETCH_HEAD
	[ "$(git -C "$dest" rev-parse HEAD)" = "$revision" ] || die 'source revision mismatch'
}

managed_files() {
	files=("$MANAGED_BIN_DIR/olcrtc" "$MANAGED_BIN_DIR/olcrtc-manager" "$CONFIG_PATH" "$PANEL_ENV_PATH" "$CONFIG_DIR/.3x-ui-revisions" "$MANAGED_SERVICE_PATH")
}

cleanup_transaction() {
	local oldwork
	oldwork="$(cat "$transaction/workdir" 2>/dev/null || true)"
	case "$oldwork" in /tmp/3x-ui-olcrtc.*) rm -rf -- "$oldwork" ;; esac
	rm -rf -- "$transaction"
}

recover_install() {
	local i failed=0
	[ -d "$transaction" ] || return 0
	if [ -f "$transaction/committed" ]; then cleanup_transaction; return 0; fi
	if [ ! -f "$transaction/ready" ]; then
		if [ -f "$transaction/active" ]; then systemctl start olcrtc-manager || return 1; fi
		cleanup_transaction
		return 0
	fi
	printf '3XUI_OLCRTC_RECOVERY=started\n'
	systemctl stop olcrtc-manager || { if systemctl is-active --quiet olcrtc-manager; then failed=1; fi; }
	managed_files
	for i in "${!files[@]}"; do
		if [ -f "$transaction/$i" ]; then
			cp -p -- "$transaction/$i" "${files[$i]}.restore" && mv -f -- "${files[$i]}.restore" "${files[$i]}" || failed=1
		else
			rm -f -- "${files[$i]}" || failed=1
		fi
	done
	systemctl daemon-reload || failed=1
	if [ -f "$transaction/enabled" ]; then
		systemctl enable olcrtc-manager || failed=1
	else
		if [ -f "$MANAGED_SERVICE_PATH" ]; then systemctl disable olcrtc-manager || failed=1; fi
	fi
	if [ -f "$transaction/active" ]; then
		systemctl start olcrtc-manager || failed=1
	fi
	[ "$failed" = 0 ] || { printf '3XUI_OLCRTC_STAGE=rollback\n'; return 1; }
	rm -f -- "$MANAGED_BIN_DIR/olcrtc.new" "$MANAGED_BIN_DIR/olcrtc-manager.new"
	cleanup_transaction
}

finish_install() {
	local result="$1"
	trap - EXIT INT TERM
	if [ "$result" != 0 ]; then
		recover_install || result=1
	fi
	rm -rf -- "$work"
	exit "$result"
}

snapshot_install() {
	local i
	mkdir -p -m 0700 "$CONFIG_DIR"
	# The previous invocation may have died during a deployment. Restore it
	# before building a new transaction. Never overwrite a recovery journal.
	recover_install
	rm -rf -- "$transaction"
	mkdir -m 0700 "$transaction"
	printf '%s\n' "$work" > "$transaction/workdir"
	if systemctl is-active --quiet olcrtc-manager; then touch "$transaction/active"; fi
	if systemctl is-enabled --quiet olcrtc-manager; then touch "$transaction/enabled"; fi
	if [ -f "$MANAGED_SERVICE_PATH" ]; then systemctl stop olcrtc-manager; fi
	managed_files
	for i in "${!files[@]}"; do
		if [ -e "${files[$i]}" ]; then cp -p -- "${files[$i]}" "$transaction/$i"; fi
	done
	# No managed file has changed before the complete snapshot exists.
	sync -f "$transaction"
	touch "$transaction/ready"
	sync -f "$transaction"
}

manager_ready() {
	local user pass attempt
	user="$(read_env_value OLCRTC_MANAGER_USER "$PANEL_ENV_PATH")"
	pass="$(read_env_value OLCRTC_MANAGER_PASS "$PANEL_ENV_PATH")"
	[ -n "$user" ] && [ -n "$pass" ] || return 1
	for attempt in $(seq 1 15); do
		if curl --fail --silent --max-time 2 --noproxy '*' --user "$user:$pass" "http://127.0.0.1:$PANEL_PORT/api/settings" -o /dev/null; then return 0; fi
		sleep 1
	done
	return 1
}

main() {
	need_root
	# Globals intentionally survive until the EXIT handler runs.
	work="$(mktemp -d /tmp/3x-ui-olcrtc.XXXXXX)"
	transaction="$CONFIG_DIR/.3x-ui-transaction"
	trap 'finish_install $?' EXIT
	trap 'exit 130' INT TERM
	recover_install
	local panel_src="$work/panel" olcrtc_src="$work/server-src"
	printf '3XUI_OLCRTC_STAGE=packages\n'
	install_packages
	printf '3XUI_OLCRTC_STAGE=toolchain\n'
	managed_toolchain
	printf '3XUI_OLCRTC_STAGE=memory\n'
	ensure_build_memory
	printf '3XUI_OLCRTC_STAGE=sources\n'
	managed_sources "$OLCRTC_REPO" "$OLCRTC_REF" "$olcrtc_src"
	managed_sources "$PANEL_REPO" "$PANEL_REF" "$panel_src"
	git -C "$panel_src" apply --check "$MANAGER_PATCH_PATH"
	git -C "$panel_src" apply "$MANAGER_PATCH_PATH"
	cp "$MANAGER_SAFETY_PATH" "$panel_src/cmd/olcrtc-manager/panel_safety.go"
	printf '3XUI_OLCRTC_STAGE=build-server\n'
	(cd "$olcrtc_src" && CGO_ENABLED=0 go build -p "${GO_BUILD_P:-2}" -o "$work/olcrtc" ./cmd/olcrtc)
	printf '3XUI_OLCRTC_STAGE=build-manager\n'
	[ -f "$panel_src/cmd/olcrtc-manager/web/dist/index.html" ] || die 'manager frontend bundle is missing'
	(cd "$panel_src" && CGO_ENABLED=0 go build -p "${GO_BUILD_P:-2}" -o "$work/olcrtc-manager" ./cmd/olcrtc-manager)
	printf '3XUI_OLCRTC_STAGE=deploy\n'
	snapshot_install
	install -m 0755 "$work/olcrtc" "$MANAGED_BIN_DIR/olcrtc.new"
	install -m 0755 "$work/olcrtc-manager" "$MANAGED_BIN_DIR/olcrtc-manager.new"
	mv -f "$MANAGED_BIN_DIR/olcrtc.new" "$MANAGED_BIN_DIR/olcrtc"
	mv -f "$MANAGED_BIN_DIR/olcrtc-manager.new" "$MANAGED_BIN_DIR/olcrtc-manager"
	detect_manager_features
	printf '3XUI_OLCRTC_STAGE=config\n'
	write_config_if_missing
	apply_config_port
	write_panel_env_if_missing
	printf '%s|%s|%s\n' "$PANEL_REF" "$OLCRTC_REF" "$MANAGER_PATCH_VERSION" > "$BUILT_REVISIONS_PATH"
	install -m 0600 "$BUILT_REVISIONS_PATH" "$CONFIG_DIR/.3x-ui-revisions"
	printf '3XUI_OLCRTC_STAGE=service\n'
	install_service
	systemctl restart olcrtc-manager
	printf '3XUI_OLCRTC_STAGE=health\n'
	manager_ready
	# The health check is part of the transaction. Failed starts restore both
	# binaries, unit, clients, credentials, revision, active/enabled state.
	sync -f "$MANAGED_BIN_DIR"
	sync -f "$(dirname "$MANAGED_SERVICE_PATH")"
	sync -f "$CONFIG_DIR"
	touch "$transaction/committed"
	sync -f "$transaction"
	cleanup_transaction
	printf '3XUI_OLCRTC_STAGE=complete\n'
}
main "$@"
