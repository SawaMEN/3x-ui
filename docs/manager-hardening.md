# Manager reliability and recovery

Work remains on the `manager` branch. The changes preserve the panel's Runtime boundary and reuse its client/device policies.

## Subscription policy

The ordinary subscription server and `/bus/:token` share `ClientService.CheckSubscriptionDevice`, including HWID registration, returning-device admission, and limit headers. The manager checks expiry and traffic before rendering every supported format. Missing HWID is rejected when a device limit is configured.

HWID admission controls subscription retrieval. It is not hardware attestation: an exported protocol credential can still be copied. Separate device credentials with aggregate accounting require a distinct client lifecycle and are not implied by an HWID counter.

## Generations and recovery

- Failed generation removes listeners created by that operation, preserving the published generation and unexpired previous generations.
- Cleanup uses the persisted publication, including when saving a new publication fails.
- Previous generations remain available for `graceMinutes` (default 120, range 60–1440). Clients are advised to refresh hourly.
- The maintenance worker retires expired generations after a panel restart too. A disabled subscription is still eligible for maintenance.
- `cleanup_pending` distinguishes cleanup failure from an unpublished generation failure. Retrying cleanup does not create another generation.
- Interrupted operations are reconciled by cleanup rather than replaying the old generation operation.
- Four bounded workers serialize operations that share a destination core or relay. The queue's global lock is not held during network calls.

A grace period accommodates subscription refresh; it cannot guarantee that an indefinitely offline client will refresh before its old listener expires. Explicit subscription deletion removes all manager-owned generations.

## Readiness

`POST /panel/api/manager/preflight` returns a per-entry readiness report without creating inbounds. New nodes expose `core.protocolAvailability` through their authenticated status response. The report checks the installed protocol component, selected core, TLS identity and fixed-port availability on the destination. Runtime application remains the final authority because a port or file can change after a check.

The backend protocol catalogue generates the frontend catalogue through `npm run gen`. It drives client attachment, node eligibility, core compatibility and speed-limit support. Older nodes retain the existing version-based compatibility path; new reports take precedence.

The configured Reality pool is separate from TLS identity. Fresh scanner results are preferred; Reality readiness is checked from the destination. Certificate-backed protocols retain their own domain and certificate pair.

## Encrypted complete recovery

The existing database download remains available. The additional CLI recovery bundle includes a consistent database snapshot, configured local certificate/key files, the node-token keyring (including rotation keys), environment metadata and panel/core versions. Missing required files fail the export. Binaries are not embedded; reinstall the recorded compatible components.

Use a password file with mode 0600 containing at least 16 bytes. Keep the password separately from the bundle.

```bash
x-ui recovery export --file /root/panel.recovery --password-file /root/recovery-password
x-ui recovery check --file /root/panel.recovery --password-file /root/recovery-password
x-ui recovery restore --file /root/panel.recovery --out /root/panel-restored --password-file /root/recovery-password
```

`XUI_RECOVERY_PASSWORD` is also supported. Passwords are not accepted as command-line values. The format uses scrypt-derived AES-256-GCM with a new salt and nonce per export, authenticated headers and SHA-256 file hashes.

Restore verifies authentication and every file before extraction. It only writes into a **new** private directory, never into the running installation or paths supplied by the archive. Review `manifest.json`, reinstall compatible components, stop the destination panel, and place files at their recorded locations. PostgreSQL backups use the existing `pg_restore` procedure. Restore the keyring with mode 0600 before enabling encrypted token storage. Recreate certificate renewal automation separately if it belongs to an external ACME installation.

## Interface and verification

The manager list is paginated and does not include full protocol settings/private keys. The editor loads the complete recipe through an owner-scoped detail endpoint. Polling uses 2 seconds while work is pending and 30 seconds when idle. The operation journal retains failure and cleanup states.

The main CI includes pushes to `manager`. Regression tests exercise device admission, failed-generation cleanup, grace-period expiry, owner-scoped compact listing and encrypted recovery authentication/extraction. Live tests use actual locally launched panel/core processes; they do not certify reachability through a particular operator's network.
