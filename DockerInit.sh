#!/bin/sh
set -eu

# Native Docker builds support the same architectures as release packages.
case "${1:-}" in
    amd64 | arm64) arch=$1 ;;
    *) echo "DockerInit: unsupported architecture: ${1:-unset}" >&2; exit 1 ;;
esac

# The runtime uses official Telemt, not the retired mtg-multi process.
python3 tools/telemt/fetch.py --arch "$arch" --output build/bin

fetch_data() {
    curl --fail --location --silent --show-error --retry 5 --retry-all-errors \
        --connect-timeout 20 --output "build/bin/$1" "$2"
}
fetch_data geoip.dat https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat
fetch_data geosite.dat https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat
fetch_data geoip_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat
fetch_data geosite_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat
fetch_data geoip_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat
fetch_data geosite_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat
