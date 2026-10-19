# Embedded into the pinned upstream installer before its main invocation.
# Keep all artifacts private to this invocation. Both builds must succeed before
# replacing either installed binary; upstream uses shared /tmp output paths.
main() {
	need_root
	local work panel_src olcrtc_src
	work="$(mktemp -d /tmp/3x-ui-olcrtc.XXXXXX)"
	# Expand now: main's local variables no longer exist when EXIT is raised.
	trap "rm -rf -- '$work'" EXIT
	panel_src="$work/panel"
	olcrtc_src="$work/olcrtc"

	printf '3XUI_OLCRTC_STAGE=packages\n'
	install_packages
	printf '3XUI_OLCRTC_STAGE=toolchain\n'
	install_go
	printf '3XUI_OLCRTC_STAGE=memory\n'
	ensure_build_memory
	printf '3XUI_OLCRTC_STAGE=sources\n'
	clone_repo "$OLCRTC_REPO" "$OLCRTC_REF" "$olcrtc_src"
	clone_repo "$PANEL_REPO" "$PANEL_REF" "$panel_src"

	printf '3XUI_OLCRTC_STAGE=build-server\n'
	(cd "$olcrtc_src" && CGO_ENABLED=0 go build -p "${GO_BUILD_P:-2}" -o "$work/olcrtc" ./cmd/olcrtc)
	printf '3XUI_OLCRTC_STAGE=build-manager\n'
	[ -f "$panel_src/cmd/olcrtc-manager/web/dist/index.html" ] || die 'manager frontend bundle is missing'
	(cd "$panel_src" && CGO_ENABLED=0 go build -p "${GO_BUILD_P:-2}" -o "$work/olcrtc-manager" ./cmd/olcrtc-manager)

	printf '3XUI_OLCRTC_STAGE=deploy\n'
	install -m 0755 "$work/olcrtc" /usr/local/bin/olcrtc
	install -m 0755 "$work/olcrtc-manager" /usr/local/bin/olcrtc-manager
	detect_manager_features
	printf '3XUI_OLCRTC_STAGE=config\n'
	write_config_if_missing
	apply_config_port
	write_panel_env_if_missing
	# Store the exact pair before starting the service: a startup failure must
	# remain recoverable by Update/Remove, rather than looking unmanaged.
	printf '%s|%s\n' "$(git -C "$panel_src" rev-parse HEAD)" "$(git -C "$olcrtc_src" rev-parse HEAD)" > "$BUILT_REVISIONS_PATH"
	install -m 0600 "$BUILT_REVISIONS_PATH" "$CONFIG_DIR/.3x-ui-revisions"
	printf '3XUI_OLCRTC_STAGE=service\n'
	install_service
	printf '3XUI_OLCRTC_STAGE=complete\n'
}
main "$@"
