# Scenario Authoring

Scenarios define the training content, cluster topology, tasks, and grading checks for tests. Scenarios are stored in a Git repository and imported into Painkiller Shell.

## Scenario Repository Structure

```
scenarios/
  cka/
    simulator-001/
      scenario.yaml
      tasks/
        task-01.md
        task-02.md
      provision/
        playbook.yaml
        group_vars/
          all.yaml
        files/
          config.yaml
      checks/
        checks.yaml
        scripts/
          check-task-01.sh
```

### Directory Layout

- **`scenario.yaml`** - Main scenario definition (required)
- **`tasks/`** - Markdown files with task prompts for students
- **`provision/`** - Ansible playbooks for scenario-specific setup
- **`checks/`** - Grading checks (YAML definitions and scripts)

## Scenario Definition

The `scenario.yaml` file defines the scenario metadata, topology, and tasks.

### Example Scenario

```yaml
id: cka-simulator-001
title: CKA Simulator 1
duration_minutes: 120
access_window_hours: 36
attempts_allowed: 2

topology:
  clusters:
    - id: cluster-a
      display_name: cluster-a
      kube_context: cluster-a-admin
      nodes:
        - name: cp-1
          role: control-plane
          template: kubeadm-control-plane
        - name: worker-1
          role: worker
          template: kubeadm-worker
    - id: cluster-b
      display_name: cluster-b
      kube_context: cluster-b-admin
      nodes:
        - name: cp-1
          role: control-plane
          template: kubeadm-control-plane
        - name: worker-1
          role: worker
          template: kubeadm-worker
        - name: worker-2
          role: worker
          template: kubeadm-worker

tasks:
  - id: task-01
    cluster_id: cluster-a
    kube_context: cluster-a-admin
    points: 8
    prompt_file: tasks/task-01.md
  - id: task-02
    cluster_id: cluster-b
    kube_context: cluster-b-admin
    points: 12
    prompt_file: tasks/task-02.md
```

### Schema Reference

**Top-Level Fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique scenario identifier |
| `title` | string | yes | Display name for the scenario |
| `duration_minutes` | integer | yes | Time limit for each attempt |
| `access_window_hours` | integer | yes | How long purchase remains valid |
| `attempts_allowed` | integer | yes | Number of attempts per purchase |
| `topology` | object | yes | Cluster and node definitions |
| `tasks` | array | yes | List of tasks for students |

**Topology**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clusters` | array | yes | List of clusters in the environment |

**Cluster**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique cluster identifier |
| `display_name` | string | yes | Name shown to students |
| `kube_context` | string | yes | kubectl context name |
| `nodes` | array | yes | List of nodes in the cluster |

**Node**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Node hostname |
| `role` | string | yes | `control-plane` or `worker` |
| `template` | string | yes | Proxmox VM template name |

**Task**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique task identifier |
| `cluster_id` | string | yes | Cluster this task uses |
| `kube_context` | string | yes | kubectl context for the task |
| `points` | integer | yes | Points available for this task |
| `prompt_file` | string | yes | Path to task prompt markdown file |

## Task Prompts

Task prompts are Markdown files in the `tasks/` directory. They describe what students need to accomplish.

### Example Task Prompt

```markdown
# Task 1: Create a NetworkPolicy

**Cluster:** cluster-a  
**Points:** 8

## Objective

Create a NetworkPolicy named `restrict-nginx` in the `default` namespace that:

1. Denies all ingress traffic to pods with label `app=nginx`
2. Allows ingress traffic only from pods with label `role=frontend`
3. Allows ingress traffic only on port 80

## Verification

After completing this task, the following should be true:

- Pods with label `app=nginx` can only receive traffic from pods with label `role=frontend`
- All other ingress traffic to `app=nginx` pods is blocked

## Hints

- Use `kubectl get pods -l app=nginx` to identify target pods
- NetworkPolicy resources are namespaced
```

## Grading Checks

Checks define how tasks are validated and scored. Checks are defined in `checks/checks.yaml`.

### Check Types

**kubectl** - Runs a kubectl command and validates output or exit code

**script** - Runs a custom script and validates exit code

### Example Checks

```yaml
checks:
  - id: task-01-check
    task_id: task-01
    cluster_id: cluster-a
    type: kubectl
    command: |
      kubectl get networkpolicy restrict-nginx -n default -o jsonpath='{.spec.podSelector.matchLabels.app}'
    expected_output: nginx
    points: 8

  - id: task-02-check
    task_id: task-02
    cluster_id: cluster-b
    type: script
    script: scripts/check-task-02.sh
    points: 12
```

### Check Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique check identifier |
| `task_id` | string | yes | Task this check validates |
| `cluster_id` | string | yes | Cluster to run check against |
| `type` | string | yes | `kubectl` or `script` |
| `command` | string | for kubectl | kubectl command to run |
| `expected_output` | string | no | Expected stdout (exact match) |
| `script` | string | for script | Path to script file |
| `points` | integer | yes | Points awarded if check passes |

### Check Scripts

For complex validations, use script checks. Scripts run on the student workstation and should:

- Exit with code 0 if the check passes
- Exit with non-zero code if the check fails
- Output diagnostic information to stdout/stderr (for debugging)

Example `scripts/check-task-02.sh`:

```bash
#!/bin/bash
set -e

# Check if deployment exists
kubectl get deployment nginx -n production

# Check replica count
REPLICAS=$(kubectl get deployment nginx -n production -o jsonpath='{.spec.replicas}')
if [ "$REPLICAS" != "3" ]; then
  echo "Expected 3 replicas, got $REPLICAS"
  exit 1
fi

# Check if pods are running
RUNNING=$(kubectl get pods -n production -l app=nginx --field-selector=status.phase=Running -o json | jq '.items | length')
if [ "$RUNNING" != "3" ]; then
  echo "Expected 3 running pods, got $RUNNING"
  exit 1
fi

exit 0
```

## Provisioning Playbooks

Scenario-specific setup is handled by Ansible playbooks in the `provision/` directory.

### Main Playbook

`provision/playbook.yaml` runs after cluster provisioning and sets up the scenario environment.

Example:

```yaml
---
- name: CKA Simulator 001 Setup
  hosts: all
  become: yes
  tasks:
    - name: Create namespaces
      kubernetes.core.k8s:
        state: present
        definition:
          apiVersion: v1
          kind: Namespace
          metadata:
            name: "{{ item }}"
      loop:
        - production
        - staging
      when: inventory_hostname in groups['control_plane']

    - name: Deploy sample workloads
      kubernetes.core.k8s:
        state: present
        src: files/workloads.yaml
      when: inventory_hostname == groups['control_plane'][0]

    - name: Introduce misconfiguration for task-01
      kubernetes.core.k8s:
        state: absent
        kind: NetworkPolicy
        name: restrict-nginx
        namespace: default
      when: inventory_hostname == groups['control_plane'][0]
```

### Group Variables

`provision/group_vars/all.yaml` contains variables available to all hosts:

```yaml
---
kubernetes_version: "1.29"
pod_network_cidr: "10.244.0.0/16"
service_cidr: "10.96.0.0/12"
```

## Scenario Validation

The scenario importer validates scenarios before importing:

### Validation Rules

1. **Cluster IDs** must be unique within the scenario
2. **Node names** must be unique within each cluster
3. **Task cluster_id** must reference an existing cluster
4. **Check task_id** must reference an existing task
5. **Points** must be positive integers
6. **At least one cluster** must be defined
7. **At least one task** must be defined
8. **Prompt files** must exist at specified paths
9. **Check scripts** must exist at specified paths (for script checks)

### Testing Validation

Test a scenario locally:

```bash
go run ./cmd/validate-scenario /path/to/scenario.yaml
```

This validates the scenario without importing it.

## Importing Scenarios

Scenarios are imported from the Git repository specified in `SCENARIO_REPO_PATH`.

### Manual Import

```bash
go run ./cmd/import-scenarios
```

This scans the scenario repository, validates all scenarios, and creates immutable versions in the database.

### Automatic Import

Scenarios can be imported automatically via webhook when the Git repository is updated. This requires:

1. Git repository webhook configuration (GitHub, GitLab, etc.)
2. API endpoint to trigger import
3. Authentication for webhook requests

## Versioning

Each import creates an immutable scenario version linked to a Git commit SHA. This ensures:

- Attempts always use the scenario version they started with
- Scenario changes don't affect in-progress attempts
- Rollback is possible by re-importing an older commit

### Version History

View scenario versions:

```sql
SELECT id, scenario_id, git_commit_sha, created_at
FROM scenario_versions
ORDER BY created_at DESC;
```

## Best Practices

### Scenario Design

1. **Clear objectives** - Each task should have a clear, measurable goal
2. **Realistic scenarios** - Mimic real-world Kubernetes administration tasks
3. **Progressive difficulty** - Start with simpler tasks, increase complexity
4. **Adequate time** - Allow enough time for students to complete tasks
5. **Multiple attempts** - Provide at least 2 attempts per purchase

### Topology Design

1. **Minimal clusters** - Use the smallest topology that meets learning objectives
2. **Consistent naming** - Use clear, descriptive cluster and node names
3. **Template reuse** - Reuse VM templates across scenarios when possible
4. **Resource limits** - Consider Proxmox resource constraints

### Check Design

1. **Validate final state** - Check the end result, not the process
2. **Multiple checks** - Use multiple checks for complex tasks
3. **Clear feedback** - Provide diagnostic output for debugging
4. **Idempotent checks** - Checks should be safe to run multiple times
5. **Partial credit** - Use multiple checks to award partial credit

### Task Prompts

1. **Be specific** - Clearly state what needs to be accomplished
2. **Provide context** - Explain why the task matters
3. **Include examples** - Show expected commands or outputs
4. **Add hints** - Provide helpful hints without giving away the solution
5. **Define verification** - Tell students how to verify their work

## Example Scenarios

### CKA Practice Exam

Multi-cluster scenario with tasks covering:
- Cluster administration
- Workload scheduling
- Services and networking
- Storage
- Security
- Troubleshooting

### CKAD Practice Exam

Single-cluster scenario with tasks covering:
- Application design and deployment
- Container building
- Observability and maintenance
- Application environment and configuration
- Services and networking

### Custom Training

Tailored scenarios for specific training objectives:
- Kubernetes upgrades
- Disaster recovery
- Performance tuning
- Security hardening
