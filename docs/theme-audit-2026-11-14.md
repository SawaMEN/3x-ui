# Panel theme audit

## Scope

Reviewed shared styles and page/component styles across the panel, login and
subscription entries. All six palettes remain supported: light, dark,
ultra-dark, colorful, blue-gray and cyberpunk. White QR code backgrounds and
cover-site preview documents are intentional and are preserved.

## Fixed causes

1. Static `Modal`, `message` and `notification` calls create independent React
   roots. They previously missed the selected Ant Design theme. A shared
   `holderRender` now supplies locale and subscribes to theme changes. Already
   open dialogs retain their input state when switching theme or low-power mode.
2. Dialog, popover, tooltip and feedback styles targeted obsolete Ant Design
   surface classes. Updated them to the installed v6 `container`/`notice`
   elements, including editor and mobile log dialog styles.
3. Select styles targeted the removed inner `selector`. They now target the
   v6 select root; hover, focus, disabled and palette rules follow that root.
   Collapse content padding now targets its v6 body.
4. Routing cards relied on page-local `--bg-card` and fell back to white in
   portals and pages without that variable. They now use the shared surface
   palette directly.
5. The Persian calendar had only three hardcoded palettes and a fixed blue
   accent. It now derives popup colors from the active Ant Design config and
   uses shared palette variables for the input and clear button.
6. The main panel entry did not include the shared Ant Design locale/form
   provider used by the login/subscription entries. Added the same provider.

## Verification

- Component regressions exercise static feedback in all six palettes, live
  switching of an open confirmation, preserving edited input, and low-power mode.
- Existing storage-denied, native scrollbar and component locale tests are
  included in the focused local check: 10 assertions/tests passed.
- Frontend lint, deprecated API check, formatting, TypeScript and Vite build.
- `frontend/tools/theme-browser-audit.mjs` mounts the actual page and dialog
  components against mocked APIs. It checks fifteen pages and nine dialog/control
  variants at 390px and 1440px, across all six palettes, plus live static-dialog
  switching and low-power mode (292 rendered checks). CI runs it with Chromium.
  These fixtures do not validate production server data or every interaction.

Local execution of Chromium is unavailable because this workspace forbids its
Unix socket. Browser acceptance must therefore be read from the CI job, rather
than inferred from the local component tests.
