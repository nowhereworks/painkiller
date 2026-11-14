// Full scoring flow on a real lab. Assumes a server is running at E2E_BASE_URL
// and that the attempt is idle (run scripts/e2e/live.js again after a reset).
//
//   E2E_SKIP_RESET=1 node scripts/e2e/live.js   # keep the lab for other scripts
const fs = require('fs');
const {base, examID, ssh, score, points, launch, status, post, waitPhase, check} = require('./lib');
const fixture = JSON.parse(fs.readFileSync(`exam/${examID}/e2e.json`, 'utf8'));

(async () => {
  const initial = await status();
  check(initial.phase === 'idle' || process.env.E2E_RESUME === '1',
    `existing ${initial.phase} attempt retained; E2E_RESUME=1 is required to resume your own harness lab`);
  if (initial.phase !== 'idle') check(initial.test_id === examID, 'refusing a different active exam');
  const browser = await launch();
  const page = await browser.newPage();
  page.on('pageerror', (e) => console.error('PAGE ERROR', e));
  page.on('dialog', (d) => d.accept());
  await page.goto(base);

  let phase = await waitPhase(page, process.env.E2E_RESUME === '1' ? ['idle', 'ready', 'running', 'error'] : ['idle', 'ready', 'error']);
  if (phase === 'idle') {
    const started = Date.now();
    await page.locator('#test-select').selectOption(examID);
    await page.getByRole('button', {name: 'Prepare lab'}).click();
    phase = await waitPhase(page, ['ready', 'error']);
    console.log('PREPARE SECONDS', (Date.now() - started) / 1000);
  }
  if (phase === 'error') throw Error('lab preparation failed: ' + await page.locator('#message').innerText());
  if (phase === 'idle') throw Error('lab did not become ready');
  const {host, test_id: selected} = await status();
  check(selected === examID, `active test ${selected} differs from requested ${examID}; refusing to modify it`);
  console.log('LAB READY', host);

  if (phase === 'ready') await page.getByRole('button', {name: 'Start exam'}).click();
  await waitPhase(page, ['running']);
  console.log('EXAM STARTED');

  // Marks and the current question survive a reload.
  await page.locator('#question-select').selectOption('4');
  await page.locator('#flag').check();
  await page.reload();
  await waitPhase(page, ['running']);
  await page.waitForFunction(() => document.querySelector('#title').textContent.length > 0);
  check(await page.locator('#flag').isChecked(), 'flag lost after reload');
  console.log('MARKS PERSIST: passed');

  // Faulted deployments can briefly retain old ready replicas after apply.
  // Require two matching captures, bounded to 60 seconds; persistent unintended
  // baseline credit must fail rather than being silently accepted.
  let baseline, stable = 0;
  const deadline = Date.now() + 60000;
  do {
    baseline = score(host);
    stable = baseline === fixture.baseline ? stable + 1 : 0;
    if (stable >= 2) break;
    await new Promise((resolve) => setTimeout(resolve, 1000));
  } while (Date.now() < deadline);
  console.log('BASELINE SCORE', baseline);
  check(stable >= 2, `baseline should stabilize at ${fixture.baseline}, got ${baseline}`);

  // Terminal works and a reconnect keeps the attempt (timer) alive.
  await page.waitForFunction(() => document.querySelector('.xterm-screen')?.textContent.includes('Connected.'),
    null, {timeout: 60000});
  await page.locator('.xterm-helper-textarea').focus();
  await page.keyboard.type("printf '%s%s\\n' 'BROWSER_' 'TERMINAL_OK'");
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.querySelector('.xterm-screen')?.textContent.includes('BROWSER_TERMINAL_OK'),
    null, {timeout: 60000});
  await page.getByRole('button', {name: 'Reconnect'}).click();
  await page.waitForFunction(() => document.querySelector('.xterm-screen')?.textContent.includes('Connected.'),
    null, {timeout: 60000});
  check(await page.locator('#timer').innerText() !== '', 'the exam timer was lost on reconnect');
  console.log('TERMINAL AND RECONNECT: passed');

  console.log('APPLYING REFERENCE SOLUTIONS');
  ssh(host, 'bash -s', fs.readFileSync(`exam/${examID}/reference-solutions.sh`, 'utf8'));
  const reference = score(host);
  console.log('REFERENCE SCORE', reference);
  check(reference === 100, `reference answers should score 100, got ${reference}`);

  for (const mutation of fixture.mutations) {
    ssh(host, mutation.remove);
    const removed = score(host);
    console.log('ONE-ANSWER-REMOVED SCORE', mutation.task, removed);
    check(removed === 100 - points(mutation.task), `expected ${100 - points(mutation.task)}, got ${removed}`);
    ssh(host, mutation.restore);
    check(score(host) === 100, `restoring ${mutation.task} did not recover 100`);
  }

  await page.reload();
  await waitPhase(page, ['running']);
  await page.getByRole('button', {name: 'Submit exam'}).click();
  await page.waitForSelector('#results h1', {timeout: 600000});
  const result = await page.locator('#results h1').innerText();
  console.log('BROWSER RESULT', result);
  check(result.includes('100 / 100'), `browser grading did not score 100: ${result}`);

  // Submission is final: no double grading, no terminal.
  const again = await post('submit');
  check(again.status === 409, `duplicate submit returned ${again.status}`);
  const terminal = await page.request.get(base + '/api/terminal');
  check(terminal.status() === 409, `terminal accepted after submit: ${terminal.status()}`);
  console.log('SUBMISSION IS FINAL: passed');

  if (!process.env.E2E_SKIP_RESET) {
    const started = Date.now();
    await page.getByRole('button', {name: 'Destroy lab / reset'}).last().click();
    const final = await waitPhase(page, ['idle', 'error'], 600000);
    if (final !== 'idle') throw Error('reset failed: ' + await page.locator('#message').innerText());
    console.log('RESET COMPLETE');
    console.log('RESET SECONDS', (Date.now() - started) / 1000);
  }
  await browser.close();
})().catch((e) => {
  console.error('LIVE TEST FAILED:', e);
  process.exit(1);
});
