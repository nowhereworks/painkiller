# Fix Proxmox Provisioning

## Context
The docker-compose file has Proxmox credentials but provisioning cannot work due to multiple code bugs, missing config, and missing infrastructure. This plan fixes the code-level issues and adds a no-op provisioner mode so we can validate the VM lifecycle (clone/start/stop/delete) against a real Proxmox server without needing Ansible playbooks.

## Changes

### 1. Docker Compose (`resources/docker-compose-dev-ephemeral.yaml`)
Add missing environment variables to `painkiller-server`:
- `PROVIDER=proxmox`
- `PROXMOX_STORAGE_POOL=local-lvm`
- `PROXMOX_NETWORK_BRIDGE=vmbr0`
- `PROXMOX_SKIP_TLS_VERIFY=true`
- `PROXMOX_PROFILES_FILE=/etc/painkiller/proxmox-profiles.yaml`
- `PROVISIONER_MODE=none`

### 2. App Config (`internal/config/config.go`)
- Add `ProxmoxSkipTLSVerify bool` field, loaded from `PROXMOX_SKIP_TLS_VERIFY`
- Add `ProvisionerMode string` field, loaded from `PROVISIONER_MODE` (default `"ansible"`)
- Add `getBoolEnv` helper

### 3. Proxmox Config (`internal/provider/proxmox/config.go`)
- Add `SkipTLSVerify bool` field to `Config`

### 4. Proxmox Client (`internal/provider/proxmox/client.go`)
- **TLS skip-verify**: When `SkipTLSVerify` is true, configure `http.Client` with `InsecureSkipVerify`
- **Task polling**: Add `waitForTask(ctx, upid)` that polls `/nodes/{node}/tasks/{upid}/status` until the task completes, returns the task result
- **Dynamic VMID**: Change `CloneVM` signature to return `(int, error)`. Omit `newid` from the clone request body so Proxmox auto-allocates. Extract VMID from the UPID (6th colon-separated field). Wait for the task to complete before returning.

### 5. Proxmox Provider (`internal/provider/proxmox/provider.go`)
- Remove `nextVMID` atomic counter
- Use VMID returned from `CloneVM` instead
- **Cloud-init fix**: Remove the unused `GenerateCloudInit` call and `_ = cloudInit`. Instead, pass Proxmox built-in cloud-init parameters directly in `ConfigureVM`: `citype=configdrive2`, `cipublickey=<ssh_key>`, `ciname=<hostname>`. This requires templates to have a cloud-init drive (ide0: cloudinit) pre-attached.
- Apply cloud-init params to both workstation and cluster node VMs

### 6. Cloud-init Cleanup (`internal/provider/proxmox/cloudinit.go`)
- Remove `GenerateCloudInit` function and `CloudInitConfig` struct (no longer needed since we use Proxmox built-in cloud-init params)
- Keep the file with just the package declaration or remove if unused elsewhere

### 7. No-Op Provisioner (`internal/provisioner/noop/noop.go`)
- Create a new `noop` package with a provisioner that logs "skipping provisioning (noop mode)" and returns `ProvisionResult{Ready: true}`
- Implements `provisioner.Provisioner` interface

### 8. Server Wiring (`cmd/server/main.go`)
- When `cfg.ProvisionerMode == "none"`, use `noop.New(logger)` instead of `ansible.New(logger)`
- Pass `SkipTLSVerify` to proxmox config
- Log which provisioner mode is active

### 9. Provision Job (`internal/orchestrator/provision_job.go`)
- Set `PlaybookPath` on the `EnvironmentProvisionSpec` using the scenario's provision directory convention: `<scenario_dir>/provision/playbook.yaml`
- This is a no-op when using the noop provisioner but ensures correctness when Ansible mode is enabled later

## Verification
1. `go build ./...` - compiles cleanly
2. `go test ./...` - all existing tests pass
3. `go vet ./...` - no issues
4. Manual: run `make run-dev` with real Proxmox credentials, trigger an attempt via the API, verify VMs are cloned/started/destroyed on Proxmox
