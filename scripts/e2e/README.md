# End-to-end harness

Reproducible browser/scoring checks against a **real** lab. Everything runs from
the repository root; the local server must also be started there because the
scripts and exam data are resolved relative to the working directory.

## Prerequisites

- A prepared Proxmox lab (see `README.md`) and the server running on
  `http://127.0.0.1:8080` (`PAINKILLER_LISTEN`).
- The server defaults to prebaked template `9100`; build and validate a
  compatible template first if it does not exist. Use an idle, marked lab range;
  never reset an existing lab merely to run this harness.
- `ssh` with the lab key setup from `scripts/configure-exam-access.sh` for the
  management host (`data/lab_known_hosts` is used by default).
- `node`, `jq`, and Google Chrome at `/usr/bin/google-chrome`.
- `playwright-core`:

  ```sh
  npm --prefix scripts/e2e install
  ```

## Environment

| Variable | Default | Purpose |
| --- | --- | --- |
| `E2E_BASE_URL` | `http://127.0.0.1:8080` | Server under test |
| `E2E_CHROME` | `/usr/bin/google-chrome` | Chrome/Chromium binary |
| `E2E_TASKS` | `exam/<E2E_EXAM>/tasks.json` | Task definitions used for scoring |
| `E2E_EXAM` | `CKA-001` | Selected test; task/reference/mutation assets come from this exam folder |
| `E2E_RESUME` | unset | Set to `1` only to resume your own failed harness attempt that is still running |
| `E2E_SKIP_RESET` | unset | Keep the lab after `live.js` |
| `LAB_KNOWN_HOSTS` | `data/lab_known_hosts` | Guest host keys used by `ssh` |

`live.js` refuses an existing attempt unless explicitly resumed, and always
refuses a different selected exam. It never resets a failed run automatically.
Each exam owns `e2e.json`: an expected `baseline` and a `mutations` array of
`{task, remove, restore}` shell commands. Each mutation must remove all credit
for exactly its task. The baseline must match on two captures within 60 seconds,
allowing freshly faulted fixtures to converge without accepting persistent
unintended credit. Captures are reused for all checks of a task in each score.

## Scripts

### `live.js` — full scoring flow (needs an idle attempt)

Prepares the lab if it is idle, starts the exam, checks that marks and the
selected question survive a reload, then:

1. scores the untouched lab: **0 / 100**;
2. applies `exam/<E2E_EXAM>/reference-solutions.sh` on the controller and re-scores:
   **100 / 100**;
3. deletes one answer (`kubectl -n exam-workloads delete configmap settings`)
   and expects exactly that task's points to be lost (currently 100 − 2 = 98);
4. restores the answer and submits through the browser, expecting
   **100 / 100** in the results header;
5. verifies a duplicate submit returns 409 and `/api/terminal` returns 409;
6. destroys the lab unless `E2E_SKIP_RESET=1`.

```sh
node scripts/e2e/live.js
```

### `terminal.js` — interactive shell and reconnect (needs a running exam)

Types a command into the xterm.js terminal, checks the echo, then reconnects and
verifies the timer is still present.

```sh
E2E_SKIP_RESET=1 node scripts/e2e/live.js   # leaves the exam running
node scripts/e2e/terminal.js
```

### `expiry.js` — automatic grading at the deadline (needs a short exam)

Start the server with `PAINKILLER_MINUTES=1`, prepare the lab, start the exam,
then run:

```sh
node scripts/e2e/expiry.js
```

It waits for `completed`, prints the result and checks that the terminal is
closed.

## Failure and recovery coverage

Failure paths are automated where a browser is not needed (see
`cmd/painkiller/main_test.go`):

| Path | Coverage |
| --- | --- |
| Provisioning fails mid-run, retry reuses VMs | `TestRetryPreparationReusesLabAndPreservesLog`, `TestFreshPreparationCreatesAndProvisions` |
| Image builder ownership, ranges, clone safety, shared lock | `TestImageBuilderRejectsOccupiedDestination`, `TestImageBuilderResumeRefusesUnownedOrPublishedVM`, `TestImageBuilderRejectsOutsideTemplateRange`, `TestImageBuilderFullClonesWithoutChangingBaseOrIdentity`, `TestImageBuilderSharesLabOperationLock` |
| Prebaked compatibility, linked clones, no install/reset on provision | `TestPrebakedCreateRejectsUnmarkedTemplate`, `TestPrebakedRejectsUnsupportedConfiguration`, `TestCreateUsesLinkedClonesAndWaitsForReady`, `TestPrebakedProvisionSkipsInstallAndReset` |
| Standalone local playbooks, invalid inputs fail before remote operations | `TestLocalPlaybooksFailBeforeRemoteOperations`, isolated-checkout builder/provision/create tests |
| Failed cleanup (destroy), retry reset | `TestRetryResetRerunsDestroy`, `TestRetryIsRestrictedToLabOperationFailures` |
| Duplicate start / submit → 409 | `TestConcurrentStartOnlyStartsOnce`, `TestDuplicateSubmitGradesOnlyOnce` |
| Restart during preparing/resetting/running/grading | `TestResumeAfterRestart`, `TestStartPersistsDeadline` |
| Expiry during a session | `TestExpiryClosesTerminalsAndGrades` |
| Grading cannot persist | `TestGradingPersistenceFailureIsReported` |
| Terminal disconnect and reconnect | `terminal.js` |
| Answer capture retries only failures | `TestCaptureRetriesTransientFailure`, `TestSnapshotGradingIsImmutableAndPrivate` |

Concurrent lab operations are rejected through `lab-operation.lock`
(`TestLabOperationLockRejectsConcurrentOperation`).
