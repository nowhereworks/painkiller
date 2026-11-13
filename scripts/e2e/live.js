// Full scoring flow on a real lab. Assumes a server is running at E2E_BASE_URL
// and that the attempt is idle (run scripts/e2e/live.js again after a reset).
//
//   E2E_SKIP_RESET=1 node scripts/e2e/live.js   # keep the lab for other scripts
const fs = require('fs');
const {base, ssh, score, points, launch, status, post, waitPhase, check} = require('./lib');

(async () => {
  const browser = await launch();
  const page = await browser.newPage();
  page.on('pageerror', (e) => console.error('PAGE ERROR', e));
  page.on('dialog', (d) => d.accept());
  await page.goto(base);

  let phase = await waitPhase(page, ['idle', 'ready', 'error']);
  if (phase === 'idle') {
    await page.getByRole('button', {name: 'Prepare lab'}).click();
    phase = await waitPhase(page, ['ready', 'error']);
  }
  if (phase === 'error') throw Error('lab preparation failed: ' + await page.locator('#message').innerText());
  if (phase === 'idle') throw Error('lab did not become ready');
  const {host} = await status();
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

  const baseline = score(host);
  console.log('BASELINE SCORE', baseline);
  check(baseline === 0, `baseline should be 0, got ${baseline}`);

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
  ssh(host, 'bash -s', fs.readFileSync('exam/CKA-001/reference-solutions.sh', 'utf8'));
  const reference = score(host);
  console.log('REFERENCE SCORE', reference);
  check(reference === 100, `reference answers should score 100, got ${reference}`);

  // One removed answer must cost exactly that task's points.
  ssh(host, 'kubectl -n exam-workloads delete configmap settings --ignore-not-found');
  const removed = score(host);
  console.log('ONE-ANSWER-REMOVED SCORE', removed);
  check(removed === 100 - points('configmap'), `expected ${100 - points('configmap')}, got ${removed}`);
  ssh(host, 'kubectl -n exam-workloads create configmap settings --from-literal=mode=production');

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
    await page.getByRole('button', {name: 'Destroy lab / reset'}).last().click();
    const final = await waitPhase(page, ['idle', 'error'], 600000);
    if (final !== 'idle') throw Error('reset failed: ' + await page.locator('#message').innerText());
    console.log('RESET COMPLETE');
  }
  await browser.close();
})().catch((e) => {
  console.error('LIVE TEST FAILED:', e);
  process.exit(1);
});
