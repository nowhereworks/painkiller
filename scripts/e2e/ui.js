// Isolated UI checks: no Go server, SSH, or Proxmox required.
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs/promises');
const path = require('node:path');
const {chromium} = require('playwright-core');

async function main() {
  const root = path.resolve(__dirname, '../../web');
  const server = http.createServer(async (req, res) => {
    const file = path.resolve(root, '.' + (req.url === '/' ? '/index.html' : req.url));
    if (!file.startsWith(root + path.sep)) { res.writeHead(403).end(); return; }
    try {
      const content = await fs.readFile(file);
      res.setHeader('Content-Type', file.endsWith('.js') ? 'text/javascript' : file.endsWith('.css') ? 'text/css' : 'text/html');
      res.end(content);
    } catch { res.writeHead(404).end(); }
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  let browser;
  try {
    browser = await chromium.launch({headless:true, executablePath:process.env.E2E_CHROME || '/usr/bin/google-chrome', args:['--no-sandbox']});
    const page = await browser.newPage({viewport:{width:1280,height:900}});
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const now = new Date('2026-11-13T12:00:00Z');
    await page.clock.install({time:now});
    await page.clock.pauseAt(now);
    await page.addInitScript(() => {
      window.WebSocket = class { close() {} send() {} };
    });
    const instructions = 'Use context: `kubectl config use-context k8s-c2-AC`\n\nFind **important information**:\n\n- Server private key location\n- Server certificate expiration\n\nWrite to `/opt/course/p1/etcd-info.txt`.\n\n```bash\nkubectl get pods --all-namespaces\n```\n\n```yaml\napiVersion: v1\nkind: Pod\n```\n\n```json\n{"ready": true}\n```\n\n```unknown\n<plain>\n```\n\n<script>window.unsafe=true</script><img src=x onerror="window.unsafe=true"><a href="javascript:alert(1)">unsafe link</a>';
    const tasks = [
      {ID:'one',Title:'Inspect the cluster',Points:3,Instructions:instructions.replace('kubectl get pods --all-namespaces', '# Inspect all namespaces\nkubectl get pods --all-namespaces'),Steps:['Run `kubectl get pods`. <img src=x onerror="window.unsafe=true"><script>window.unsafe=true</script>','Verify that the expected Pods exist.'],Solution:'kubectl get pods\n# <script>window.unsafe=true</script>',Explanation:'Inspect the running Pods.'},
      {ID:'two',Title:'Create a namespace',Points:2,Instructions:'Create namespace `exam-sandbox`.'},
      {ID:'three',Title:'Review configuration',Points:2,Instructions:'Check the configuration.'}
    ];
    const state = {phase:'running',deadline:new Date(now.getTime()+90*60000).toISOString(),duration_seconds:7200,flags:{},done:{}};
    await page.route('**/api/**', async route => {
      const name = new URL(route.request().url()).pathname.split('/').pop();
      if (name === 'status') return route.fulfill({json:state});
      if (name === 'tasks') return route.fulfill({json:tasks});
      const body = route.request().postDataJSON();
      if (name === 'select') state.selected = body.id;
      if (name === 'mark') { state.flags[body.id] = body.flag; state.done[body.id] = body.done; }
      await route.fulfill({status:204});
    });
    const base = `http://127.0.0.1:${server.address().port}`;
    await page.goto(base);
    await page.waitForFunction(() => document.querySelector('#timer').textContent === '90 minutes');
    assert.equal(await page.locator('#previous').isDisabled(), true);
    assert.equal(await page.locator('#next').isDisabled(), false);
    assert.equal(await page.locator('#time-fill').evaluate(el => el.style.width), '75%');
    assert.equal(await page.locator('#time-progress').getAttribute('data-level'), 'green');
    assert.equal(await page.locator('#instructions p code').first().innerText(), 'kubectl config use-context k8s-c2-AC');
    assert.equal(await page.locator('#instructions li').count(), 2);
    for (const language of ['bash','yaml','json']) {
      assert.ok(await page.locator(`#instructions code.language-${language} .token`).count() > 0, `${language} highlighting`);
    }
    assert.equal(await page.locator('#instructions code.language-unknown').innerText(), '<plain>\n');
    assert.equal(await page.locator('#instructions script, #instructions img, #instructions a').count(), 0);
    assert.equal(await page.evaluate(() => window.unsafe), undefined);
    assert.notEqual(await page.locator('#instructions p code').first().evaluate(el => getComputedStyle(el).backgroundColor), 'rgba(0, 0, 0, 0)');

    assert.equal(await page.locator('#solution').isHidden(), true);
    assert.equal(await page.locator('#reveal').innerText(), 'Reveal Instructions');
    await page.locator('#reveal').click();
    assert.equal(await page.locator('#solution').isVisible(), true);
    assert.equal(await page.locator('#reveal').getAttribute('aria-expanded'), 'true');
    assert.equal(await page.locator('#solution ol > li').count(), 2);
    assert.equal(await page.locator('#solution pre code').innerText(), tasks[0].Solution);
    assert.ok(await page.locator('#solution code.language-bash .token').count() > 0);
    assert.equal(await page.locator('#solution script, #solution img').count(), 0);
    assert.equal(await page.evaluate(() => window.unsafe), undefined);
    await page.evaluate(() => refresh());
    assert.equal(await page.locator('#solution').isVisible(), true, 'polling must not hide the walkthrough');
    await page.locator('#reveal').click();
    assert.equal(await page.locator('#solution').isHidden(), true);
    assert.equal(await page.locator('#reveal').getAttribute('aria-expanded'), 'false');
    await page.locator('#reveal').click();

    // Keep the option and rendered content DOM intact across polling.
    await page.evaluate(() => {
      window.originalOption = document.querySelector('#question-select option');
      window.originalCode = document.querySelector('#instructions code');
    });
    await page.evaluate(() => refresh());
    assert.equal(await page.evaluate(() => window.originalOption === document.querySelector('#question-select option') && window.originalCode === document.querySelector('#instructions code')), true);

    await page.locator('#next').click();
    await page.waitForFunction(() => document.querySelector('#title').textContent.includes('Create a namespace'));
    assert.equal(await page.locator('#solution').isHidden(), true, 'switching questions hides the walkthrough');
    assert.equal(await page.locator('#solution').innerHTML(), '');
    assert.equal(await page.locator('#reveal').getAttribute('aria-expanded'), 'false');
    await page.locator('#flag').check();
    await page.waitForFunction(() => document.querySelector('#question-select option[value="1"]').textContent.includes('⚑'));
    await page.locator('#done').check();
    await page.waitForFunction(() => document.querySelector('#question-select option[value="1"]').textContent.includes('✓'));
    await page.reload();
    await page.waitForFunction(() => document.querySelector('#question-select').value === '1');
    assert.equal(await page.locator('#flag').isChecked(), true);
    assert.equal(await page.locator('#done').isChecked(), true);
    await page.locator('#question-select').selectOption('2');
    await page.waitForFunction(() => document.querySelector('#next').disabled);
    await page.locator('#previous').click();
    await page.waitForFunction(() => document.querySelector('#question-select').value === '1');

    for (const [seconds, duration, level, width, text] of [
      [1800,7200,'orange',25,'30 minutes'], [720,7200,'red',10,'12 minutes'],
      [1801,7200,'green',1801/72,'31 minutes'], [721,7200,'orange',721/72,'13 minutes'],
      [2700,5400,'green',50,'45 minutes'], [59,5400,'red',59/54,'1 minute'],
      [0,5400,'red',0,'0 minutes'], [-60,5400,'red',0,'0 minutes'],
      [8000,7200,'green',100,'134 minutes']
    ]) {
      state.deadline = new Date(now.getTime()+seconds*1000).toISOString();
      state.duration_seconds = duration;
      await page.evaluate(() => refresh());
      assert.equal(await page.locator('#timer').innerText(), text);
      assert.equal(await page.locator('#time-progress').getAttribute('data-level'), level);
      const actual = await page.locator('#time-fill').evaluate(el => parseFloat(el.style.width));
      assert.ok(Math.abs(actual-width)<0.001, `bar width: ${actual} vs ${width}`);
    }
    state.deadline = new Date(now.getTime()+90*60000).toISOString();
    state.duration_seconds = 7200;
    state.selected = 'one';
    // Capture only the safe, realistic portion of the formatting fixture.
    tasks[0].Instructions = instructions.split('\n\n```bash')[0];
    await page.evaluate(() => refresh());
    if (process.env.E2E_SCREENSHOT_DIR) {
      await fs.mkdir(process.env.E2E_SCREENSHOT_DIR, {recursive:true});
      await page.screenshot({path:path.join(process.env.E2E_SCREENSHOT_DIR,'question-desktop.png')});
    }
    await page.setViewportSize({width:600,height:900});
    // xterm's ResizeObserver fits its canvas after the viewport resize.
    await page.waitForFunction(() => document.documentElement.scrollWidth <= innerWidth);
    if (process.env.E2E_SCREENSHOT_DIR) await page.screenshot({path:path.join(process.env.E2E_SCREENSHOT_DIR,'question-mobile.png')});
    // Every packaged question has its own steps and exact solution commands.
    const packaged = JSON.parse(await fs.readFile(path.resolve(__dirname, '../../exam/tasks.json'), 'utf8'));
    tasks.splice(0, tasks.length, ...packaged.map(t => ({ID:t.id,Title:t.title,Points:t.points,Instructions:t.instructions,Steps:t.steps,Solution:t.solution,Explanation:t.explanation})));
    for (const task of tasks) {
      state.selected = task.ID;
      await page.evaluate(() => refresh());
      assert.equal(await page.locator('#solution').isHidden(), true);
      await page.locator('#reveal').click();
      assert.equal(await page.locator('#solution ol > li').count(), task.Steps.length);
      assert.equal(await page.locator('#solution pre code').innerText(), task.Solution);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    }
    assert.deepEqual(errors, []);
    console.log('UI checks passed: navigation, polling, reload, timer boundaries, Markdown, syntax highlighting, sanitization, responsive layout, all 29 solution walkthroughs.');
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
