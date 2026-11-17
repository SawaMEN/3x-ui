// Optional acceptance check invoked by TELEMT_SITE_BROWSER_SCRIPT in native Go tests.
import { chromium } from '../../frontend/node_modules/playwright/index.mjs';
const [url, host, name] = process.argv.slice(2);
const browser = await chromium.launch({
  executablePath: process.env.CHROMIUM_BINARY || chromium.executablePath(),
  headless: true,
  args: ['--no-sandbox'],
});
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const failures = [];
  page.on('pageerror', (error) => failures.push(error.message));
  page.on('console', (msg) => {
    if (msg.type() === 'error') failures.push(msg.text());
  });
  page.on('requestfailed', (req) => failures.push(`${req.url()}: ${req.failure()?.errorText}`));
  await page.route('**/*', async (route) => {
    if (new URL(route.request().url()).origin !== new URL(url).origin) {
      failures.push(`Unexpected external resource: ${route.request().url()}`);
      await route.abort();
      return;
    }
    const response = await route.fetch({ headers: { ...route.request().headers(), host } });
    if (response.status() >= 400)
      failures.push(`HTTP ${response.status()}: ${route.request().url()}`);
    await route.fulfill({ response });
  });
  await page.goto(url, { waitUntil: 'networkidle', timeout: 15000 });
  const result = await page.evaluate(() => ({
    title: document.title,
    styles: document.styleSheets.length,
    expectedStyles: document.querySelectorAll('style,link[rel="stylesheet"]').length,
    body: document.body.innerText.length,
    ready: document.body.dataset.ready,
    color: document.querySelector('h1')
      ? getComputedStyle(document.querySelector('h1')).color
      : null,
    canvas: !!document.querySelector('canvas'),
  }));
  if (!result.body || (result.expectedStyles > 0 && result.styles < 1))
    failures.push(`Empty or unstyled cover page: ${JSON.stringify(result)}`);
  if (name.includes('WithBasePath/imported') && result.ready !== 'yes')
    failures.push('Imported page script did not execute');
  if (name.includes('WithBasePath/imported') && result.color !== 'rgb(20, 40, 60)')
    failures.push('Imported stylesheet was not applied');
  if (await page.locator('#btn-sound').count()) {
    const before = await page.locator('#btn-sound').textContent();
    await page.locator('#btn-sound').click();
    if ((await page.locator('#btn-sound').textContent()) === before)
      failures.push('Game controls did not respond');
  }
  if (failures.length) throw new Error(failures.join('\n'));
  console.log(JSON.stringify({ name, ...result }));
} finally {
  await browser.close();
}
