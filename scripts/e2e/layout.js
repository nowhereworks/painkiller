// Browser-only layout regression; no server or lab required.
const fs = require('fs');
const {launch, check} = require('./lib');

(async () => {
  const browser = await launch();
  try {
    const page = await browser.newPage({viewport: {width: 1200, height: 800}});
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.setContent(fs.readFileSync('web/index.html', 'utf8')
      .replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, '')
      .replace(/<link\b[^>]*>/g, ''));
    await page.addStyleTag({path: 'web/style.css'});
    await page.addStyleTag({path: 'web/vendor/xterm.css'});
    await page.evaluate(() => {
      window.fetch = async () => ({json: async () => ({phase: 'idle'})});
      window.WebSocket = class {
        constructor() { this.readyState = 1; queueMicrotask(() => this.onopen?.()); }
        send(data) { window.lastTerminalSize = JSON.parse(data); }
        close() {}
      };
    });
    await page.addScriptTag({path: 'web/vendor/xterm.js'});
    await page.addScriptTag({path: 'web/vendor/addon-fit.js'});
    await page.addScriptTag({path: 'web/app.js'});
    await page.evaluate(() => {
      document.getElementById('home').hidden = true;
      document.getElementById('exam').hidden = false;
      document.getElementById('reconnect').click();
    });
    const widths = () => page.evaluate(() => ['question-panel', 'terminal-panel']
      .map(id => document.getElementById(id).getBoundingClientRect().width));
    let [question, terminal] = await widths();
    check(Math.abs(question / (question + terminal) - .4) < .001, 'default split is not 40/60');
    await page.waitForFunction(() => window.lastTerminalSize?.cols > 0);
    const initialCols = await page.evaluate(() => window.lastTerminalSize.cols);
    const divider = page.locator('#panel-divider');
    const box = await divider.boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + 100);
    await page.mouse.down();
    await page.mouse.move(700, 150);
    await page.mouse.up();
    [question] = await widths();
    check(Math.abs(question - 696) < 1, 'drag did not resize questions');
    await page.waitForFunction(cols => window.lastTerminalSize.cols < cols, initialCols);
    await divider.focus();
    await page.keyboard.press('ArrowLeft');
    check(Math.abs((await widths())[0] - (question - 10)) < 1, 'keyboard resize failed');
    await page.keyboard.press('Home');
    check(Math.abs((await widths())[0] - 270) < 1, 'question minimum failed');
    await page.keyboard.press('End');
    check(Math.abs((await widths())[1] - 150) < 1, 'terminal minimum failed');
    await page.setViewportSize({width: 800, height: 800});
    check((await widths())[1] >= 150, 'viewport resize collapsed terminal');
    await page.setViewportSize({width: 600, height: 800});
    check(!await divider.isVisible(), 'mobile divider should be hidden');
    check(Math.abs((await widths())[0] - 600) < 1, 'mobile questions should fill width');
    check(errors.length === 0, 'browser errors: ' + errors.join(', '));
    console.log('Layout checks passed: default split, drag, keyboard, minimums, terminal fit, mobile.');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
