# CKS-001B — work in progress

This is an incomplete authoring workspace, not an exam and not loaded by the
server. Move its completed, exam-owned files to `exam/CKS-001B/` only when the
required definition, preparation, readiness and grading contracts are ready.
Do not use this directory to provision a lab.

`test.json` and `questions.json` capture the proposed topology, 16 adapted
questions and source weights. The latter is authoring input, **not** the
server's `tasks.json` schema: solutions, walkthroughs, captures, checks,
fixtures and explanations still need to be implemented.

`assets/fixture_objects.py` generates the static Kubernetes starting objects.
`assets/install.sh` stages pinned host dependencies, and
`assets/host_fixtures.py` creates the Dockerfile, binary, initial invoice
manifest, Falco configuration and TLS key pairs once. The partial
`core_ready.py` and `host_ready.py` checks cover only those subsets. The draft
`prepare.sh` deliberately refuses to run until the full exam definition,
supporting services, readiness wrapper and asset checksums exist. Ingress,
Istio installation, TLS Secret creation, image archives, network test endpoints,
node faults and their readiness checks are not yet implemented or validated.

Dependency investigation (2026-11-18): upstream
[ingress-nginx is retired](https://github.com/kubernetes/ingress-nginx#ingress-nginx-retirement)
and its published v1.15.1 compatibility table ends at Kubernetes 1.35, whereas
the shared bootstrap currently defaults to 1.36.1. Do not assume that either a
retired ingress-nginx controller or Traefik enforces Q12's nginx annotations;
the final lab needs a verified compatible implementation and real HTTP redirect
and HTTPS probes. For Istio, upstream publishes an
[amd64 1.32.0 release](https://github.com/istio/istio/releases/tag/1.32.0);
its compatibility, download pin, installation and real sidecar behavior have
not yet been validated in this lab.

`assets/network_policy.py` provides a bounded model for the specific
NetworkPolicy probe cases in questions 2, 4 and 13. It is supplementary to real
Calico connectivity tests, not proof that a policy is enforced by the CNI.
Run `python3 -B -m unittest discover -s drafts/CKS-001B/tests -v`.
