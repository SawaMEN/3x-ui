# Routing list catalog

Open **Core settings → Routing → List catalog**. Choose a category, select an
outbound and add its rule. Save the core configuration to apply the route.
Existing specific rules keep priority; additions go before the first enabled
catch-all. Reassigning a catalog item updates its existing rule in place. Rules
can be reordered, disabled, scoped to clients/inbounds or removed in **Rules**.

Built-in categories include YouTube, Telegram, Discord, AI services, OpenAI,
Google, GitHub, Netflix, Spotify, games, social networks and advertising. These
use categories from the installed GeoSite database and the panel's existing
geodata updater. Missing categories are shown as unavailable. The content viewer
uses the existing geodata browser. No Routerich account or device API is needed.

## Custom sources

Add an HTTPS URL serving plain TXT domains, IP/CIDR entries or hosts rows.
`domain:`, `full:`, `keyword:` and `regexp:` domain matchers are supported.
Plain domains are normalized as suffix matches (`domain:example.com`); CIDRs
are masked, duplicates removed and entries sorted. Source files with recursive
`include:` directives, HTML, JSON, SRS or AdGuard syntax are not supported.
Convert these to a plain list before adding them.

Limits: 4 MiB per response, 50,000 entries, 16 KiB per line, 100 sources.
Intervals are 1–168 hours, or 0 for manual updates. Source creation/editing
downloads and validates the list before saving. A cron task checks due sources
every minute. Invalid, oversized, empty or unavailable updates keep the last
working cache and expose the error and last successful update in the catalog.
HTTPS certificates are verified; redirects are bounded and cannot downgrade to
HTTP; direct downloads use the existing private-address/DNS-rebinding guard.

Source changes are saved immediately. Rule assignments are staged with other
core settings until the configuration is saved. An updated source used by the
saved configuration marks the runtime for normal validated reconciliation.

Custom rules store `routingListId` rather than copying thousands of entries into
the editable template. Runtime generation loads cached lists without network
I/O, replacing each reference with separate domain and IP rules (OR semantics),
preserving rule order, target and client/inbound constraints. Native/hiddify
generation reads sources and settings in the same database snapshot. Missing
or invalid caches fail generation instead of becoming catch-all rules. Disabled
references generate no runtime rules. Domain/IP fields of a managed reference
must be edited in its source, not in the rule editor.

The current hiddify-core editor stores built-in selections as
`routingListCategory` references. Runtime generation expands them into native
domain matchers. The editor restores the saved references when reading an
expanded runtime configuration, keeping categories and custom sources live
across unrelated settings edits. The legacy compatibility editor uses ordinary
`geosite:` rules for built-in categories.

To delete a source, remove its rules and save the configuration first. Source
IDs are local to the panel database: JSON rule exports and presets retain these
references and require matching sources when imported elsewhere. Database
backups retain both source definitions and cached content. This catalog governs
server routing; it does not install client-side rules on phones or routers.

## API

All endpoints use the existing authenticated panel API:

| Method | Path under `/panel/api/xray/routingCatalog` | Behavior                                                 |
| ------ | ------------------------------------------- | -------------------------------------------------------- |
| GET    | `/categories`                               | Built-in IDs/tokens and installed availability           |
| GET    | `/list`                                     | Custom sources, counts, interval, timestamp and error    |
| GET    | `/:id/entries`                              | Cached domain and IP entries                             |
| POST   | `/save`                                     | JSON `{id, name, url, updateInterval}`; 0/new ID creates |
| POST   | `/:id/refresh`                              | Validate and refresh; result indicates content change    |
| POST   | `/:id/delete`                               | Delete an unreferenced source                            |

`updateInterval` is in seconds (0 or 3600–604800). `/save` has a 16 KiB request
limit. The API uses the standard `{success, msg, obj}` response envelope.
