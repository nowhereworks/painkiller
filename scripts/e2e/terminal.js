// Interactive terminal and reconnect against a running exam.
//   node scripts/e2e/terminal.js
const {base, launch, waitPhase, check} = require('./lib');

(async () => {
  const browser = await launch();
  const page = await browser.newPage();
  await page.goto(base);
  await waitPhase(page, ['running']);
  await page.waitForFunction(() => document.querySelector('.xterm-screen')?.textContent.includes('Connected.'),
    null, {timeout: 60000});

  await page.locator('.xterm-helper-textarea').focus();
  await page.keyboard.type("printf '%s%s\\n' 'BROWSER_' 'TERMINAL_OK'");
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.querySelector('.xterm-screen')?.textContent.includes('BROWSER_TERMINAL_OK'),
    null, {timeout: 60000});
  console.log('INTERACTIVE TERMINAL: passed');

  await page.getByRole('button', {name: 'Reconnect'}).click();
  await page.waitForFunction(() => document.querySelector('.xterm-screen')?.textContent.includes('Connected.'),
    null, {timeout: 60000});
  check(await page.locator('#timer').innerText() !== '', 'the exam timer was lost on reconnect');
  console.log('TERMINAL RECONNECT: passed');

  await browser.close();
})().catch((e) => {
  console.error('TERMINAL TEST FAILED:', e);
  process.exit(1);
});
