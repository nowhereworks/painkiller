// Expiry path: the attempt is graded automatically at the deadline and the
// terminal is closed. Start the server with PAINKILLER_MINUTES=1 first:
//   PAINKILLER_MINUTES=1 go run ./cmd/painkiller
// then prepare the lab, start the exam, and run this script.
const {base, launch, status, waitPhase, check} = require('./lib');

(async () => {
  const s = await status();
  if (s.phase !== 'running') throw Error(`this script needs a running exam, got ${s.phase}`);
  const remaining = (new Date(s.deadline) - Date.now()) / 1000;
  if (remaining > 150) {
    throw Error(`exam has ${Math.round(remaining)}s left; use PAINKILLER_MINUTES=1 for this test`);
  }
  console.log('WAITING FOR EXPIRY', Math.max(0, Math.round(remaining)), 'seconds');
  const browser = await launch();
  const page = await browser.newPage();
  await page.goto(base);
  await waitPhase(page, ['completed'], remaining * 1000 + 600000);
  const result = await page.locator('#results h1').innerText();
  check(result.length > 0, 'expired attempt produced no results');
  console.log('EXPIRED ATTEMPT GRADED:', result);
  const terminal = await page.request.get(base + '/api/terminal');
  check(terminal.status() === 409, `terminal still open after expiry: ${terminal.status()}`);
  console.log('TERMINAL CLOSED AT EXPIRY: passed');
  await browser.close();
})().catch((e) => {
  console.error('EXPIRY TEST FAILED:', e);
  process.exit(1);
});
