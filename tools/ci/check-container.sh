#!/usr/bin/env bash
set -euo pipefail
image=${1:?image required}
arch=${2:?architecture required}
core="/app/bin/hiddify-core-linux-$arch"
# No entrypoint side effects are needed for identity/config inspection.
docker run --rm --entrypoint /app/x-ui "$image" -v
docker run --rm --entrypoint "$core" "$image" version
docker run --rm --entrypoint /bin/sh "$image" -ec '
  test -x /app/bin/telemt
  test -x /app/bin/mita
  test ! -e /app/bin/mtg-linux-amd64
  test ! -e /app/bin/mtg-linux-arm64
  test ! -e /app/bin/libcronet.so
  /app/bin/telemt --version
  /app/bin/mita version
  cd /app/bin
  sha256sum -c hiddify-core-linux-*.sha256
'
# Exercise the actual packaged kernel parser and Cronet initialization.
docker run --rm --entrypoint /bin/sh "$image" -ec '
  printf "%s\n" '\''{"outbounds":[{"type":"naive","tag":"cronet-check","server":"127.0.0.1","server_port":443,"username":"audit","password":"synthetic","tls":{"enabled":true}}]}'\'' > /tmp/audit.json
  /app/bin/hiddify-core-linux-'"$arch"' check -c /tmp/audit.json
'
# Inspect and execute the exact musl kernel from the image on the native runner.
task_tmp=$(mktemp -d)
probe=$(docker create "$image")
docker cp "$probe:$core" "$task_tmp/core"
docker cp "$probe:/app/bin/target.json" "$task_tmp/target.json"
docker rm "$probe" >/dev/null
readelf -h "$task_tmp/core"
if readelf -l "$task_tmp/core" | grep -q INTERP || readelf -d "$task_tmp/core" | grep -q NEEDED; then
  echo 'Packaged musl kernel unexpectedly needs a dynamic loader/library' >&2
  exit 1
fi
python3 - "$task_tmp/target.json" <<'PYLOCK'
import json, sys
actual=json.load(open(sys.argv[1]))
expected=json.load(open('docs/hiddify-core-target.json'))
assert actual['target']==expected['target']
assert actual['panel_build']['cronet_linkage']=='musl'
assert 'with_musl' in actual['panel_build']['tags']
assert 'with_purego' not in actual['panel_build']['tags']
PYLOCK
mkdir -p internal/web/dist
touch internal/web/dist/.gitkeep
HIDDIFY_CONFIG_CHECK_BINARY="$task_tmp/core" go test -json ./internal/hiddify -run '^TestPinnedBinaryChecksGeneratedConfigurations$' -count=1 | tee "$task_tmp/config-check.jsonl"
python3 tools/ci/require-go-tests.py "$task_tmp/config-check.jsonl" TestPinnedBinaryChecksGeneratedConfigurations
rm -rf -- "$task_tmp"
container=$(docker run -d -e XUI_ENABLE_FAIL2BAN=false -p 127.0.0.1::2053 "$image")
trap 'docker logs "$container"; docker rm -f "$container" >/dev/null' EXIT
address=$(docker port "$container" 2053/tcp)
for attempt in {1..60}; do
  if curl --fail --silent "http://$address/" >/dev/null; then
    echo "Native $arch container panel is ready"
    exit 0
  fi
  if [[ $(docker inspect --format '{{.State.Running}}' "$container") != true ]]; then
    echo 'Container exited before readiness' >&2
    exit 1
  fi
  sleep 2
done
echo 'Container readiness timeout' >&2
exit 1
