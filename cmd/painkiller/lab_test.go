package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabEndpointHosts(t *testing.T) {
	for _, host := range []string{"192.168.254.80", "::1", "cka002-api", "api.lab.example"} {
		if !validEndpointHost(host) {
			t.Errorf("rejected valid endpoint host %q", host)
		}
	}
	for _, host := range []string{"", "-api", "api-", "api..lab", "api;true", "$(id)", "api/path", strings.Repeat("a", 64)} {
		if validEndpointHost(host) {
			t.Errorf("accepted invalid endpoint host %q", host)
		}
	}
}

func cleanLabEnv(t *testing.T) {
	t.Helper()
	for _, key := range labEnvKeys {
		t.Setenv(key, "")
	}
}

func TestLabContractJSON(t *testing.T) {
	cleanLabEnv(t)
	var spec labSpec
	err := json.Unmarshal([]byte(`{"controllers":3,"workers":2,"initial_controllers":2,"provision_mode":"legacy","cni":"calico","endpoint":"192.168.254.80:8443","memory_mb":4096,"prepare":"prepare.sh"}`), &spec)
	if err != nil {
		t.Fatal(err)
	}
	exams := examFixture(t)
	exam := exams[1]
	exam.Lab = &spec
	if err := os.WriteFile(filepath.Join(exam.dir, "prepare.sh"), []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateLabSpec(exam); err != nil {
		t.Fatal(err)
	}
	config, err := resolveLab(exam)
	if err != nil {
		t.Fatal(err)
	}
	if config.Env["LAB_VM_END"] != "3004" || config.Env["LAB_CONTROLLERS"] != "3" ||
		config.Env["LAB_INITIAL_CONTROLLERS"] != "2" || config.Env["LAB_TEMPLATE_ID"] != "9010" ||
		config.Env["LAB_CNI_PLUGIN"] != "calico" || config.TestID != exam.ID {
		t.Fatalf("incorrect resolution: %+v", config)
	}
	for key, value := range map[string]string{
		"LAB_CONTROLLERS": "1", "LAB_PROVISION_MODE": "prebaked", "LAB_CNI_PLUGIN": "flannel",
		"LAB_VM_END": "3002", "LAB_CONTROL_PLANE_ENDPOINT": "192.0.2.1:6443", "LAB_VM_MEMORY": "3072",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := resolveLab(exam); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("incompatible explicit constraint accepted: %v", err)
			}
		})
	}
	t.Setenv("LAB_VM_START", "3996")
	t.Setenv("LAB_VM_END", "4000")
	if _, err := resolveLab(exam); err == nil {
		t.Fatal("unsafe range accepted")
	}
}

func TestPrebakedHALabContract(t *testing.T) {
	cleanLabEnv(t)
	exam := examFixture(t)[1]
	exam.Lab = &labSpec{Controllers: 3, Workers: 2, InitialControllers: 2, Mode: "prebaked",
		CNI: "calico", Endpoint: "192.168.254.80:8443", MemoryMB: 4096, Template: "9200"}
	if err := validateLabSpec(exam); err != nil {
		t.Fatalf("prebaked HA contract rejected: %v", err)
	}
	config, err := resolveLab(exam)
	if err != nil {
		t.Fatal(err)
	}
	if config.Env["LAB_TEMPLATE_ID"] != "9200" || config.Env["LAB_PROVISION_MODE"] != "prebaked" ||
		config.Env["LAB_CONTROLLERS"] != "3" || config.Env["LAB_INITIAL_CONTROLLERS"] != "2" ||
		config.Env["LAB_CNI_PLUGIN"] != "calico" || config.Env["LAB_CONTROL_PLANE_ENDPOINT"] != "192.168.254.80:8443" {
		t.Fatalf("incorrect prebaked HA resolution: %+v", config)
	}

	// A multi-controller prebaked contract must name its HA template.
	exam.Lab.Template = ""
	if err := validateLabSpec(exam); err == nil {
		t.Fatal("prebaked multi-controller contract without template_id accepted")
	}
	// Out-of-range template IDs are rejected.
	exam.Lab.Template = "9300"
	if err := validateLabSpec(exam); err == nil {
		t.Fatal("out-of-range template_id accepted")
	}
	// The single-controller Flannel prebaked contract needs no template_id.
	exam.Lab = &labSpec{Controllers: 1, Workers: 2, InitialControllers: 1, Mode: "prebaked", CNI: "flannel", MemoryMB: 3072}
	if err := validateLabSpec(exam); err != nil {
		t.Fatalf("single-controller prebaked contract rejected: %v", err)
	}
}

func TestLabSnapshotSurvivesEnvironmentChanges(t *testing.T) {
	cleanLabEnv(t)
	t.Setenv("LAB_VM_START", "3100")
	t.Setenv("LAB_VM_END", "3102")
	t.Setenv("LAB_MARKER", "saved-owner")
	t.Setenv("LAB_SSH_USER", "saved-user")
	config, err := resolveLab(examFixture(t)[0])
	if err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	a.state = state{Phase: "ready", TestID: "CKA-001", Lab: config}
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	var saved string
	if err := a.db.QueryRow("SELECT state FROM attempt WHERE id=1").Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(saved), &a.state); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAB_VM_START", "3200")
	t.Setenv("LAB_VM_END", "3204")
	t.Setenv("LAB_MARKER", "another-owner")
	t.Setenv("LAB_SSH_USER", "another-user")
	if a.labValue("LAB_VM_START", "") != "3100" || a.labValue("LAB_VM_END", "") != "3102" ||
		a.labValue("LAB_MARKER", "") != "saved-owner" || a.labValue("LAB_SSH_USER", "") != "saved-user" {
		t.Fatalf("persisted config changed: %+v", a.state.Lab)
	}
	cmd := a.labCommand("env")
	out, err := cmd.Output()
	if err != nil || !strings.Contains(string(out), "LAB_MARKER=saved-owner\n") ||
		strings.Contains(string(out), "LAB_MARKER=another-owner") {
		t.Fatalf("command did not use snapshot: %s %v", out, err)
	}
}

func TestTrustedPreparationPaths(t *testing.T) {
	exam := examFixture(t)[1]
	exam.Lab = &labSpec{Controllers: 3, Workers: 2, InitialControllers: 2, Mode: "legacy", CNI: "calico", MemoryMB: 4096}
	for _, path := range []string{"../prepare.sh", "/tmp/prepare.sh", "assets/../prepare.sh", "prepare.sh;true", "missing.sh"} {
		exam.Lab.Prepare = path
		if err := validateLabSpec(exam); err == nil {
			t.Fatalf("unsafe/missing path accepted: %s", path)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.sh")
	if err := os.WriteFile(outside, []byte("true"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(exam.dir, "prepare.sh")); err != nil {
		t.Fatal(err)
	}
	exam.Lab.Prepare = "prepare.sh"
	if err := validateLabSpec(exam); err == nil {
		t.Fatal("symlink escape accepted")
	}
}

func TestCatalogOmitsTrustedLabInternals(t *testing.T) {
	a := testApp(t)
	a.exams = examFixture(t)
	a.exams[1].Lab = &labSpec{Controllers: 3, Workers: 2, Prepare: "private-prepare.sh", Endpoint: "192.168.254.80:8443"}
	recorder := httptest.NewRecorder()
	a.testList(recorder, httptest.NewRequest("GET", "/api/tests", nil))
	w := recorder.Body.String()
	if strings.Contains(w, "private-prepare") || strings.Contains(w, "192.168.254.80") ||
		!strings.Contains(w, `"controllers":3`) {
		t.Fatalf("unsafe or missing public topology: %s", w)
	}
}

func TestStagedPreparationAndFailures(t *testing.T) {
	for _, fail := range []string{"", "preflight", "infrastructure", "provision", "access", "fixtures", "readiness", "readiness-timeout"} {
		t.Run(fail, func(t *testing.T) {
			cleanLabEnv(t)
			a := testApp(t)
			a.exams = examFixture(t)
			exam := &a.exams[1]
			exam.Lab = &labSpec{Controllers: 3, Workers: 2, InitialControllers: 2, Mode: "legacy",
				CNI: "calico", MemoryMB: 4096, Prepare: "prepare.sh"}
			root := t.TempDir()
			t.Chdir(root)
			t.Setenv("CALLS", filepath.Join(root, "calls"))
			t.Setenv("FAIL_PHASE", fail)
			if err := os.Mkdir("scripts", 0700); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				"scripts/create-lab.sh": `#!/bin/sh
echo "$1 $LAB_VM_START $LAB_VM_END $LAB_INITIAL_CONTROLLERS" >> "$CALLS"
if [ "$1" = "$FAIL_PHASE" ]; then exit 1; fi
if [ "$1" = status ]; then echo '{"controller_ips":"192.0.2.1,192.0.2.2,192.0.2.3"}'; fi
`,
				"scripts/configure-exam-access.sh": "#!/bin/sh\necho access >> \"$CALLS\"\n[ \"$FAIL_PHASE\" != access ]\n",
				filepath.Join(exam.dir, "prepare.sh"): `#!/bin/sh
echo "$1 $LAB_VM_END" >> "$CALLS"
echo "PHASE: $1"
[ "$1" != "$FAIL_PHASE" ]
`,
				filepath.Join(exam.dir, "check-ready.sh"): "#!/bin/sh\necho readiness >> \"$CALLS\"\n[ \"$FAIL_PHASE\" != readiness ]\n",
			}
			for path, body := range files {
				if err := os.WriteFile(path, []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			stubCommand(t, "ssh", "#!/bin/sh\necho unexpected-setup >> \"$CALLS\"\nexit 1\n")
			stubCommand(t, "timeout", "#!/bin/sh\nif [ \"$1\" = 60s ] && [ \"$FAIL_PHASE\" = readiness-timeout ]; then echo readiness-timeout >> \"$CALLS\"; exit 124; fi\nexec /usr/bin/timeout \"$@\"\n")
			config, err := resolveLab(*exam)
			if err != nil {
				t.Fatal(err)
			}
			a.state = state{Phase: "preparing", TestID: exam.ID, Lab: config}
			a.prepareLab(true)
			calls, err := os.ReadFile(os.Getenv("CALLS"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(calls), "unexpected-setup") || a.state.Lab != config {
				t.Fatalf("trusted preparation lost binding or applied default setup: %s %+v", calls, a.state)
			}
			if fail != "" {
				if a.state.Phase != "error" || a.state.FailedOperation != "prepare" {
					t.Fatalf("failed phase became ready: %+v", a.state)
				}
				// No later phase is allowed to run after the failing stage.
				stages := []string{"preflight", "create", "infrastructure", "provision", "access", "status", "fixtures", "readiness"}
				failed := false
				for _, stage := range stages {
					if failed && strings.Contains(string(calls), stage) {
						t.Fatalf("continued after %s: %s", fail, calls)
					}
					failed = failed || stage == fail
				}
				return
			}
			if a.state.Phase != "ready" || a.state.Host != "192.0.2.1" {
				t.Fatalf("preparation failed: %+v", a.state)
			}
			if strings.Index(string(calls), "fixtures") > strings.Index(string(calls), "readiness") {
				t.Fatalf("readiness ran before fixtures: %s", calls)
			}
			for _, want := range []string{"preflight 3004", "create 3000 3004 2", "infrastructure 3004", "provision 3000 3004 2", "fixtures 3004", "readiness"} {
				if !strings.Contains(string(calls), want) {
					t.Fatalf("missing %s: %s", want, calls)
				}
			}
			log, err := os.ReadFile(filepath.Join(filepath.Dir(a.file), "operation.log"))
			if err != nil || !strings.Contains(string(log), "PHASE: fixtures") {
				t.Fatalf("trusted phases not streamed to operation log: %s %v", log, err)
			}
			// A retry must inspect existing VMs, never create them again.
			if err := os.WriteFile(os.Getenv("CALLS"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			a.prepareLab(false)
			calls, err = os.ReadFile(os.Getenv("CALLS"))
			if err != nil || strings.Contains(string(calls), "create ") || a.state.Phase != "ready" {
				t.Fatalf("unsafe retry: %s %v %+v", calls, err, a.state)
			}
		})
	}
}

func TestResetAndCaptureUseSavedLab(t *testing.T) {
	cleanLabEnv(t)
	t.Setenv("LAB_VM_START", "3100")
	t.Setenv("LAB_VM_END", "3102")
	t.Setenv("LAB_MARKER", "saved-owner")
	t.Setenv("LAB_SSH_USER", "saved-user")
	t.Setenv("LAB_KNOWN_HOSTS", filepath.Join(t.TempDir(), "saved-hosts"))
	a := testApp(t)
	config, err := resolveLab(examFixture(t)[0])
	if err != nil {
		t.Fatal(err)
	}
	a.state = state{Phase: "resetting", TestID: "CKA-001", Lab: config}
	config.CreationStarted = true
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("scripts", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("scripts/create-lab.sh", []byte("#!/bin/sh\nprintf '%s %s %s %s' \"$1\" \"$LAB_VM_START\" \"$LAB_VM_END\" \"$LAB_MARKER\" > reset-config\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAB_VM_START", "3200")
	t.Setenv("LAB_VM_END", "3204")
	t.Setenv("LAB_MARKER", "other-owner")
	t.Setenv("LAB_SSH_USER", "other-user")
	t.Setenv("LAB_KNOWN_HOSTS", "/other-hosts")
	stubCommand(t, "ssh", "#!/bin/sh\nprintf '%s\\n' \"$*\" > ssh-args\nprintf '{}'\n")
	snap := a.captureAnswer(a.labValue("LAB_SSH_USER", "milkman"), "192.0.2.1", task{Resource: "read-only-capture"})
	if snap.Error != "" {
		t.Fatal(snap.Error)
	}
	args, err := os.ReadFile("ssh-args")
	if err != nil || !strings.Contains(string(args), "saved-user@192.0.2.1") ||
		!strings.Contains(string(args), config.Env["LAB_KNOWN_HOSTS"]) || strings.Contains(string(args), "other") {
		t.Fatalf("capture used changed environment: %s %v", args, err)
	}
	w := httptest.NewRecorder()
	a.status(w, httptest.NewRequest("GET", "/api/status", nil))
	if strings.Contains(w.Body.String(), "saved-owner") || strings.Contains(w.Body.String(), `"lab"`) {
		t.Fatalf("operational snapshot exposed: %s", w.Body.String())
	}
	a.reset()
	b, err := os.ReadFile("reset-config")
	if err != nil || string(b) != "destroy 3100 3102 saved-owner" || a.state.Phase != "idle" || a.state.Lab != nil {
		t.Fatalf("reset used changed environment or lost state: %s %v %+v", b, err, a.state)
	}
}

func TestFiveNodeRolesAndVirtualIPDiscovery(t *testing.T) {
	cleanLabEnv(t)
	root := isolatedLabScripts(t)
	bin := t.TempDir()
	ssh := `#!/bin/bash
echo "$*" >> "$CALLS"
case "$*" in
  *"qm config "*) echo 'description: ha-test' ;;
  *"qm agent "*"network-get-interfaces"*)
    id=$(sed -n 's/.*qm agent \([0-9]*\).*/\1/p' <<<"$*")
    printf '[{"ip-addresses":[{"ip-address-type":"ipv4","prefix":24,"ip-address":"192.168.254.%s"}' "$((id-2990))"
    if [[ $id == 3003 ]]; then printf ',{"ip-address-type":"ipv4","prefix":32,"ip-address":"192.168.254.80"}'; fi
    printf ']}]\n' ;;
  *"qm guest exec "*"ssh_host_ed25519_key.pub"*) echo '{"exited":1,"exitcode":0,"out-data":"ssh-ed25519 YWJj lab"}' ;;
  *"qm guest exec "*) echo '{"exited":1,"exitcode":0}' ;;
  *"kubectl get nodes --no-headers"*) printf 'cp1\ncp2\nworker1\nworker2\n' ;;
  *'cat "$HOME/.ssh/painkiller_lab.pub"'*) echo 'ssh-ed25519 YWJj lab' ;;
esac
`
	for name, body := range map[string]string{
		"ssh": ssh, "ansible-playbook": "#!/bin/sh\necho \"$*\" >> \"$CALLS\"\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	known := filepath.Join(bin, "known")
	if err := os.WriteFile(known, []byte("# Painkiller managed host keys\n"), 0600); err != nil {
		t.Fatal(err)
	}
	vars := []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "CALLS=" + filepath.Join(bin, "calls"),
		"LAB_VM_START=3000", "LAB_VM_END=3004", "LAB_CONTROLLERS=3", "LAB_INITIAL_CONTROLLERS=2",
		"LAB_PROVISION_MODE=legacy", "LAB_CNI_PLUGIN=calico", "LAB_MARKER=ha-test", "LAB_KNOWN_HOSTS=" + known,
		"LAB_CONTROL_PLANE_ENDPOINT=192.168.254.80:8443",
	}
	run := func(action string) string {
		t.Helper()
		cmd := exec.Command("bash", filepath.Join(root, "scripts/create-lab.sh"), action)
		cmd.Dir = bin
		cmd.Env = append(os.Environ(), vars...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v %s", action, err, out)
		}
		return string(out)
	}
	var status struct {
		Controllers string `json:"controller_ips"`
		Workers     string `json:"worker_ips"`
	}
	if err := json.Unmarshal([]byte(run("status")), &status); err != nil {
		t.Fatal(err)
	}
	if status.Controllers != "192.168.254.10,192.168.254.11,192.168.254.12" ||
		status.Workers != "192.168.254.13,192.168.254.14" {
		t.Fatalf("reserved controller was reclassified or VIP became guest identity: %+v", status)
	}
	run("prerequisites")
	calls, err := os.ReadFile(filepath.Join(bin, "calls"))
	if err != nil || !strings.Contains(string(calls), "--tags inventory,prerequisites") ||
		strings.Contains(string(calls), "kubectl get nodes") {
		t.Fatalf("prerequisites bootstrapped the cluster: %s %v", calls, err)
	}
	run("provision") // four joined nodes, five prepared machines
	calls, err = os.ReadFile(filepath.Join(bin, "calls"))
	if err != nil || !strings.Contains(string(calls), "initial_controllers=2 preserve_existing=true") ||
		!strings.Contains(string(calls), "worker_ips=192.168.254.13,192.168.254.14") {
		t.Fatalf("wrong bootstrap roles: %s %v", calls, err)
	}
	// The candidate receives access to the reserved (third) controller as well.
	cmd := exec.Command("bash", "../../scripts/configure-exam-access.sh")
	cmd.Env = append(os.Environ(), vars...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("candidate access failed: %s %v", out, err)
	}
	calls, err = os.ReadFile(filepath.Join(bin, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"192.168.254.11", "192.168.254.12", "192.168.254.13", "192.168.254.14"} {
		if !strings.Contains(string(calls), "milkman@"+ip+" set -eu; mkdir") {
			t.Fatalf("candidate cannot reach %s: %s", ip, calls)
		}
	}
}

func TestRestoreRefusesDifferentLabBinding(t *testing.T) {
	a := testApp(t)
	a.exams = examFixture(t)
	a.state = state{Phase: "ready", TestID: "CKA-002", Lab: &labConfig{TestID: "CKA-001"}}
	if _, err := a.restoreExam(); err == nil || !strings.Contains(err.Error(), "refusing to change topology") {
		t.Fatalf("migrated an active lab to a different exam: %v", err)
	}
}

func TestInvalidLabMetadataFailsStartup(t *testing.T) {
	for _, spec := range []labSpec{
		{},
		{Controllers: 3, Workers: 2, InitialControllers: 4, Mode: "legacy", CNI: "calico", MemoryMB: 4096},
		{Controllers: 3, Workers: -1, InitialControllers: 2, Mode: "legacy", CNI: "calico", MemoryMB: 4096},
		{Controllers: 3, Workers: 2, InitialControllers: 2, Mode: "prebaked", CNI: "calico", MemoryMB: 4096},
		{Controllers: 3, Workers: 2, InitialControllers: 2, Mode: "legacy", CNI: "invalid", MemoryMB: 4096},
		{Controllers: 3, Workers: 2, InitialControllers: 2, Mode: "legacy", CNI: "calico", MemoryMB: 1024},
		{Controllers: 3, Workers: 2, InitialControllers: 2, Mode: "legacy", CNI: "calico", MemoryMB: 4096, Endpoint: "192.0.2.1:0"},
	} {
		exam := examFixture(t)[1]
		exam.Lab = &spec
		b, err := json.Marshal(exam)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(exam.dir, "test.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadExams(filepath.Dir(exam.dir)); err == nil {
			t.Fatalf("invalid lab accepted: %+v", spec)
		}
	}
}

func TestFailedCreatePreflightCannotDestroyExistingLab(t *testing.T) {
	cleanLabEnv(t)
	a := testApp(t)
	a.exams = examFixture(t)
	config, err := resolveLab(a.exams[0])
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("scripts", 0700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$1\" >> calls\necho 'VM 3000 already exists; refusing to create' >&2\nexit 1\n"
	if err := os.WriteFile("scripts/create-lab.sh", []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	a.state = state{Phase: "preparing", TestID: a.exams[0].ID, Lab: config}
	a.prepareLab(true)
	if a.state.Phase != "error" || config.CreationStarted {
		t.Fatalf("failed preflight authorized mutation: %+v", a.state)
	}
	a.reset()
	calls, err := os.ReadFile("calls")
	if err != nil || string(calls) != "preflight\n" || a.state.Phase != "idle" {
		t.Fatalf("reset touched an existing lab: %s %v %+v", calls, err, a.state)
	}
}
