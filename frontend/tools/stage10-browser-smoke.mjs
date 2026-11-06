// Real Chromium viewport smoke for the served login bundle and session recovery.
// This runs against Vite with intercepted API responses; it does not claim a
// production backend login or a physical-device acceptance test.
import assert from 'node:assert/strict';
import { chromium } from 'playwright';

const root = process.env.STAGE10_BASE_URL || 'http://127.0.0.1:5173';
const browser = await chromium.launch({ headless: true });
const viewports = [
  { name: 'compact-phone', width: 320, height: 568 },
  { name: 'mobile-phone', width: 375, height: 667 },
  { name: 'tablet', width: 768, height: 1024 },
  { name: 'desktop', width: 1366, height: 768 },
];
try {
  for (const viewport of viewports) {
    const context = await browser.newContext({
      viewport: { width: viewport.width, height: viewport.height },
      reducedMotion: 'reduce',
      isMobile: viewport.width <= 768,
      hasTouch: viewport.width <= 768,
    });
    const page = await context.newPage();
    await page.route('**/getTwoFactorEnable', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, obj: false }),
      });
    });

    const navigation = await page.goto(root + '/login.html', { waitUntil: 'domcontentloaded' });
    assert.equal(navigation?.status(), 200, 'login document must load');
    await page.locator('.login-card input[autocomplete="username"]').waitFor({ timeout: 20_000 });
    await page.locator('.login-card input[autocomplete="current-password"]').fill('test-value');

    const geometry = await page.evaluate(() => {
      const card = document.querySelector('.login-card');
      const submit = document.querySelector('.login-card button[type="submit"]');
      const toolbar = document.querySelector('.login-toolbar');
      if (
        !(card instanceof HTMLElement) ||
        !(submit instanceof HTMLElement) ||
        !(toolbar instanceof HTMLElement)
      )
        throw Error('login UI missing');
      const cardRect = card.getBoundingClientRect();
      const submitRect = submit.getBoundingClientRect();
      const toolbarRect = toolbar.getBoundingClientRect();
      return {
        bodyScrollWidth: document.documentElement.scrollWidth,
        viewportWidth: window.innerWidth,
        cardLeft: cardRect.left,
        cardRight: cardRect.right,
        submitWidth: submitRect.width,
        toolbarRight: toolbarRect.right,
        cardMinHeight: cardRect.height,
      };
    });
    assert.ok(
      geometry.bodyScrollWidth <= geometry.viewportWidth + 1,
      `${viewport.name}: horizontal overflow: ${JSON.stringify(geometry)}`,
    );
    assert.ok(
      geometry.cardLeft >= -1 && geometry.cardRight <= viewport.width + 1,
      `${viewport.name}: login card outside viewport: ${JSON.stringify(geometry)}`,
    );
    assert.ok(
      geometry.submitWidth >= 100 && geometry.toolbarRight <= viewport.width + 1,
      `${viewport.name}: login actions inaccessible: ${JSON.stringify(geometry)}`,
    );

    await page.locator('.login-card input[autocomplete="username"]').fill('admin');
    await page.locator('.login-card input[autocomplete="current-password"]').focus();
    assert.equal(
      await page.locator('.login-card input[autocomplete="username"]').inputValue(),
      'admin',
      `${viewport.name}: editable login input`,
    );
    console.log(`PASS ${viewport.name} ${viewport.width}x${viewport.height}: responsive login`);
    await context.close();
  }

  // The frontend must navigate to the login page after an authenticated
  // API request returns 401. This exercises the actual browser HTTP client.
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.route('**/getTwoFactorEnable', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: '{"success":true,"obj":false}',
    });
  });
  await page.route('**/panel/api/server/status', async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: '{"success":false}',
    });
  });
  await page.goto(root + '/login.html');
  await page.locator('.login-card').waitFor();
  const wentHome = page.waitForURL((u) => u.pathname === '/', { timeout: 15_000 });
  await page.evaluate(() => {
    void import('/src/api/http-init.ts')
      .then(({ httpRequest }) => httpRequest('GET', '/panel/api/server/status'))
      .catch(() => {});
  });
  await wentHome;
  console.log('PASS browser session expiry: 401 causes safe navigation to login');
  await context.close();
} finally {
  await browser.close();
}
