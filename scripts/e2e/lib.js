// Shared helpers for the end-to-end harness. Run every script from the
// repository root; the server must be started there too.
const {spawnSync} = require('child_process');
const fs = require('fs');

const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:8080';
const chrome = process.env.E2E_CHROME || '/usr/bin/google-chrome';
const knownHosts = process.env.LAB_KNOWN_HOSTS || 'data/lab_known_hosts';
const examID = process.env.E2E_EXAM || 'CKA-001';
if (!/^[A-Z][A-Z0-9]*-[0-9]{3}$/.test(examID)) throw Error('invalid E2E_EXAM');
const sshBase = ['-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10', '-o', 'StrictHostKeyChecking=yes',
  '-o', 'GlobalKnownHostsFile=/dev/null', '-o', 'UserKnownHostsFile=' + knownHosts];
const tasks = JSON.parse(fs.readFileSync(process.env.E2E_TASKS || `exam/${examID}/tasks.json`, 'utf8'));

function ssh(host, command, input) {
  const r = spawnSync('ssh', [...sshBase, 'milkman@' + host, command],
    {input, encoding: 'utf8', timeout: 600000, maxBuffer: 64 * 1024 * 1024});
  if (r.status !== 0) throw Error(`ssh command failed (${command}): ${r.stderr} ${r.stdout}`);
  return r.stdout;
}

function jq(expression, input) {
  const r = spawnSync('jq', ['-e', `try ((${expression}) // false) catch false`], {input, encoding: 'utf8'});
  if (r.status === 0) return true;
  if (r.status === 1) return false;
  throw Error(`invalid check ${expression}: ${r.stderr}`);
}

// score reads the live cluster the same way the server does, so it reports what
// a submission would score right now.
function score(host) {
  let total = 0;
  for (const t of tasks) {
    const captured = (ssh(host, t.resource) || '').trim() || 'null';
    for (const c of t.checks) {
      if (jq(c.expression, captured)) total += c.points;
    }
  }
  return total;
}

function points(id) {
  const t = tasks.find((x) => x.id === id);
  if (!t) throw Error(`unknown task ${id}`);
  return t.points;
}

async function launch() {
  const {chromium} = require('playwright-core');
  return chromium.launch({headless: true, executablePath: chrome, args: ['--no-sandbox']});
}

async function status() {
  const r = await fetch(base + '/api/status', {cache: 'no-store'});
  return r.json();
}

async function post(action, body) {
  return fetch(base + '/api/' + action, {
    method: 'POST',
    headers: {'Content-Type': 'application/json', Origin: base},
    body: JSON.stringify(body || (action === 'prepare' ? {test_id: examID} : {})),
  });
}

async function waitPhase(page, phases, timeout) {
  await page.waitForFunction((want) => want.includes(document.querySelector('#phase').textContent),
    phases, {timeout: timeout || 3600000});
  return page.locator('#phase').innerText();
}

function check(condition, message) {
  if (!condition) throw Error(message);
}

module.exports = {base, chrome, examID, tasks, ssh, jq, score, points, launch, status, post, waitPhase, check};
