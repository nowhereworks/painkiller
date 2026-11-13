// Browser-only clipboard regression in Chrome; no server or lab required.
const fs = require('fs');
const {launch, check} = require('./lib');

(async () => {
  const browser = await launch();
  try {
    const context = await browser.newContext({permissions: ['clipboard-read', 'clipboard-write']});
    const page = await context.newPage();
    await page.route('http://localhost/**', route => route.fulfill({
      contentType: 'text/html',
      body: fs.readFileSync('web/index.html', 'utf8')
        .replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, '')
        .replace(/<link\b[^>]*>/g, '')
    }));
    await page.goto('http://localhost/');
    await page.addStyleTag({path: 'web/vendor/xterm.css'});
    await page.evaluate(() => {
      window.sent = [];
      window.fetch = async () => ({json: async () => ({phase: 'idle'})});
      window.WebSocket = class {
        constructor() { this.readyState = 1; queueMicrotask(() => this.onopen?.()); }
        send(data) { if (typeof data !== 'string') window.sent.push(new TextDecoder().decode(data)); }
        close() {}
      };
      document.getElementById('terminal').style.cssText = 'width:800px;height:400px';
    });
    await page.addScriptTag({path: 'web/vendor/xterm.js'});
    await page.addScriptTag({path: 'web/vendor/addon-fit.js'});
    await page.addScriptTag({path: 'web/app.js'});
    await page.evaluate(() => {
      document.getElementById('exam').hidden = false;
      connect();
      document.addEventListener('keydown', e => {
        if (e.ctrlKey && e.shiftKey && ['c', 'v'].includes(e.key.toLowerCase()))window.lastShortcutEvent = e;
      }, true);
    });
    await page.waitForFunction(() => document.querySelector('.xterm-screen')?.textContent.includes('Connected.'));
    await page.evaluate(() => { term.select(0, 1, 10); term.focus(); });
    const selection = await page.evaluate(() => term.getSelection());
    check(selection === 'Connected.', 'test selection is incorrect');
    await page.keyboard.press('Control+Shift+C');
    await page.waitForFunction(async text => await navigator.clipboard.readText() === text, selection);
    check(await page.evaluate(() => window.lastShortcutEvent.defaultPrevented), 'copy did not cancel Chrome default');
    check(await page.evaluate(() => window.sent.length === 0), 'copy sent input to shell');
    await page.evaluate(() => { term.clearSelection(); return navigator.clipboard.writeText('paste-test\nsecond-line'); });
    await page.keyboard.press('Control+Shift+V');
    await page.waitForFunction(() => window.sent.join('') === 'paste-test\rsecond-line');
    check(await page.evaluate(() => window.lastShortcutEvent.defaultPrevented), 'paste did not cancel Chrome default');
    await page.evaluate(() => { window.sent = []; });
    await page.keyboard.press('Control+C');
    check(await page.evaluate(() => window.sent.join('') === '\x03'), 'Ctrl+C no longer interrupts');
    await page.evaluate(() => navigator.clipboard.writeText('keep-me'));
    await page.keyboard.press('Control+Shift+C');
    check(await page.evaluate(() => navigator.clipboard.readText()) === 'keep-me', 'empty selection overwrote clipboard');
    const failures = [];
    page.on('dialog', async dialog => { failures.push(dialog.message()); await dialog.dismiss(); });
    await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', {
      configurable: true, value: {readText: async () => { throw new Error('Permission denied'); }}
    }));
    await page.keyboard.press('Control+Shift+V');
    await page.waitForFunction(() => document.hasFocus());
    check(failures.some(text => text.includes('Permission denied')), 'clipboard denial was not reported');
    await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', {value: undefined}));
    await page.keyboard.press('Control+Shift+V');
    check(failures.some(text => text.includes('HTTPS or localhost')), 'insecure-context guidance missing');
    await page.locator('#panel-divider').focus();
    await page.keyboard.press('Control+Shift+V');
    check(await page.evaluate(() => !window.lastShortcutEvent.defaultPrevented), 'shortcut intercepted outside terminal');
    console.log('Clipboard checks passed: Chrome shortcuts, copy, paste, Ctrl+C, empty selection, failures, focus scope.');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
