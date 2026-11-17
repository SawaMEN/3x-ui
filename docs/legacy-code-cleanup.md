# Unreachable legacy code cleanup

The cleanup is based on production call-graph analysis (`golang.org/x/tools/cmd/deadcode v0.51.0`), frontend dependency analysis (`knip 6.45.1`) and repository-wide reference checks. Analyzer findings alone are not a deletion list: reflected callbacks, test seams, generated contracts, migration readers, public assets and documented CLI tools remain legitimate entry points.

Removed:

- The unused `internal/util/tokenaead` transport and its isolated tests. No production package imported it.
- The disconnected sing-box `ConnectionAPIClient`, its protobuf decoder/traffic fallback and transport-specific tests. Current session/traffic operations use the owner runtime through `internal/hiddify`. The shared `singbox.ActiveSession` wire DTO remains in place.
- NaiveProxy's uncalled standalone `Ensure`/`Sync`/`RenderConfig` reconciler and launcher. `internal/protocolserver` owns current listeners. The component downloader, version/status API and safe shutdown of PID-file processes from older installations remain.
- Five unreachable frontend modules: the old core-capability hook, standalone sing-box utility page, old VK TURN dashboard management/status components and an unreferenced mask-preset module. Current VK TURN settings/logs and the main native-core pages remain.
- Uncalled `DBInbound` cache/accessor methods and the no-op `RandomUtil` import reference. Normal JSON serialization retains the same public inbound fields.
- AmneziaWG's ineffective `routeThroughXray` opt-in from the retired kernel-module architecture, including its runtime copy and frontend/generated declaration. Old stored flags remain harmless unknown JSON fields; a regression test verifies that both old values produce the same embedded instance. MTProto's active setting with the same name remains.

Compatibility logic that still loads saved databases, clients, imported subscriptions, firewall rules and old node configurations is retained. Historical audit inventories are not rewritten.
