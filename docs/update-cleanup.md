# Pre-update disk cleanup

Panel installation and updates acquire the shared download and migration locks
before scanning disposable update files, before downloading a new archive, and
before checking the free-space budget. A recent interruption is sufficient:
there is no six-hour waiting period for a known orphaned download.

The scan is restricted to updater-owned filenames directly beneath the
installation parent and the installation root:

- temporary panel archives, checksum partners and partial downloads;
- legacy AMD64/ARM64 release archives and their partial/checksum files;
- temporary migration helpers and unpacked release staging directories;
- abandoned modified-core staging directories;
- upgrade snapshots explicitly marked as successfully verified.

Symlink candidates and symlink verification markers are skipped. Unverified
rollback snapshots, core backups, the working binaries, database, certificates,
user files and unrelated temporary directories are retained. The scan is
non-recursive; deleting an owned staging directory does not follow symlinks
within it. Managed concurrent installers use the same locks, so their files
cannot be removed by another update.

The independent modified-core updater also removes its abandoned staging and
PID-suffixed replacement payloads before downloading. It preserves recovery
backups and active asset filenames. Check-only runs perform no cleanup.

Each removal and its allocated size is written to update output, followed by a
summary of removed artifacts and reclaimed KiB. Panel updates additionally
report free space before and after cleanup. The existing preliminary and exact
panel disk-space checks remain in place; insufficient space aborts before the
running installation is replaced.

The first update using these scripts performs cleanup itself; no scheduled
cleanup job or manual deletion is required.
