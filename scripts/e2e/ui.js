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
    tasks[0].Explanation = 'Inspect the running Pods with `kubectl get pods`. <script>window.unsafe=true</script><img src=x onerror="window.unsafe=true">';
    const state = {phase:'running',deadline:new Date(now.getTime()+90*60000).toISOString(),duration_seconds:7200,flags:{},done:{}};
    const actions = [];
    await page.route('**/api/**', async route => {
      const name = new URL(route.request().url()).pathname.split('/').pop();
      if (name === 'status') return route.fulfill({json:state});
      if (name === 'tests') return route.fulfill({json:[
        {id:'CKA-001',title:'CKA practice exam',description:'Original test',minutes:120},
        {id:'CKA-002',title:'Second test fixture',description:'Another test',minutes:60}
      ]});
      if (name === 'tasks') return route.fulfill({json:tasks});
      if (name === 'log') return route.fulfill({body:''});
      const body = route.request().postDataJSON();
      actions.push(name);
      if (name === 'prepare') { state.test_id = body.test_id; state.phase = 'preparing'; }
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
    assert.equal(await page.locator('#solution > div p code').innerText(), 'kubectl get pods');
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

    // A new attempt in the same page must not inherit question or terminal state.
    await page.locator('#reveal').click();
    await page.evaluate(() => term.write('OLD LAB OUTPUT\r\n'));
    await page.clock.runFor(100);
    now.setTime(now.getTime()+100);
    assert.equal(await page.evaluate(() => {
      const buffer = term.buffer.active;
      return Array.from({length:buffer.length}, (_, i) => buffer.getLine(i).translateToString()).join('\n').includes('OLD LAB OUTPUT');
    }), true);
    await page.evaluate(() => { window.oldSocket = ws; });
    state.phase = 'resetting';
    await page.evaluate(() => refresh());
    state.phase = 'idle';
    delete state.selected;
    state.flags = {}; state.done = {};
    await page.evaluate(() => refresh());
    state.phase = 'running';
    await page.evaluate(() => refresh());
    assert.equal(await page.locator('#question-select').inputValue(), '0');
    assert.equal(await page.locator('#solution').isHidden(), true);
    assert.equal(await page.locator('#solution').innerHTML(), '');
    assert.equal(await page.locator('#flag').isChecked(), false);
    assert.equal(await page.locator('#done').isChecked(), false);
    await page.evaluate(() => {
      window.oldSocket.onmessage({data:new TextEncoder().encode('STALE OUTPUT').buffer});
      window.oldSocket.onclose();
    });
    assert.equal(await page.evaluate(() => {
      const buffer = term.buffer.active;
      return Array.from({length:buffer.length}, (_, i) => buffer.getLine(i).translateToString()).join('\n');
    }).then(text => /OLD LAB OUTPUT|STALE OUTPUT|Disconnected/.test(text)), false);

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
    const packaged = JSON.parse(await fs.readFile(path.resolve(__dirname, '../../exam/CKA-001/tasks.json'), 'utf8'));
    tasks.splice(0, tasks.length, ...packaged.map(t => ({ID:t.id,Title:t.title,Points:t.points,Instructions:t.instructions,Steps:t.steps,Solution:t.solution,Explanation:t.explanation})));
    for (const task of tasks) {
      state.selected = task.ID;
      await page.evaluate(() => refresh());
      assert.equal(await page.locator('#solution').isHidden(), true);
      await page.locator('#reveal').click();
      assert.equal(await page.locator('#solution ol > li').count(), task.Steps.length);
      assert.equal(await page.locator('#solution pre code').innerText(), task.Solution);
      if (task.ID === 'etcd-snapshot') {
        assert.deepEqual(await page.locator('#solution > div p code').allTextContents(), ['etcdctl snapshot save', 'etcdctl snapshot restore']);
        assert.equal((await page.locator('#solution > div').innerText()).includes('`'), false);
      }
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    }
    // Results: separate cards, partial scores, errors, safe formatting and reset controls.
    tasks.splice(0, tasks.length,
      {ID:'pass',Title:'Correct answer',Points:3,Explanation:instructions,Solution:'echo "<script>window.unsafe=true</script>"\n  kubectl get pods'},
      {ID:'partial',Title:'Partial answer',Points:2,Solution:'kubectl get nodes'},
      {ID:'fail',Title:'Wrong answer',Points:2},
      {ID:'error',Title:'Grading failure',Points:1},
      {ID:'missing',Title:'Missing evaluation',Points:1}
    );
    state.phase = 'completed';
    state.results = [
      {id:'pass',passed:true,earned:3,checks:[{description:'Pods exist',earned:3,points:3}]},
      {id:'partial',passed:false,earned:1,checks:[{description:'Created',earned:1,points:1},{description:'Configured',earned:0,points:1}]},
      {id:'fail',passed:false,earned:0},
      {id:'error',passed:false,earned:0,error:'command failed:\n  <script>window.unsafe=true</script>',checks:[{description:'Read resource',earned:0,points:1,error:'connection refused\n  retry later'}]}
    ];
    await page.evaluate(() => refresh());
    assert.equal(await page.locator('#results').isVisible(), true);
    assert.equal(await page.locator('#results h1').innerText(), 'Provisional results (grading errors): 4 / 9');
    assert.equal(await page.locator('.result-card').count(), 5);
    assert.equal(await page.locator('.result-card.correct').count(), 1);
    assert.equal(await page.locator('.result-card.wrong').count(), 2);
    assert.equal(await page.locator('.result-card.error').count(), 2);
    assert.match(await page.locator('.result-card').nth(1).innerText(), /Partially correct — 1 \/ 2/);
    assert.equal(await page.locator('.result-checks>.correct').count(), 2);
    assert.equal(await page.locator('.result-checks>.wrong').count(), 1);
    const card = page.locator('.result-card').first();
    assert.equal(await card.locator(':scope > pre code').innerText(), tasks[0].Solution);
    for (const language of ['bash','yaml','json']) assert.ok(await card.locator(`code.language-${language} .token`).count() > 0);
    assert.equal(await page.locator('.result-card.error > pre code').first().innerText(), state.results[3].error);
    assert.equal(await card.locator('pre').first().evaluate(el => getComputedStyle(el).whiteSpace), 'pre');
    assert.notEqual(await card.evaluate(el => getComputedStyle(el).borderLeftColor), await page.locator('.result-card.wrong').first().evaluate(el => getComputedStyle(el).borderLeftColor));
    assert.equal(await page.locator('#results script, #results img, #results a').count(), 0);
    assert.equal(await page.evaluate(() => window.unsafe), undefined);
    assert.equal(await page.locator('.results-reset').count(), 2);
    assert.equal(await page.locator('#results > :nth-child(2)').getAttribute('class'), 'results-reset');
    assert.equal(await page.locator('#results > :last-child').getAttribute('class'), 'results-reset');
    await page.getByRole('button', {name:'Retry grading',exact:true}).click();
    assert.ok(actions.includes('regrade'));
    for (const position of [0,1]) {
      const before = actions.filter(name => name === 'reset').length;
      page.once('dialog', dialog => dialog.dismiss());
      await page.locator('.results-reset').nth(position).click();
      assert.equal(actions.filter(name => name === 'reset').length, before);
      page.once('dialog', dialog => dialog.accept());
      await Promise.all([
        page.waitForResponse(response => response.url().endsWith('/api/status')),
        page.locator('.results-reset').nth(position).click()
      ]);
      assert.equal(actions.filter(name => name === 'reset').length, before+1);
    }
    for (const width of [1280,375]) {
      await page.setViewportSize({width,height:900});
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
      if (process.env.E2E_SCREENSHOT_DIR) await page.screenshot({path:path.join(process.env.E2E_SCREENSHOT_DIR,`results-${width}.png`),fullPage:true});
    }
    state.results = state.results.filter(r => r.id !== 'error');
    await page.evaluate(() => refresh());
    assert.equal(await page.getByRole('button', {name:'Retry grading',exact:true}).count(), 0);
    state.phase = 'idle';
    delete state.test_id;
    await page.evaluate(() => refresh());
    assert.equal(await page.locator('#test-select option').count(), 2);
    assert.equal(await page.locator('#test-select').isEnabled(), true);
    assert.equal(await page.locator('#test-select').inputValue(), 'CKA-001');
    await page.locator('#test-select').selectOption('CKA-002');
    assert.equal(await page.locator('#test-description').innerText(), 'Another test (60 minutes)');
    await page.locator('#prepare').click();
    await page.waitForFunction(() => document.querySelector('#phase').textContent === 'preparing');
    assert.equal(state.test_id, 'CKA-002');
    assert.equal(await page.locator('#test-select').isDisabled(), true);
    state.phase = 'ready';
    await page.reload();
    await page.waitForFunction(() => document.querySelector('#test-select').value === 'CKA-002');
    assert.equal(await page.locator('#test-select').isDisabled(), true);
    state.phase = 'idle';
    delete state.test_id;
    await page.evaluate(() => refresh());
    assert.equal(await page.locator('#test-select').isEnabled(), true);
    await page.locator('#test-select').selectOption('CKA-001');
    assert.deepEqual(errors, []);
    console.log('UI checks passed: test selection and locking, navigation, polling, reload, timer boundaries, Markdown, syntax highlighting, sanitization, responsive layout, all 29 solution walkthroughs, results cards and reset controls.');
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
