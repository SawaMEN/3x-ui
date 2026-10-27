# Project audit: 27 October 2026

This audit fixes confirmed failures, removes unused internal compatibility code,
and tightens the project's checks. It does **not** claim 200 independent bugs:
automated warnings, formatting changes, and test cases are separate categories.

## Functional corrections

| Area                    | Correction                                                                                                                                                                                                  | Verification                                                                                                                               |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Delayed subscriptions   | Zero or negative traffic snapshots cannot start the subscription clock.                                                                                                                                     | `TestAuditZeroTrafficDoesNotStartDelayedSubscription`                                                                                      |
| Traffic aggregation     | A negative first snapshot cannot cancel later positive upload/download deltas. Adapter snapshots stay immutable.                                                                                            | `TestAuditNegativeFirstDeltaCannotCancelPositiveTraffic`                                                                                   |
| Expiry arithmetic       | An extreme negative duration cannot wrap into a negative absolute deadline.                                                                                                                                 | `TestAuditExtremeDelayedExpiryRemainsPositive`                                                                                             |
| Damaged client settings | Null client entries cannot panic during activation or renewal. Missing client arrays return an error.                                                                                                       | `TestAuditNullClientsDoNotPanicDuringActivation`, `TestAuditNullRenewalAndSaturatedRenewalCount`, `TestAuditInvalidAddPayloadReturnsError` |
| Durable activation      | Inbound-save and canonical-client synchronization failures propagate to the transaction instead of acknowledging partial activation.                                                                        | `TestAuditDelayedActivationDatabaseFailureRollsBackTraffic`                                                                                |
| Durable renewal         | Client synchronization errors abort renewal instead of persisting inconsistent projections.                                                                                                                 | Existing transactional and renewal suites                                                                                                  |
| Counter repair          | Negative persisted counters are repaired without dropping new usage. Saturated counters still update online status.                                                                                         | `TestAuditNegativeStoredCountersDoNotLoseNewUsage`, `TestAuditSaturatedTrafficStillUpdatesOnline`                                          |
| Database scale          | Traffic lookups, delayed-start lookups, and activation/renewal saves are batched. Duplicate sources consume one lookup key.                                                                                 | `TestAuditTrafficQueriesRespectSQLiteVariableBudget`                                                                                       |
| Renewal arithmetic      | Fixed-period catch-up is computed in constant time. Period multiplication, deadlines, allowance checks, and renewal counters are protected from overflow. Invalid periods are rejected at write boundaries. | `TestRenewalIntervalBoundsAndAllowances`, existing calendar/cap tests                                                                      |
| OxiDNS                  | Local status and health probes honor caller cancellation.                                                                                                                                                   | Package tests and context-aware dialers                                                                                                    |
| IP checks               | The fail2ban detection subprocess has a deadline.                                                                                                                                                           | Job suite                                                                                                                                  |
| Telemt startup          | Both startup and cleanup errors remain discoverable through wrapping.                                                                                                                                       | Manager suite and `errorlint`                                                                                                              |
| HTTP utilities          | HTTP 204/205 empty responses are accepted; a string `false` is not a success flag. Null/undefined thrown values cannot break error handling.                                                                | `http-util.test.ts`                                                                                                                        |
| Request options         | GET honors parameters supplied in options. Silent DELETE and intentional cancellation do not emit unwanted errors.                                                                                          | `http-util.test.ts`                                                                                                                        |
| Log viewers             | All four viewers cancel obsolete work. Late replies cannot replace newer data or update a closed viewer. Automatic polls do not overlap and pause in hidden tabs.                                           | `refresh-request.test.tsx`, `log-viewer-requests.test.tsx`                                                                                 |
| Log payloads            | Array-shaped data is checked before rendering; malformed peer timestamps cannot crash AmneziaWG log downloads.                                                                                              | Typecheck, shared request tests, viewer integration tests                                                                                  |
| CI                      | Go lint failures now fail the job. The deprecated frontend API check is part of CI and the local lint target.                                                                                               | Workflow configuration                                                                                                                     |

## Legacy removal and simplification

- Removed the unused `xray.Traffic` and `xray.ClientTraffic` aliases and old
  `GenXrayInboundConfig` / `CommitXrayTraffic` wrappers. Tests use current names.
- Removed the unused relay-port allocator, duplicate heartbeat-event builders,
  dead host/client/fallback helpers, and unused fields.
- Consolidated positive-traffic aggregation, bounded database lookups, HTTP
  request/error handling, and log-viewer request lifecycles.
- Replaced deprecated `parser.ParseDir` with deterministic source-file parsing;
  generated API files remain unchanged.
- Migrated 50 deprecated frontend API uses and removed the existing 41 reported
  Go lint diagnostics. Import/format normalization is maintenance, not a bug count.
- Kept active database and import migrations: existing installations and backups
  still require those readers to preserve stored clients and settings.

## Validation

Required checks include the full shuffled Go suite, unit/component frontend
suite, targeted race tests for accounting and renewal, full Go/frontend lint,
deprecated API lint, typecheck, formatter checks, API generation freshness,
production frontend build, Storybook build, and the repository's CI checks.

Regression tests preserve earlier accounting and renewal tests and explicitly
check rollback, null inputs, extreme integers, SQL variable budgets, late replies,
unmount cancellation, polling overlap, hidden tabs, and failed requests.
