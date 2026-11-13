# Test organization

Each test lives in `exam/<ID>/`, using an uppercase family and three-digit
number (for example `CKA-001`). The folder name must match `test.json`'s `id`.
Keep all test-specific files, assets, and documentation inside that folder.
Shared server code, lifecycle scripts, and provisioning playbooks stay shared.

Required files:

| File | Purpose |
| --- | --- |
| `test.json` | `id`, `title`, `description`, and `minutes` (1–1440) |
| `tasks.json` | Questions, points, walkthroughs, solutions, capture commands, and checks |
| `setup.yaml` | Kubernetes resources applied after provisioning |
| `reference-solutions.sh` | Reference answers for validation on a disposable lab |

Use `CKA-001` as the starting example. The server discovers and validates
folders at startup; adding a test requires no server or UI code changes.
Restart after adding or editing definitions. Invalid definitions fail startup.
Test files contain trusted code executed on the lab; do not accept untrusted
uploads. Reference scripts should refuse to run on an unmarked cluster.

The first screen lists available tests. Preparing binds the selected test to
the current attempt, including setup, questions, and grading. Only one lab and
attempt are active at a time; reset before choosing another test. Restarts
restore the selected test, and attempts predating test selection belong to
`CKA-001`. Do not remove or edit a test while its attempt is active.

Duration comes from `test.json`; `PAINKILLER_MINUTES`, when set, overrides it.
Provisioning remains shared: tests currently use the configured lab topology
(one controller and two workers by default).
