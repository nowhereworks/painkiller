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
    await page.addStyleTag({path: 'web/vendor/xterm.css'});
    await page.evaluate(() => {
      window.logOutput = Array.from({length: 200}, (_, i) => `Provisioning line ${i}`).join('\n');
      window.fetch = async url => ({
        json: async () => ({phase: 'preparing'}),
        text: async () => window.logOutput
      });
    });
    await page.addScriptTag({path: 'web/vendor/xterm.js'});
    await page.addScriptTag({path: 'web/vendor/addon-fit.js'});
    await page.addScriptTag({path: 'web/app.js'});
    const waitForOutput = text => page.waitForFunction(text => {
      const buffer = logTerm.buffer.active;
      return Array.from({length: buffer.length}, (_, i) => buffer.getLine(i).translateToString(true)).join('\n').includes(text);
    }, text);
    await waitForOutput('Provisioning line 199');
    const metrics = () => page.evaluate(() => {
      const log = document.getElementById('log');
      return {
        height: log.clientHeight,
        bottom: logTerm.buffer.active.viewportY === logTerm.buffer.active.baseY,
        fits: log.getBoundingClientRect().bottom <= innerHeight,
        pageFits: document.documentElement.scrollHeight <= innerHeight,
        editable: !logTerm.options.disableStdin
      };
    });
    const initial = await metrics();
    check(initial.bottom && initial.fits && initial.pageFits && !initial.editable, 'initial log layout or scrolling failed');
    await page.evaluate(async () => {
      logTerm.scrollToTop();
      await refresh();
    });
    check(await page.evaluate(() => logTerm.buffer.active.viewportY === 0), 'unchanged output should preserve manual scrolling');
    await page.evaluate(async () => { window.logOutput += '\nNew output'; await refresh(); });
    await waitForOutput('New output');
    check((await metrics()).bottom, 'new output did not auto-scroll');
    await page.locator('#log textarea').focus();
    await page.keyboard.type('cannot edit');
    check(await page.evaluate(() => !logTerm.buffer.active.getLine(logTerm.buffer.active.length - 1).translateToString(true).includes('cannot edit')), 'log should be read-only');
    await page.setViewportSize({width: 1200, height: 1000});
    check((await metrics()).height > initial.height + 190, 'log did not grow with viewport');
    await page.setViewportSize({width: 375, height: 667});
    const mobile = await metrics();
    check(mobile.height > 0 && mobile.fits && mobile.pageFits, 'mobile log should scroll internally');
    await page.evaluate(async () => { window.logOutput = '\x1b[31mRed\x1b[0m normal'; await refresh(); });
    await waitForOutput('Red normal');
    check(await page.evaluate(() => {
      const line = logTerm.buffer.active.getLine(0);
      return line.getCell(0).getFgColor() === 1 && line.getCell(4).isFgDefault();
    }), 'ANSI red and reset were not rendered');
    check(await page.evaluate(() => logTerm.buffer.active.baseY === 0), 'replaced snapshot should clear old scrollback');
    await page.evaluate(async () => { window.logOutput += '\n\x1b[3'; await refresh(); });
    await page.evaluate(async () => { window.logOutput += '2mGreen\x1b[0m'; await refresh(); });
    await waitForOutput('Green');
    check(await page.evaluate(() => logTerm.buffer.active.getLine(1).getCell(0).getFgColor() === 2), 'split ANSI sequence was not preserved');
    await page.evaluate(async () => { window.logOutput = ''; await refresh(); });
    check(!await page.locator('#log').isVisible(), 'empty log should be hidden');
    await page.evaluate(() => { document.getElementById('home').hidden = true; });
    check(!await page.locator('#home').isVisible(), 'flex layout must respect hidden');
    console.log('Log checks passed: sizing, auto-scroll, read-only, mobile, ANSI colors, split escapes, snapshot replacement, hidden.');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
