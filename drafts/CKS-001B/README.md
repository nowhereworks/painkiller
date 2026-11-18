# CKS-001B — work in progress

This is an incomplete authoring workspace, not an exam and not loaded by the
server. Move its completed, exam-owned files to `exam/CKS-001B/` only when the
required definition, preparation, readiness and grading contracts are ready.
Do not use this directory to provision a lab.

`test.json` and `questions.json` capture the proposed topology, 16 adapted
questions and source weights. The latter is authoring input, **not** the
server's `tasks.json` schema: solutions, walkthroughs, captures, checks,
fixtures and explanations still need to be implemented.

`assets/network_policy.py` provides a bounded model for the specific
NetworkPolicy probe cases in questions 2, 4 and 13. It is supplementary to real
Calico connectivity tests, not proof that a policy is enforced by the CNI.
Run `python3 -B -m unittest discover -s drafts/CKS-001B/tests -v`.
