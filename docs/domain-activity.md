# Domain activity (stat branch)

The panel can show **recent domains contacted by Xray clients**, grouped by
client email. This is destination metadata, **not** the browser's current tab,
full URL, or proof that a connection is still open.

## Enable

1. Select the **Xray** core.
2. Enable **Access Log** in Xray settings and ensure Xray is running.
3. Open the dashboard's **Domain activity** button and explicitly enable it.
4. Optionally open a client's activity from the **Clients** table actions or
   mobile client menu. The client filter is pre-filled.

The feature is **off by default** (setting `domainActivityEnabled=false`).
Disabling it immediately stops the activity API from returning destinations.
It does **not** turn off, redact, or delete the separately configured Xray
access log. Administrators are responsible for access-log retention and
informing users where required.

## Data model and limitations

- Only the Xray core is supported. The other cores report an unsupported
  state; they do not silently fall back to Xray logs.
- The API reads up to 2,000 existing log entries per request and returns only
  entries timestamped within the past **120 seconds**.
- A maximum of **30 domains per client** and **300 domains overall** is returned.
  Each row contains client identifier, domain, latest logged connection time,
  and number of logged connections in the window.
- Duplicate destinations are aggregated; raw IP destinations and malformed
  hostnames are omitted. Blocked log entries are filtered out.
- A request every **3 seconds** refreshes the modal only while it is open;
  no background activity collector or separate history database exists.
- Domain attribution requires the client's email in the Xray access log.
  TLS SNI, HTTP Host and QUIC sniffing may help Xray obtain domain names
  where the client initially connects to an IP address.
- ECH, IP-only destinations, encrypted application protocols, split tunneling,
  or traffic bypassing Xray can hide activity. Counts are connections, **not**
  page visits or user actions. Older connections can remain open even after
  they leave the 120-second window.

## API

Authenticated panel API routes (existing authorization, token scopes and CSRF
protection apply):

- `GET /panel/api/server/domainActivity` returns
  `{enabled, supported, accessLogEnabled, windowSeconds, entries}`.
- `POST /panel/api/server/domainActivity/config` with JSON
  `{"enabled":true}` or `{"enabled":false}` updates the persisted opt-in.
  Enabling requires the Xray core and an enabled access log.

No raw access-log lines, client IP addresses, URL paths, or HTTP request bodies
are returned by the activity endpoint.

## Checks

Go unit tests in `internal/web/service/domain_activity_test.go` cover
destination validation, IP exclusion, blocked events, stale/future timestamps,
deduplication and ordering. Run `go test ./internal/web/service ./internal/web/controller`
and `cd frontend && npm run build` in a full checkout before deploying.
