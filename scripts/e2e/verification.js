// Learning verification rendering; mocked HTTP, no server or lab required.
const fs = require('fs');
const {launch, check} = require('./lib');

(async () => {
  const browser = await launch();
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.setContent(fs.readFileSync('web/index.html', 'utf8')
      .replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, '')
      .replace(/<link\b[^>]*>/g, ''));
    await page.addStyleTag({path: 'web/style.css'});
    await page.evaluate(() => {
      window.fetch = async url => ({ok: true, json: async () => {
        if (url === '/api/tests') return [];
        if (url === '/api/verify') {
          if (window.delayVerification) return new Promise(resolve => { window.finishVerification = resolve; });
          return window.verificationResult;
        }
        return {phase: 'idle'};
      }});
      window.setInterval = () => 0;
    });
    await page.addScriptTag({path: 'web/vendor/xterm.js'});
    await page.addScriptTag({path: 'web/vendor/addon-fit.js'});
    await page.addScriptTag({path: 'web/app.js'});
    await page.evaluate(() => {
      state = {phase: 'running', mode: 'learning'};
      tasks = [{ID: 'one'}, {ID: 'two'}];
      index = 0;
      document.getElementById('home').hidden = true;
      document.getElementById('exam').hidden = false;
      document.getElementById('verify').hidden = false;
      window.verificationResult = {checks: [
        {description: 'First requirement', points: 1, earned: 1},
        {description: 'Three correctly configured containers are running', points: 8, earned: 0,
          details: ['Container alpha must use busybox:1.28', '<img src=x onerror=alert(1)>']},
        {description: 'Third requirement', points: 1, earned: 0, error: 'SSH unavailable'},
      ]};
    });
    await page.locator('#verify').click();
    await page.waitForSelector('.verification-error');
    check(await page.locator('.verification-check').count() === 3, 'each check needs its own block');
    check(await page.locator('.verification-passed strong').textContent() === 'Passed', 'missing pass label');
    check(await page.locator('.verification-failed strong').textContent() === 'Failed', 'missing fail label');
    check((await page.locator('.verification-failed').textContent()).includes('Requirement: Three'), 'requirement is not distinguished from result');
    check(await page.locator('#verification img').count() === 0, 'diagnostics must be plain text');
    const colors = await page.locator('.verification-check').evaluateAll(blocks =>
      blocks.map(block => getComputedStyle(block).backgroundColor));
    check(new Set(colors).size === 3, 'pass, fail and error need different backgrounds');
    await page.evaluate(() => { window.verificationResult = {error: 'Capture unavailable'}; });
    await page.locator('#verify').click();
    await page.waitForFunction(() => document.getElementById('verification').textContent.includes('Capture unavailable'));
    check(await page.locator('.verification-failed').count() === 0, 'capture errors are not incorrect answers');
    await page.evaluate(() => { window.delayVerification = true; });
    await page.locator('#verify').click();
    await page.waitForFunction(() => !!window.finishVerification);
    await page.evaluate(() => {
      index = 1;
      document.getElementById('verification').replaceChildren();
      window.finishVerification(window.verificationResult);
    });
    await page.waitForFunction(() => !document.getElementById('verify').disabled);
    check(await page.locator('.verification-check').count() === 0, 'stale response appeared on another question');
    check(errors.length === 0, errors.join('\n'));
    console.log('Learning verification rendering passed');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
