[English](/README.md) | [Русский](/README.ru_RU.md)

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./media/3x-ui-dark.png">
    <img alt="3x-ui" src="./media/3x-ui-light.png">
  </picture>
</p>

<p align="center">
  <a href="https://github.com/SawaMEN/3x-ui/releases"><img src="https://img.shields.io/github/v/release/SawaMEN/3x-ui" alt="Release"></a>
  <a href="https://github.com/SawaMEN/3x-ui/actions"><img src="https://img.shields.io/github/actions/workflow/status/SawaMEN/3x-ui/release.yml.svg" alt="Build"></a>
  <a href="#"><img src="https://img.shields.io/github/go-mod/go-version/SawaMEN/3x-ui.svg" alt="Go Version"></a>
  <a href="https://github.com/SawaMEN/3x-ui/releases/latest"><img src="https://img.shields.io/github/downloads/SawaMEN/3x-ui/total.svg" alt="Downloads"></a>
  <a href="https://www.gnu.org/licenses/gpl-3.0.en.html"><img src="https://img.shields.io/badge/license-GPL%20V3-blue.svg?longCache=true" alt="License"></a>
  <a href="https://pkg.go.dev/github.com/SawaMEN/3x-ui/v3"><img src="https://pkg.go.dev/badge/github.com/SawaMEN/3x-ui/v3.svg" alt="Go Reference"></a>
  <a href="https://docs.sanaei.dev"><img src="https://img.shields.io/badge/docs-docs.sanaei.dev-22d3ee" alt="Documentation"></a>
</p>

# 3X-UI

**3X-UI** is an open-source web control panel for managing Xray-core servers. This fork focuses on a clean, modern interface, practical administration tools, multi-node management, traffic accounting, and a Material 3-inspired visual design.

The panel interface is intentionally limited to **English and Russian**. Other bundled translations have been removed to keep the project smaller and easier to maintain.

> [!IMPORTANT]
> This project is intended for personal use and testing. Do not use it for illegal activity or in environments where you cannot independently verify the security and configuration of the software.

## Features

- **Multi-protocol inbounds** — VLESS, VMess, Trojan, Shadowsocks, WireGuard, AmneziaWG, TUIC v5, Hysteria2, MTProto, HTTP, SOCKS (Mixed), Dokodemo-door / Tunnel, and TUN.
- **Modern transports & security** — TCP (Raw), mKCP, WebSocket, gRPC, HTTPUpgrade, and XHTTP, with TLS, XTLS, and REALITY support.
- **Per-client management** — traffic quotas, expiry dates, IP limits, device limits, online status, share links, QR codes, and subscriptions.
- **Traffic statistics** — per inbound, per client, and per outbound statistics with reset controls.
- **Multi-node support** — manage multiple servers from a single panel.
- **Routing & outbounds** — custom routing rules, WARP/NordVPN/PIA integrations, load balancing, and outbound proxy chaining.
- **Built-in subscription server** — raw, JSON, and Clash output with custom page templates.
- **Telegram and Discord bots** for remote monitoring and management.
- **REST API** with scoped, optionally expiring tokens and an in-panel API reference.
- **Installable panel (PWA)** — use 3X-UI like an installed desktop or mobile application.
- **SQLite or PostgreSQL** storage.
- **Fail2ban integration** for IP-limit enforcement.
- **Material 3-inspired UI** — refreshed navigation, surfaces, cards, controls, spacing, and responsive behavior while keeping the existing Ant Design foundation.

## Screenshots

<details>
<summary>Click to expand</summary>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/01-overview-dark.png">
  <img alt="Overview" src="./media/01-overview-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/02-add-inbound-dark.png">
  <img alt="Inbounds" src="./media/02-add-inbound-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/03-add-client-dark.png">
  <img alt="Add client" src="./media/03-add-client-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/05-add-nodes-dark.png">
  <img alt="Nodes" src="./media/05-add-nodes-light.png">
</picture>

</details>

## Quick Start

Install the latest release:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh)
```

Install a specific version:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh) v3.7.0
```

Install the rolling development build:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh) dev-latest
```

During installation, a random username, password, and access path are generated. Run `x-ui` after installation to open the management menu.

Release archives are published with SHA-256 checksums, and the installer/updater verifies the archive before installation.

For complete installation, configuration, operations, and API documentation, see **[docs.sanaei.dev](https://docs.sanaei.dev)**.

## Supported Platforms

**Operating systems:** Ubuntu, Debian, Armbian, Fedora, CentOS, RHEL, AlmaLinux, Rocky Linux, Oracle Linux, Amazon Linux, Virtuozzo, Arch, Manjaro, Parch, openSUSE (Tumbleweed / Leap), Alpine, and Windows.

**Architectures:** `amd64` · `386` · `arm64` · `armv7` · `armv6` · `armv5` · `s390x`.

## Database Options

3X-UI supports:

- **SQLite** (default) — simple, zero-setup local storage.
- **PostgreSQL** — suitable for larger deployments and multi-node setups.

Example environment configuration:

```bash
XUI_DB_TYPE=postgres
XUI_DB_DSN=postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable
```

To migrate an existing SQLite database:

```bash
x-ui migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
```

## Docker

Start the default SQLite setup:

```bash
docker compose up -d
```

To use the bundled PostgreSQL service:

```bash
docker compose --profile postgres up -d
```

When Fail2ban is enabled, the container needs the network administration capabilities configured by the provided Compose file.

## Supported Languages

Only two panel languages are included:

- English (`en-US`)
- Русский (`ru-RU`)

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](/CONTRIBUTING.md) before opening an issue or pull request.

## AI-Assisted Development

Parts of this fork — including code changes, UI work, documentation, refactoring, and repository maintenance — were **generated and assisted by ChatGPT and other AI tools**.

AI assistance does not replace human review. Changes should be reviewed, tested, and validated before being used in production or distributed as a release.

## Acknowledgments

- [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) (GPL-3.0)
- [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat) (GPL-3.0)
- [3X-UI Manager](https://github.com/yukh975/3X-UI-Manager) (MIT)

## License

This project is licensed under **GPL-3.0**.
