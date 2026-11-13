// Run against `npm run dev`. Uses isolated browser contexts and mocked panel
// APIs: verifies rendered CSS and portals, without contacting production nodes.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';

const base = process.env.THEME_AUDIT_BASE_URL || 'http://127.0.0.1:5173';
const modes = ['light', 'dark', 'ultra-dark', 'colorful', 'blue-gray', 'cyberpunk'];
const pages = {
  dashboard: 'index/IndexPage',
  login: 'login/LoginPage',
  subscription: 'sub/SubPage',
  inbounds: 'inbounds/InboundsPage',
  clients: 'clients/ClientsPage',
  groups: 'groups/GroupsPage',
  nodes: 'nodes/NodesPage',
  manager: 'manager/ManagerPage',
  hosts: 'hosts/HostsPage',
  settings: 'settings/SettingsPage',
  xray: 'xray/XrayPage',
  singbox: 'singbox/SingBoxPage',
  olcrtc: 'olcrtc/OlcRTCPage',
  'api-docs': 'api-docs/ApiDocsPage',
  telemt: 'telemt/TelemtPage',
};

const browser = await chromium.launch({
  headless: true,
  executablePath: process.env.THEME_AUDIT_CHROMIUM,
});
let checks = 0;
try {
  for (const width of [390, 1440]) {
    const context = await browser.newContext({
      viewport: { width, height: 1000 },
      reducedMotion: 'reduce',
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', (error) => {
      errors.push(error.message);
      console.error('Browser page error:', error.message);
    });
    await page.route('**/__theme_audit__', (route) =>
      route.fulfill({
        contentType: 'text/html',
        body: `<!doctype html><html><head><script type="module">
          import RefreshRuntime from '/@react-refresh';
          RefreshRuntime.injectIntoGlobalHook(window);
          window.$RefreshReg$ = () => {};
          window.$RefreshSig$ = () => type => type;
          window.__vite_plugin_react_preamble_installed__ = true;
        </script></head><body><div id="app"></div></body></html>`,
      }),
    );
    await page.route('**/panel/api/**', (route) => {
      const path = new URL(route.request().url()).pathname;
      let obj = [];
      if (/setting\/all/.test(path)) obj = {};
      if (/server\/status/.test(path)) obj = { xray: { state: 'running' } };
      if (/telemt\/status/.test(path))
        obj = {
          installed: false,
          active: false,
          enabled: false,
          configured: false,
          version: '',
          latestVersion: '',
          updateAvailable: false,
          mekoEnabled: false,
          webProxy: {
            enabled: false,
            supported: false,
            frontend: 'nginx',
            frontendActive: false,
            carrier: 'https',
            carriers: [],
            aliases: [],
            links: [],
            domain: '',
            basePath: '',
            defaultDomain: '',
            link: '',
            error: '',
            carrierNegotiationSupported: false,
            nginxInstalled: false,
            nginxActive: false,
            certificateReady: false,
            certificateFile: '',
            listenPort: 15080,
            port443Available: true,
            port443Owner: '',
          },
        };
      if (/singbox\/config/.test(path))
        obj = {
          config: { inbounds: [], outbounds: [], route: {} },
          managedSections: [],
          inboundTags: [],
          subscriptionTags: [],
          warnings: [],
          running: true,
          version: 'test',
        };
      if (/csrf-token/.test(path)) obj = 'theme-audit';
      if (/openapi.json/.test(path))
        return route.fulfill({
          contentType: 'application/json',
          body: '{"openapi":"3.0.0","info":{"title":"Theme audit","version":"1"},"paths":{}}',
        });
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ success: true, obj }),
      });
    });
    await page.goto(base + '/__theme_audit__');
    await page.evaluate(async () => {
      const { mountThemeAudit } = await import('/tools/theme-browser-fixture.jsx');
      await mountThemeAudit();
    });
    await page.waitForFunction(() => typeof window.audit?.setMode === 'function');

    async function checkSurfaces(label, dark) {
      await page.waitForTimeout(120);
      const result = await page.evaluate(() => {
        const selectors =
          '.ant-card,.ant-modal-container,.ant-popover-container,.ant-select,.ant-select-dropdown,.ant-picker-panel-container,.ant-input,.ant-message-notice,.ant-notification-notice,.rule-card,.jdp-wrap input,.cm-editor';
        const surfaces = [...document.querySelectorAll(selectors)]
          .filter((el) => el.getBoundingClientRect().width > 0)
          .map((el) => ({ className: el.className, bg: getComputedStyle(el).backgroundColor }));
        return { surfaces, text: document.body.textContent };
      });
      assert.ok(!/Unexpected Application Error/i.test(result.text), `${label}: render error`);
      if (dark) {
        for (const surface of result.surfaces) {
          const c = surface.bg.match(/[\d.]+/g)?.map(Number) || [];
          if (c.length < 3 || (c.length === 4 && c[3] < 0.5)) continue;
          assert.ok(
            Math.max(...c.slice(0, 3)) < 220 || Math.min(...c.slice(0, 3)) < 150,
            `${label}: light surface ${JSON.stringify(surface)}`,
          );
        }
      }
      assert.equal(errors.length, 0, `${label}: ${errors.join('; ')}`);
      checks++;
    }

    for (const mode of modes) {
      await page.evaluate((mode) => window.audit.setMode(mode), mode);
      await page.waitForFunction((mode) => document.documentElement.dataset.theme === mode, mode);
      const dark = ['dark', 'ultra-dark', 'blue-gray', 'cyberpunk'].includes(mode);
      for (const [name, path] of Object.entries(pages)) {
        await page.evaluate((path) => window.audit.showPage(path), path);
        await checkSurfaces(`${width}/${mode}/${name}`, dark);
      }
      for (const kind of [
        'inbound',
        'client',
        'node',
        'outbound',
        'rule',
        'backup',
        'security',
        'controls',
        'static',
      ]) {
        await page.evaluate((kind) => window.audit.showModal(kind), kind);
        await page.locator('.ant-modal-container').last().waitFor();
        await checkSurfaces(`${width}/${mode}/${kind}`, dark);
      }
      await page.evaluate(() => window.audit.clearFeedback());
      console.log(`PASS ${width}px ${mode}: ${Object.keys(pages).length} pages + 9 dialogs`);
    }
    // Switch the theme while a static dialog is open: it must retain its root
    // and content while receiving dark tokens rather than keeping light ones.
    await page.evaluate(() => {
      window.audit.setMode('light');
    });
    await page.evaluate(() => window.audit.showModal('static'));
    await page.locator('.ant-modal-confirm').waitFor();
    await page.evaluate(() => {
      window.audit.setMode('cyberpunk');
    });
    await checkSurfaces(`${width}/open-static-theme-switch`, true);
    await page.evaluate(() => window.audit.toggleLowPower());
    await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
    await checkSurfaces(`${width}/low-power-static-theme-switch`, true);
    await context.close();
  }
  console.log(`PASS ${checks} rendered theme checks`);
} finally {
  await browser.close();
}
