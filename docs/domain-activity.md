# Domain activity (stat branch)

The panel displays **domains used by connected clients** for each of the three
selectable proxy cores. This is metadata about VPN connections, **not** a full
URL or a browser tab title.

## Supported cores

| Selected core | Source | Meaning |
|---|---|---|
| `sing-box` (bundled) | Existing authenticated Clash API session snapshot | Currently active connections |
| `sing-box-upstream` (official) | Existing Clash API `GET /connections` | Currently active connections |
| `xray` (standalone) | Existing Xray Access Log | Recently logged connections (past 120 seconds) |

For both sing-box variants, the panel reuses
`SingBoxService.ActiveSessions(ctx)` / `hiddifySessions(ctx)` and reads
`session.User`, `session.Domain` (or the domain-form destination), and
`session.ID`. The current snapshot contains the **number of active
connections per user and domain**, not a count of visits.

The official sing-box build needs its optional Clash API enabled and
accessible locally (the generated config normally supplies
`experimental.clash_api.external_controller`). When that API is disabled,
unreachable, or does not expose authenticated users/domain metadata, the
panel **reports a read error or has no attributable domains**. It does not
fabricate user associations or silently fall back to inspecting packets.
Only local selected-core sessions are included; independent protocol sidecars
and remote nodes do not automatically provide domain-level data.

## Enable

1. Select the desired core in **Settings → Additional settings → Proxy Core**.
2. For **Xray only**, enable the Xray **Access Log**.
3. Open the dashboard's **Domain activity** button and enable the optional
   monitoring switch.
4. Optionally open a client's domain activity from the **Clients** actions or
   mobile menu. The client filter is prefilled.

The feature is **disabled by default** (`domainActivityEnabled=false`).
Disabling it immediately stops API responses from showing destinations.
No new database or background collector is started.

## Limits and privacy

- Active sing-box connections are refreshed every **3 seconds**, while the
  viewer is open and visible. Rows show *time of observation*, not the time
  that a website was opened. Active sessions disappear on subsequent
  successful snapshots after they close.
- Xray reads at most 2,000 lines from its existing access log and returns only
  events from the last **120 seconds**.
- All engines return up to **30 domains per client**, **300 overall**.
  A maximum of 20,000 native session records is examined per request.
- Duplicate session IDs are ignored; IP-only and invalid hostnames, absent
  client identifiers, and blocked Xray events are omitted.
- ECH, DNS-only traffic, encrypted protocols, IP-based routing and traffic
  bypassing the core may hide domains. A displayed domain may be a background
  resource requested by an app instead of a site the user intentionally opened.
- Disabling this feature does **not** delete existing Xray access logs. Their
  retention is configured separately. Consider user notices, least-privilege
  access and applicable data-protection requirements.

## API

Authenticated panel API routes (existing authorization, token scopes, CSRF):

- `GET /panel/api/server/domainActivity` returns
  `{enabled, supported, accessLogEnabled, core, mode, windowSeconds, entries}`.
  `mode="active"` means current sing-box sessions; `mode="recent"` means
  Xray's recent access-log events.
- `POST /panel/api/server/domainActivity/config` with JSON
  `{"enabled":true}` or `{"enabled":false}` persists the opt-in.
  Enabling Xray requires a configured access log; sing-box does **not**.

The activity endpoint returns no raw access-log lines, client IP addresses,
HTTP paths or request bodies. Responses use `Cache-Control: no-store`.

## Verification

Go tests in `internal/web/service/domain_activity_test.go` and
`internal/web/service/domain_activity_native_test.go` cover Xray and
sing-box aggregation. Run
`go test ./internal/web/service ./internal/web/controller` and
`cd frontend && npm run typecheck && npm run build:ci`
in a full checkout before deployment. Live testing with both sing-box
variants is needed to verify that the selected binary exposes the Clash
connection API and its user/domain metadata.
