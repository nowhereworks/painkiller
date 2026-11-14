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
      window.fetch = async url => ({ok:true,json: async () => url === '/api/tests'
        ? [{id:'CKA-001',title:'CKA practice exam',description:'Practice test',minutes:120}]
        : {phase: 'idle'}});
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
    await page.evaluate(() => {
      document.getElementById('instructions').textContent = 'Question content\n'.repeat(200);
    });
    for (const viewport of [
      {width: 1200, height: 800}, {width: 1365, height: 767},
      {width: 800, height: 601}, {width: 600, height: 800},
      {width: 375, height: 667},
    ]) {
      await page.setViewportSize(viewport);
      await page.evaluate(() => new Promise(resolve =>
        requestAnimationFrame(() => requestAnimationFrame(resolve))));
      const layout = await page.evaluate(() => {
        const panel = document.getElementById('terminal-panel').getBoundingClientRect();
        const screen = document.querySelector('#terminal .xterm-screen').getBoundingClientRect();
        const questions = document.getElementById('question-panel');
        return {
          pageOverflow: document.documentElement.scrollHeight - innerHeight,
          bottomGap: panel.bottom - screen.bottom,
          screenBottom: screen.bottom,
          questionsScroll: questions.scrollHeight > questions.clientHeight,
        };
      });
      check(layout.pageOverflow <= 0, `page overflows at ${JSON.stringify(viewport)}`);
      check(layout.bottomGap >= 8, `terminal bottom spacing missing at ${JSON.stringify(viewport)}`);
      check(layout.screenBottom <= viewport.height - 8, 'last terminal row extends past viewport');
      check(layout.questionsScroll, 'long questions should scroll inside their panel');
    }
    await page.evaluate(() => {
      document.getElementById('exam').hidden = true;
      const results = document.getElementById('results');
      results.hidden = false;
      results.innerHTML = '<p>Result</p>'.repeat(100);
    });
    check(await page.evaluate(() => document.documentElement.scrollHeight > innerHeight),
      'long results should still allow page scrolling');
    check(errors.length === 0, 'browser errors: ' + errors.join(', '));
    console.log('Layout checks passed: default split, drag, keyboard, minimums, terminal fit, mobile, overflow, bottom spacing, results scrolling.');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
