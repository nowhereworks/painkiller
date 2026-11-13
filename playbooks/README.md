# Project-owned playbooks

Provisioning and image building use only this directory, never another Git
checkout. Install the required collection with:

```sh
ansible-galaxy collection install -r playbooks/requirements.yml
```

`kubeadm-cluster/kubeadm-cluster.yml` was originally copied from
https://github.com/nowhereworks/lab-ansible-playbooks at commit
`8ec676e93017ce3e110c27485e25d1f32d541056`. That tree contains no license file
or playbook licensing notice. No other playbooks, local roles, or local files
are referenced by this playbook. Downloads and Ansible collections still
require their normal external sources; this is not an offline installation.

`SHA256SUMS` guards the playbook layout used by the scripts' phase extraction.
When changing the playbook, update its checksum. Templates are selected by VM ID;
when image contents need to change, build a new template at a free ID in
9100–9200 and select it with `LAB_TEMPLATE_ID`. Never overwrite a template
with dependent linked clones. Both builder and provisioner validate the local
checksum before modifying VMs. Missing or incompatible images fail without a
legacy fallback.

During provisioning, every lab node receives a managed `/etc/hosts` block and
a dedicated SSH identity. Each node's public key is authorized on every node;
private keys remain on their originating nodes. Hostname-scoped SSH configuration
uses the lab account (`milkman` by default) and verifies the nodes' host keys.
