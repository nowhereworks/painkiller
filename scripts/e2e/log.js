// Browser-only operation log regression; no server or lab required.
const fs = require('fs');
const {launch, check} = require('./lib');

(async () => {
  const browser = await launch();
  try {
    const page = await browser.newPage({viewport: {width: 1200, height: 800}});
    await page.setContent(fs.readFileSync('web/index.html', 'utf8')
      .replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, '')
      .replace(/<link\b[^>]*>/g, ''));
    await page.addStyleTag({path: 'web/style.css'});
    await page.evaluate(() => {
      window.logOutput = Array.from({length: 200}, (_, i) => `Provisioning line ${i}`).join('\n');
      window.fetch = async url => ({
        json: async () => ({phase: 'preparing'}),
        text: async () => window.logOutput
      });
    });
    await page.addScriptTag({path: 'web/app.js'});
    await page.waitForFunction(() => document.getElementById('log').textContent.includes('line 199'));
    const metrics = () => page.evaluate(() => {
      const log = document.getElementById('log');
      return {
        height: log.clientHeight,
        bottom: Math.abs(log.scrollHeight - log.clientHeight - log.scrollTop) <= 1,
        fits: log.getBoundingClientRect().bottom <= innerHeight,
        pageFits: document.documentElement.scrollHeight <= innerHeight,
        editable: log.isContentEditable
      };
    });
    const initial = await metrics();
    check(initial.bottom && initial.fits && initial.pageFits && !initial.editable, 'initial log layout or scrolling failed');
    await page.evaluate(async () => {
      document.getElementById('log').scrollTop = 0;
      await refresh();
    });
    check(!await page.locator('#log').evaluate(el => el.scrollTop), 'unchanged output should preserve manual scrolling');
    await page.evaluate(async () => { window.logOutput += '\nNew output'; await refresh(); });
    check((await metrics()).bottom, 'new output did not auto-scroll');
    await page.locator('#log').focus();
    await page.keyboard.type('cannot edit');
    check(!await page.locator('#log').evaluate(el => el.textContent.includes('cannot edit')), 'log should be read-only');
    await page.setViewportSize({width: 1200, height: 1000});
    check((await metrics()).height > initial.height + 190, 'log did not grow with viewport');
    await page.setViewportSize({width: 375, height: 667});
    const mobile = await metrics();
    check(mobile.height > 0 && mobile.fits && mobile.pageFits, 'mobile log should scroll internally');
    await page.evaluate(() => { document.getElementById('home').hidden = true; });
    check(!await page.locator('#home').isVisible(), 'flex layout must respect hidden');
    console.log('Log checks passed: viewport sizing, auto-scroll, manual scrolling, read-only, mobile, hidden.');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
