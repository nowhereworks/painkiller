# CKAD-001 — mock 01

21 questions, 100 points, 120 minutes, adapted from `docs/ckad-mock-01.md`.
Source order, weights, and core skills are retained. Names, paths, wording,
and selected equivalent numeric targets differ. The mock weights are not the
official CKAD domain percentages.

## Standalone lab and authoring contract

The user approved the simplest practical topology **without changing the core
questions to accommodate it**. One controller and two workers form one cluster;
all candidate files and commands belong on the initial controller. Source
context-switching boilerplate is removed, not simulated with misleading aliases.

Linked clones of generic base **9199**, at least 4096 MiB per VM, use the shared
full-install (`legacy`) provisioning path with Calico. This deliberately overrides
legacy's historical 9010 default. No image build, image modification, extra VM,
reserved LAN address, or cross-exam asset dependency is required. DHCP, NICs,
machine IDs, and host keys remain under the existing clone identity contract.
The shared playbook pins Kubernetes and Calico; Helm 3.19.0 is installed with
archive checksum verification if absent. Preparation requires package and image
downloads. Changes are confined to the new exam and its documentation/tests.
Infrastructure preparation waits for cloud-init on every fresh clone before
package provisioning, including completion with only recoverable warnings.
Exam assets, including the intentionally broken chart and trusted capture/
mutation helpers, are verified against `assets/SHA256SUMS` at preflight.

`prepare.sh` reconciles its own phases with guest-local markers under
`/var/lib/painkiller/ckad001`. Completed preparations preserve candidate work.
The Kubernetes setup owns all namespaces, fixtures, isolated faults, and the
Anime CRD. The external web server is a host systemd service, not a Kubernetes
Pod. The supporting MySQL Deployment is ephemeral, without external storage.
The `ledger-client` credential-checking fixture is intentionally lightweight:
this question assesses Secret environment import, not SQL administration.

`check-ready.sh` is read-only. It verifies three Ready nodes, Calico, supplied
files, healthy supporting services, and intentional faults. Separate canary
Pods prove working general connectivity, blocked ingress, and blocked egress.
The question's target namespace is still absent when Start becomes available.

## Light variations and necessary corrections

| Q | Variant |
| --- | --- |
| 1 | `container-lab/trio-runner`; alpha/web/omega; LEAD/TAIL; declared port 8081 |
| 2 | `cedar-static`; original provisioner, binding mode, expansion flag |
| 3 | `batch-lab/process-report`; node:22-bookworm; every 20 minutes |
| 4 | `pod-workshop/ubuntu-rest`; ubuntu-agent; ubuntu:24.04; sleep 7200 |
| 5 | azure-web/jade-web; 10 Pods, 60/40 split; release-entry, 8081 → 8080 |
| 6 | `dev-workshop/bulletin-web`; bad nginx configuration path; 2 replicas |
| 7 | `/opt/lantern-chart`; lantern-web; frontend-workshop; 4 replicas; appVersion 1.21.0 |
| 8 | parcel-web/parcel-entry; 6445 → 80 |
| 9 | sealed-workshop/isolate-everything; deny all ingress and egress |
| 10 | host-web-entry/host-web-backend; controller host server, 8082 → 9998 |
| 11 | message-web/message-entry; tier=messages; 6381 → 8080 |
| 12 | resolver-alias; dns.google; UDP 53 |
| 13 | qos-workshop/elastic-web; elastic-container; /root/qos-report.txt |
| 14 | evening-series, Cowboy Bebop, 26 episodes; discover Anime CRD |
| 15 | workshop-settings; /root/workshop-env/{app,cache}.env |
| 16 | database-workshop/ledger-client; ledger-settings; sql02/ledger/practice456 |
| 17 | quota-workshop/quota-object-cap; one ResourceQuota |
| 18 | vault-reader; vault-settings; credential-volume; /etc/credentials |
| 19 | warmup-web; original delayed-start image and /ready:8080; delay 20 |
| 20 | /root/workshop-busybox.yaml; toolbox-rest; busybox:1.36 |
| 21 | redis-watch; redis:7.4; exec ls; delay 8 |

- ExternalName uses a real DNS hostname instead of the source's IP literal:
  CNAME records do not implement IP forwarding.
- External backend repair uses EndpointSlice, the current API for manually
  managed selectorless-Service backends, instead of deprecated Endpoints.
- Declared container ports do not change listeners. Services explicitly state
  their real target ports. The first nginx task assesses declared metadata only.
- Shared Service traffic ratios are approximate, based on 6/4 ready endpoints;
  they are not true active/passive blue-green switching or exact request ratios.
- Chart appVersion remains metadata, separate from the existing image tag.
- StorageClass expansion is a field-definition task, not actual disk resizing.

## Grading and evidence limits

Each question is all-or-nothing at its source weight. Captures are read-only and
bounded; API failures propagate rather than becoming incorrect answers.
Missing resources and malformed candidate YAML fail the answer. Service tasks
include HTTP probes from the observer Pod. QoS evidence is compared against all
live namespace Pods, independent of row order. Secret import and the read-only
Secret mount include live container checks. Both quota spellings are accepted.

The deny-all question checks its complete policy and absence of competing
policies, backed by the separate initial enforcement checks; grading does not
create probe Pods in the candidate namespace. Command history cannot be proven
from final cluster state, including imperative creation. Student-admin access
is not a tamper-resistant grading boundary.

## Validation

Follow `docs/testing.md`; inspect the actual host and choose a completely free
three-VM range before running:

```sh
LAB_VM_START=3100 scripts/test-exam.sh CKAD-001
```

Expected baseline is zero; reference solutions must score 100/100. One isolated
loss/restore mutation per question must remove exactly that question's points,
then restore the full score. This is representative mutation coverage, not an
exhaustive adversarial audit. Successful validation destroys only its marked
VMs; failures preserve the lab for inspection. It does not cover browser UI.

Validated on 2026-11-14 on prox-k12 with `scripts/test-exam.sh CKAD-001`:
baseline 0, reference answers 100/100, all 21 isolated loss/restore mutations,
and automatic lab destruction passed. The complete run took 711.24 seconds.
All 168 local Go tests (with race detection), vet, shellcheck, asset checksums,
JavaScript syntax, and playbook syntax checks also passed. No browser coverage
is claimed.
