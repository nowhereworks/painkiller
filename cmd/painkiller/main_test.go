package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func testApp(t *testing.T) *app {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TABLE attempt (id INTEGER PRIMARY KEY, state TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &app{db: db, state: state{Phase: "idle"}, terminals: map[*os.File]*exec.Cmd{}}
}
func TestStartPersistsDeadline(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "ready", Host: "192.0.2.1"}
	req := httptest.NewRequest("POST", "/api/start", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	a.action(w, req)
	if w.Code != 204 || a.state.Phase != "running" || time.Until(a.state.Deadline) < 119*time.Minute {
		t.Fatalf("unexpected start: %d %+v", w.Code, a.state)
	}
	var b []byte
	err := a.db.QueryRow(`SELECT state FROM attempt WHERE id=1`).Scan(&b)
	if err != nil {
		t.Fatal(err)
	}
	var saved state
	if err = json.Unmarshal(b, &saved); err != nil || !saved.Deadline.Equal(a.state.Deadline) {
		t.Fatalf("deadline not persisted: %v", err)
	}
}
func TestRejectInvalidTransition(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/start", nil))
	if w.Code != 409 || a.state.Phase != "idle" {
		t.Fatalf("invalid transition accepted: %d", w.Code)
	}
}

func TestRetryIsRestrictedToLabOperationFailures(t *testing.T) {
	for _, op := range []string{"persistence", ""} {
		a := testApp(t)
		a.state = state{Phase: "error", FailedOperation: op}
		w := httptest.NewRecorder()
		a.action(w, httptest.NewRequest("POST", "/api/retry", nil))
		if w.Code != 409 || a.state.Phase != "error" {
			t.Fatalf("retry accepted for %q: %d", op, w.Code)
		}
	}
}

// stubCommand puts an executable on PATH for the duration of the test.
func stubCommand(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func waitPhase(t *testing.T, a *app, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		phase := a.state.Phase
		a.mu.Unlock()
		if phase == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("phase %q was never reached", want)
}

func TestRetryResetRerunsDestroy(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("scripts", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("scripts/create-lab.sh", []byte("#!/bin/sh\necho \"$1\" >> calls\n"), 0700); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	a.file = filepath.Join(dir, "state.db")
	a.lockDir = dir
	a.state = state{Phase: "error", FailedOperation: "reset", Message: "destroy failed"}
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/retry", nil))
	if w.Code != 204 {
		t.Fatalf("reset retry rejected: %d", w.Code)
	}
	waitPhase(t, a, "idle")
	calls, err := os.ReadFile("calls")
	if err != nil || string(calls) != "destroy\n" {
		t.Fatalf("unexpected lab calls: %q %v", calls, err)
	}
}

func TestDuplicateSubmitGradesOnlyOnce(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Deadline: time.Now().Add(time.Hour), Host: "192.0.2.1"}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			a.action(w, httptest.NewRequest("POST", "/api/submit", nil))
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	seen := map[int]int{}
	for code := range codes {
		seen[code]++
	}
	if seen[204] != 1 || seen[409] != 1 {
		t.Fatalf("unexpected submit results: %v", seen)
	}
	waitPhase(t, a, "completed")
}

func TestExpiryClosesTerminalsAndGrades(t *testing.T) {
	a := testApp(t)
	f, err := os.CreateTemp(t.TempDir(), "terminal")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sleep", "30")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	a.terminals[f] = cmd
	a.state = state{Phase: "running", Deadline: time.Now().Add(-time.Second)}
	a.expire()
	if a.state.Phase != "grading" {
		t.Fatalf("expired attempt did not start grading: %s", a.state.Phase)
	}
	if len(a.terminals) != 0 {
		t.Fatal("expiry left terminals open")
	}
	waitPhase(t, a, "completed")
}

func TestResumeAfterRestart(t *testing.T) {
	preparing := testApp(t)
	preparing.state = state{Phase: "preparing"}
	if err := preparing.resume(); err != nil {
		t.Fatal(err)
	}
	if preparing.state.Phase != "error" || preparing.state.FailedOperation != "prepare" {
		t.Fatalf("interrupted preparation not retryable: %+v", preparing.state)
	}
	resetting := testApp(t)
	resetting.state = state{Phase: "resetting"}
	if err := resetting.resume(); err != nil {
		t.Fatal(err)
	}
	if resetting.state.Phase != "error" || resetting.state.FailedOperation != "reset" {
		t.Fatalf("interrupted reset not retryable: %+v", resetting.state)
	}
	running := testApp(t)
	deadline := time.Now().Add(time.Hour).Truncate(time.Second)
	running.state = state{Phase: "running", Deadline: deadline, Host: "192.0.2.1"}
	if err := running.resume(); err != nil {
		t.Fatal(err)
	}
	if running.state.Phase != "running" || !running.state.Deadline.Equal(deadline) {
		t.Fatalf("running attempt not preserved: %+v", running.state)
	}
	grading := testApp(t)
	grading.state = state{Phase: "grading", Host: "192.0.2.1"}
	if err := grading.resume(); err != nil {
		t.Fatal(err)
	}
	waitPhase(t, grading, "completed")
}

func TestGradingPersistenceFailureIsReported(t *testing.T) {
	stubCommand(t, "ssh", "#!/bin/sh\nprintf '%s' '{\"metadata\":{\"name\":\"ok\"}}'\n")
	a := testApp(t)
	a.state = state{Phase: "grading", Host: "192.0.2.1"}
	a.tasks = []task{{ID: "one", Points: 2, Resource: "kubectl get thing -o json", Checks: []check{{Description: "exists", Points: 2, Expression: `.metadata.name == "ok"`}}}}
	if _, err := a.db.Exec("DROP TABLE attempt"); err != nil {
		t.Fatal(err)
	}
	a.grade()
	if a.state.Phase != "error" || a.state.FailedOperation != "persistence" {
		t.Fatalf("persistence failure not surfaced: %+v", a.state)
	}
}

func TestCaptureRunsConcurrently(t *testing.T) {
	stubCommand(t, "ssh", "#!/bin/sh\nsleep 0.4\nprintf '%s' '{\"metadata\":{\"name\":\"ok\"}}'\n")
	a := testApp(t)
	a.state = state{Phase: "grading", Host: "192.0.2.1"}
	for i := 0; i < 6; i++ {
		a.tasks = append(a.tasks, task{ID: string(rune('a' + i)), Points: 2, Resource: "kubectl get thing -o json", Checks: []check{{Description: "exists", Points: 2, Expression: `.metadata.name == "ok"`}}})
	}
	start := time.Now()
	a.grade()
	elapsed := time.Since(start)
	// Six captures of 400 ms each take at least 2.4 s when run sequentially.
	if elapsed > 1800*time.Millisecond {
		t.Fatalf("captures were not concurrent: %v", elapsed)
	}
	for _, r := range a.state.Results {
		if r.Earned != 2 || r.Error != "" {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
}

func TestRetryPreparationReusesLabAndPreservesLog(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, path := range []string{"scripts", "exam/CKA-001", "bin"} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"scripts/create-lab.sh":            "#!/bin/sh\ncase \"$1\" in status) echo '{\"controller_ips\":\"192.0.2.1\",\"worker_ips\":\"192.0.2.2\"}';; provision) echo provisioned;; *) exit 88;; esac\n",
		"scripts/configure-exam-access.sh": "#!/bin/sh\necho configured\n",
		"bin/ssh":                          "#!/bin/sh\ncat >/dev/null\n",
		"exam/CKA-001/setup.yaml":          "kind: List\nitems: []\n",
		"operation.log":                    "earlier failure\n",
	}
	for name, body := range files {
		if err := os.WriteFile(name, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := testApp(t)
	a.file = filepath.Join(dir, "state.db")
	a.lockDir = dir
	a.exams = []examDefinition{{ID: "CKA-001", dir: "exam/CKA-001"}}
	a.state = state{Phase: "preparing", TestID: "CKA-001"}
	a.prepareLab(false)
	if a.state.Phase != "ready" || a.state.Host != "192.0.2.1" {
		t.Fatalf("retry failed: %+v", a.state)
	}
	b, err := os.ReadFile("operation.log")
	if err != nil || !strings.Contains(string(b), "earlier failure") || !strings.Contains(string(b), "Retrying preparation") {
		t.Fatalf("logs lost: %s %v", b, err)
	}
}

func TestFreshPreparationCreatesAndProvisions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, path := range []string{"scripts", "exam/CKA-001", "bin"} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"scripts/create-lab.sh":            "#!/bin/sh\necho \"$1\" >> calls\ncase \"$1\" in status) echo '{\"controller_ips\":\"192.0.2.1\"}';; preflight|create|provision) :;; *) exit 88;; esac\n",
		"scripts/configure-exam-access.sh": "#!/bin/sh\necho access >> calls\n",
		"bin/ssh":                          "#!/bin/sh\ncat >/dev/null\n",
		"exam/CKA-001/setup.yaml":          "kind: List\nitems: []\n",
	}
	for name, body := range files {
		if err := os.WriteFile(name, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := testApp(t)
	a.file = filepath.Join(dir, "state.db")
	a.lockDir = dir
	a.exams = []examDefinition{{ID: "CKA-001", dir: "exam/CKA-001"}}
	a.state = state{Phase: "preparing", TestID: "CKA-001"}
	a.prepareLab(true)
	if a.state.Phase != "ready" || a.state.Host != "192.0.2.1" {
		t.Fatalf("fresh preparation failed: %+v", a.state)
	}
	b, err := os.ReadFile("calls")
	if err != nil || string(b) != "preflight\ncreate\nprovision\naccess\nstatus\n" {
		t.Fatalf("unexpected preparation sequence: %q %v", b, err)
	}
}
func TestOrigin(t *testing.T) {
	r := httptest.NewRequest("POST", "http://localhost:8080/api/start", nil)
	if sameOrigin(r) {
		t.Fatal("missing origin accepted")
	}
	r.Header.Set("Origin", "http://evil.example")
	if sameOrigin(r) {
		t.Fatal("cross origin accepted")
	}
	r.Header.Set("Origin", "http://localhost:8080")
	if !sameOrigin(r) {
		t.Fatal("same origin rejected")
	}
}
func TestShellQuote(t *testing.T) {
	if got := shellQuote("it's ok"); got != "'it'\\''s ok'" {
		t.Fatal(got)
	}
}

func TestPersistenceFailureRejectsStart(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "ready", Host: "192.0.2.1"}
	if _, err := a.db.Exec("DROP TABLE attempt"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/start", nil))
	if w.Code != 503 || a.state.Phase != "ready" {
		t.Fatalf("start was not rolled back: %d %+v", w.Code, a.state)
	}
}

func TestExpiredAttemptFailsClosedWhenStorageFails(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Deadline: time.Now().Add(-time.Second)}
	if _, err := a.db.Exec("DROP TABLE attempt"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.terminal(w, httptest.NewRequest("GET", "/api/terminal", nil))
	if w.Code != 409 {
		t.Fatalf("expired terminal accepted: %d", w.Code)
	}
}

func TestTerminalRejectedAfterSubmission(t *testing.T) {
	for _, phase := range []string{"grading", "completed", "error"} {
		a := testApp(t)
		a.state = state{Phase: phase, Deadline: time.Now().Add(time.Hour)}
		w := httptest.NewRecorder()
		a.terminal(w, httptest.NewRequest("GET", "/api/terminal", nil))
		if w.Code != 409 {
			t.Fatalf("terminal allowed in %s: %d", phase, w.Code)
		}
	}
}

func TestValidateTasks(t *testing.T) {
	tasks := []task{{ID: "one", Title: "Title", Instructions: "Instructions", Solution: "Solution", Points: 100, Checks: []check{{Description: "part", Points: 100, Command: "true"}}}}
	if err := validateTasks(tasks); err != nil {
		t.Fatal(err)
	}
	tasks[0].Checks[0].Points = 99
	if err := validateTasks(tasks); err == nil {
		t.Fatal("accepted inconsistent partial points")
	}
}

func TestNormalizeLegacyScores(t *testing.T) {
	s := state{Results: []result{{ID: "passed", Passed: true}, {ID: "failed", Passed: false}, {ID: "partial", Earned: 2}}}
	if !normalizeLegacyScores(&s, []task{{ID: "passed", Points: 6}, {ID: "failed", Points: 6}, {ID: "partial", Points: 6}}) {
		t.Fatal("legacy score not migrated")
	}
	if s.Results[0].Earned != 6 || s.Results[1].Earned != 0 || s.Results[2].Earned != 2 {
		t.Fatalf("incorrect migration: %+v", s.Results)
	}
}

func TestPackagedExam(t *testing.T) {
	b, err := os.ReadFile("../../exam/CKA-001/tasks.json")
	if err != nil {
		t.Fatal(err)
	}
	var tasks []task
	if err := json.Unmarshal(b, &tasks); err != nil {
		t.Fatal(err)
	}
	if err := validateTasks(tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 29 {
		t.Fatalf("got %d questions", len(tasks))
	}
	for _, task := range tasks {
		if task.Explanation == "" {
			t.Fatalf("task %s lacks an explanation", task.ID)
		}
		for _, check := range task.Checks {
			for _, fixture := range []string{"null", "{}"} {
				cmd := exec.Command("jq", "-e", "try ("+check.Expression+") catch false")
				cmd.Stdin = strings.NewReader(fixture)
				if out, err := cmd.CombinedOutput(); err == nil || (string(out) != "false\n" && string(out) != "null\n") {
					t.Fatalf("%s invalid empty-state check: %v %s", task.ID, err, out)
				}
			}
		}
	}
}

func TestGradePartialAndInfrastructureFailure(t *testing.T) {
	dir := t.TempDir()
	ssh := `#!/bin/sh
case "$*" in
  *readyz*) exit 0 ;;
  *GOOD*) exit 0 ;;
  *BAD*) exit 1 ;;
  *BROKEN*) echo 'connection lost' >&2; exit 255 ;;
esac
exit 2
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := testApp(t)
	a.state = state{Phase: "grading", Host: "192.0.2.1"}
	a.tasks = []task{{ID: "partial", Points: 6, Checks: []check{{Description: "good", Points: 3, Command: "GOOD"}, {Description: "bad", Points: 3, Command: "BAD"}}}, {ID: "error", Points: 4, Check: "BROKEN"}}
	a.grade()
	if a.state.Phase != "completed" || a.state.Results[0].Earned != 3 || a.state.Results[0].Error != "" || a.state.Results[1].Error == "" {
		t.Fatalf("unexpected grading: %+v", a.state.Results)
	}
}

func TestDestroyRefusesUnownedVM(t *testing.T) {
	dir := t.TempDir()
	ssh := `#!/bin/sh
echo "$*" >> "$CALLS"
case "$*" in
  *"qm list"*) printf 'VMID NAME STATUS\n3000 somebody-elses-vm running\n' ;;
  *"qm config 3000"*) printf 'description: unrelated\n' ;;
  *) exit 99 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(dir, "calls")
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "destroy")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "LAB_VM_START=3000", "LAB_VM_END=3000", "LAB_CONTROLLERS=1")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "refusing operation") {
		t.Fatalf("unexpected result: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "qm stop") || strings.Contains(string(b), "qm destroy") {
		t.Fatalf("destructive command issued: %s", b)
	}
}

func TestDestroyPreflightsWholeRangeBeforeStopping(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	ssh := `#!/bin/sh
echo "$*" >> "$CALLS"
case "$*" in
 *"qm list"*) printf 'VMID NAME STATUS\n3000 ours running\n3001 unrelated running\n' ;;
 *"qm config 3000"*) echo 'description: painkiller-mvp' ;;
 *"qm config 3001"*) echo 'description: unrelated' ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "destroy")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "LAB_VM_START=3000", "LAB_VM_END=3001", "LAB_CONTROLLERS=1")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "refusing operation") {
		t.Fatalf("mixed range accepted: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "qm stop") || strings.Contains(string(b), "qm destroy") {
		t.Fatalf("partial destructive action: %s", b)
	}
}

func TestLabRejectsOutsideAuthorizedRange(t *testing.T) {
	for _, bounds := range [][2]string{{"2999", "3000"}, {"3000", "4000"}, {"9100", "9100"}} {
		cmd := exec.Command("bash", "../../scripts/create-lab.sh", "destroy")
		cmd.Env = append(os.Environ(), "LAB_VM_START="+bounds[0], "LAB_VM_END="+bounds[1], "LAB_CONTROLLERS=1")
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "within 3000") {
			t.Fatalf("unsafe range accepted: %v %s", err, out)
		}
	}
}

func TestLabAcceptsUpperAuthorizedBoundary(t *testing.T) {
	dir := t.TempDir()
	ssh := "#!/bin/sh\ncase \"$*\" in *'qm list'*) printf 'VMID NAME STATUS\\n';; *) exit 99;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "destroy")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "LAB_VM_START=3999", "LAB_VM_END=3999", "LAB_CONTROLLERS=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("upper authorized boundary rejected: %v %s", err, out)
	}
}

func TestPrebakedCreateRejectsNonTemplate(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	ssh := "#!/bin/sh\necho \"$*\" >> \"$CALLS\"\ncase \"$*\" in *'qm list'*) echo 'VMID NAME STATUS';; *'qm config 9100'*) printf 'description: unrelated\\n';; *) exit 99;; esac\n"
	git := "#!/bin/sh\ncase \"$*\" in *'rev-parse HEAD'*) echo 8ec676e93017ce3e110c27485e25d1f32d541056;; esac\n"
	for name, body := range map[string]string{"ssh": ssh, "git": git} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "create")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "LAB_PROVISION_MODE=prebaked", "LAB_TEMPLATE_ID=9100", "LAB_CONTROLLERS=1", "LAB_CNI_PLUGIN=flannel", "LAB_CONTROL_PLANE_ENDPOINT=")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "not a template") {
		t.Fatalf("non-template accepted: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil || strings.Contains(string(b), "qm clone") {
		t.Fatalf("unexpected clone: %s %v", b, err)
	}
}

func TestPrebakedCreateSelectsTemplateByID(t *testing.T) {
	dir := t.TempDir()
	ssh := "#!/bin/sh\ncase \"$*\" in *'qm config 9100'*) printf 'template: 1\\ndescription: unrelated\\n';; *'qm list'*) exit 1;; *) exit 99;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "create")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "LAB_PROVISION_MODE=prebaked", "LAB_TEMPLATE_ID=9100", "LAB_CONTROLLERS=1", "LAB_CNI_PLUGIN=flannel", "LAB_CONTROL_PLANE_ENDPOINT=")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "Could not list VMs") {
		t.Fatalf("template ID not accepted: %v %s", err, out)
	}
}

func TestImageBuilderRejectsOccupiedDestination(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	ssh := "#!/bin/sh\necho \"$*\" >> \"$CALLS\"\ncase \"$*\" in *'qm list'*) printf 'VMID NAME STATUS\\n9100 other stopped\\n';; *) exit 99;; esac\n"
	git := "#!/bin/sh\ncase \"$*\" in *'rev-parse HEAD'*) echo 8ec676e93017ce3e110c27485e25d1f32d541056;; esac\n"
	for name, body := range map[string]string{"ssh": ssh, "git": git} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "../../scripts/build-template.sh")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "PAINKILLER_LAB_LOCK_DIR="+dir, "LAB_IMAGE_TEMPLATE_ID=9100", "LAB_BASE_TEMPLATE_ID=9199")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "already exists") {
		t.Fatalf("occupied builder ID accepted: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil || strings.Contains(string(b), "qm clone") {
		t.Fatalf("unexpected clone: %s %v", b, err)
	}
}

func TestImageBuilderResumeRefusesUnownedOrPublishedVM(t *testing.T) {
	for _, description := range []string{"description: unrelated", "description: painkiller-image-v1\ntemplate: 1"} {
		dir := t.TempDir()
		calls := filepath.Join(dir, "calls")
		ssh := "#!/bin/sh\necho \"$*\" >> \"$CALLS\"\ncase \"$*\" in *'qm list'*) printf 'VMID NAME STATUS\\n9100 other stopped\\n';; *'qm config 9100'*) printf '%s\\n' \"$CONFIG\";; *) exit 99;; esac\n"
		git := "#!/bin/sh\ncase \"$*\" in *'rev-parse HEAD'*) echo 8ec676e93017ce3e110c27485e25d1f32d541056;; esac\n"
		for name, body := range map[string]string{"ssh": ssh, "git": git} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command("bash", "../../scripts/build-template.sh", "--resume")
		cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "CONFIG="+description, "PAINKILLER_LAB_LOCK_DIR="+dir, "LAB_IMAGE_TEMPLATE_ID=9100", "LAB_BASE_TEMPLATE_ID=9199")
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "not an unfinished owned builder") {
			t.Fatalf("unsafe builder resume accepted: %v %s", err, out)
		}
		b, err := os.ReadFile(calls)
		if err != nil || strings.Contains(string(b), "qm start") || strings.Contains(string(b), "qm clone") {
			t.Fatalf("unexpected mutation: %s %v", b, err)
		}
	}
}

func TestImageBuilderRejectsOutsideTemplateRange(t *testing.T) {
	for _, id := range []string{"3000", "9010", "9099", "9201", "x"} {
		cmd := exec.Command("bash", "../../scripts/build-template.sh")
		cmd.Env = append(os.Environ(), "LAB_IMAGE_TEMPLATE_ID="+id)
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "Invalid image build configuration") {
			t.Fatalf("unsafe image ID %s accepted: %v %s", id, err, out)
		}
	}
}

func TestImageBuilderSharesLabOperationLock(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "lab-operation.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "../../scripts/build-template.sh")
	cmd.Env = append(os.Environ(), "PAINKILLER_LAB_LOCK_DIR="+dir, "LAB_IMAGE_TEMPLATE_ID=9100", "LAB_BASE_TEMPLATE_ID=9199")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "Another lab operation") {
		t.Fatalf("builder ignored lab lock: %v %s", err, out)
	}
}

func TestPrebakedRejectsUnsupportedConfiguration(t *testing.T) {
	for _, setting := range []string{"LAB_CONTROLLERS=2", "LAB_CNI_PLUGIN=calico", "LAB_TEMPLATE_ID=9010", "LAB_CONTROL_PLANE_ENDPOINT=192.0.2.1:6443"} {
		cmd := exec.Command("bash", "../../scripts/create-lab.sh", "create")
		cmd.Env = append(os.Environ(), "LAB_PROVISION_MODE=prebaked", "LAB_VM_START=3000", "LAB_VM_END=3002", "LAB_TEMPLATE_ID=9100", "LAB_CONTROLLERS=1", "LAB_CNI_PLUGIN=flannel", "LAB_CONTROL_PLANE_ENDPOINT=", setting)
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "Prebaked mode requires") {
			t.Fatalf("unsupported configuration %s accepted: %v %s", setting, err, out)
		}
	}
}

// Copy only runtime inputs: no .git directory or external playbook checkout.
func isolatedLabScripts(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"scripts/create-lab.sh", "scripts/build-template.sh", "scripts/validate-image.sh", "scripts/playbooks.sh", "playbooks/SHA256SUMS", "playbooks/kubeadm-cluster/kubeadm-cluster.yml"} {
		b, err := os.ReadFile(filepath.Join("../..", name))
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, b, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLocalPlaybooksFailBeforeRemoteOperations(t *testing.T) {
	for _, action := range []string{"preflight", "create", "prerequisites", "provision", "build"} {
		for _, damage := range []string{"missing", "modified", "missing-manifest"} {
			t.Run(action+"/"+damage, func(t *testing.T) {
				root := isolatedLabScripts(t)
				file := filepath.Join(root, "playbooks/kubeadm-cluster/kubeadm-cluster.yml")
				if damage == "missing-manifest" {
					file = filepath.Join(root, "playbooks/SHA256SUMS")
				}
				if strings.HasPrefix(damage, "missing") {
					if err := os.Remove(file); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.WriteFile(file, []byte("invalid\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				bin := t.TempDir()
				calls := filepath.Join(bin, "calls")
				for _, name := range []string{"ssh", "git", "ansible-playbook"} {
					if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho unexpected >> \"$CALLS\"\nexit 99\n"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				script := "create-lab.sh"
				args := []string{filepath.Join(root, "scripts", script), action}
				if action == "build" {
					args = []string{filepath.Join(root, "scripts/build-template.sh")}
				}
				cmd := exec.Command("bash", args...)
				cmd.Dir = bin
				cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "PAINKILLER_LAB_LOCK_DIR="+bin, "LAB_PROVISION_MODE=legacy")
				if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "local playbook") {
					t.Fatalf("invalid local input accepted: %v %s", err, out)
				}
				if b, err := os.ReadFile(calls); !os.IsNotExist(err) {
					t.Fatalf("external command ran before validation: %s %v", b, err)
				}
			})
		}
	}
}

func TestLabCleanupDoesNotRequirePlaybookMetadata(t *testing.T) {
	root := isolatedLabScripts(t)
	if err := os.Rename(filepath.Join(root, "playbooks"), filepath.Join(root, "unavailable-playbooks")); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\ncase \"$*\" in *'qm list'*) echo 'VMID NAME STATUS';; *) exit 99;; esac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts/create-lab.sh"), "destroy")
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup requires playbooks: %v %s", err, out)
	}
}

func TestImageBuilderFullClonesWithoutChangingBaseOrIdentity(t *testing.T) {
	dir := t.TempDir()
	root := isolatedLabScripts(t)
	ssh := `#!/bin/sh
echo "$*" >> "$CALLS"
case "$*" in
 *"qm list"*) echo 'VMID NAME STATUS' ;;
 *"qm config 9199"*) printf 'template: 1\nmemory: 8192\nscsi0: test:base-9199-disk-0,size=1G\n' ;;
 *"qm config 9100"*) echo 'description: painkiller-image-v1' ;;
 *"pvesm status"*) echo 'test lvmthin active 9999999 0 9999999' ;;
 *"MemAvailable:"*) echo 65536 ;;
 *"qm agent 9100 network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.10"}]}]' ;;
 *"qm guest exec "*"ssh_host_ed25519_key.pub"*) echo '{"exited":1,"exitcode":0,"out-data":"ssh-ed25519 YWJj lab"}' ;;
 *"qm guest exec "*) echo '{"exited":1,"exitcode":0}' ;;
 *"qm status 9100"*) echo 'status: stopped' ;;
 *"bash -s"*) cat >> "$CALLS" ;;
esac
`
	git := "#!/bin/sh\necho 'unexpected git dependency' >> \"$CALLS\"\nexit 99\n"
	for name, body := range map[string]string{"ssh": ssh, "git": git, "ansible-playbook": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	calls := filepath.Join(dir, "calls")
	cmd := exec.Command("bash", filepath.Join(root, "scripts/build-template.sh"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "TMPDIR="+dir, "PAINKILLER_LAB_LOCK_DIR="+dir, "LAB_IMAGE_TEMPLATE_ID=9100", "LAB_BASE_TEMPLATE_ID=9199", "LAB_SSH_USER=milkman")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("image build failed: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"qm clone 9199 9100 --name painkiller-image-9100 --full 1", "qm shutdown 9100", "qm template 9100", "crictl pull"} {
		if !strings.Contains(string(b), required) {
			t.Fatalf("missing build step %q: %s", required, b)
		}
	}
	for _, forbidden := range []string{"unexpected git dependency", "qm set 9199", "qm start 9199", "--net0", "cloud-init clean", "machine-id", "ssh-keygen", "qm destroy"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("unsafe build step %q: %s", forbidden, b)
		}
	}
}

func TestPrebakedProvisionSkipsInstallAndReset(t *testing.T) {
	dir := t.TempDir()
	root := isolatedLabScripts(t)
	ssh := `#!/bin/sh
echo "$*" >> "$CALLS"
case "$*" in
 *"qm config 3000"*) echo 'description: painkiller-mvp' ;;
 *"qm agent 3000 network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.10"}]}]' ;;
 *"qm guest exec "*"ssh_host_ed25519_key.pub"*) echo '{"exited":1,"exitcode":0,"out-data":"ssh-ed25519 YWJj lab"}' ;;
 *"qm guest exec "*) echo '{"exited":1,"exitcode":0}' ;;
 *"kubectl get nodes --no-headers"*) echo node1 ;;
 *"bash -s -- 1.36.1"*) if [ "$FAIL_VALIDATE" = 1 ]; then exit 24; fi ;;
 *"bash -s -- 192.168.254.10"*) if [ "$FAIL_HEALTH" = 1 ]; then echo 'Existing control plane unhealthy' >&2; exit 25; fi ;;
esac
`
	git := "#!/bin/sh\necho 'unexpected git dependency' >> \"$CALLS\"\nexit 99\n"
	ansible := "#!/bin/sh\necho \"$*\" >> \"$CALLS\"\necho \"$*\" > \"$CAPTURE\"\n"
	for name, body := range map[string]string{"ssh": ssh, "git": git, "ansible-playbook": ansible} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	known := filepath.Join(dir, "known")
	if err := os.WriteFile(known, []byte("# Painkiller managed host keys\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "scripts/create-lab.sh")
	cmd := exec.Command("bash", script, "provision")
	cmd.Dir = dir
	calls := filepath.Join(dir, "calls")
	capture := filepath.Join(dir, "captured.yml")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "CAPTURE="+capture, "LAB_PROVISION_MODE=prebaked", "LAB_TEMPLATE_ID=9100", "LAB_VM_START=3000", "LAB_VM_END=3000", "LAB_CONTROLLERS=1", "LAB_CNI_PLUGIN=flannel", "LAB_CONTROL_PLANE_ENDPOINT=", "LAB_KNOWN_HOSTS="+known)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prebaked provision failed: %v %s", err, out)
	}
	b, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"kubeadm-cluster/kubeadm-cluster.yml", "--skip-tags prerequisites,images", "preserve_existing=true", "flannel_manifest_url=/opt/painkiller/flannel.yml"} {
		if !strings.Contains(string(b), required) {
			t.Fatalf("missing prebaked provision arg %q: %s", required, b)
		}
	}
	if strings.Contains(string(b), "--tags inventory,prerequisites") {
		t.Fatalf("prebaked provision must skip preparation, not select it: %s", b)
	}
	b, err = os.ReadFile(calls)
	if err != nil || strings.Contains(string(b), "unexpected git dependency") || !strings.Contains(string(b), "flannel_manifest_url=/opt/painkiller/flannel.yml") {
		t.Fatalf("unexpected provision commands: %s %v", b, err)
	}
	failedCapture := filepath.Join(dir, "must-not-exist.yml")
	cmd2 := exec.Command("bash", script, "provision")
	cmd2.Dir = dir
	cmd2.Env = append(cmd.Env, "FAIL_VALIDATE=1", "CAPTURE="+failedCapture)
	if out, err := cmd2.CombinedOutput(); err == nil || !strings.Contains(string(out), "Incompatible image") {
		t.Fatalf("invalid image accepted: %v %s", err, out)
	}
	if _, err := os.Stat(failedCapture); !os.IsNotExist(err) {
		t.Fatalf("bootstrap ran despite invalid image: %v", err)
	}
	cmd3 := exec.Command("bash", script, "provision")
	cmd3.Dir = dir
	cmd3.Env = append(cmd.Env, "FAIL_HEALTH=1", "CAPTURE="+failedCapture)
	if out, err := cmd3.CombinedOutput(); err == nil || !strings.Contains(string(out), "Existing control plane unhealthy") {
		t.Fatalf("unhealthy control plane accepted: %v %s", err, out)
	}
	if _, err := os.Stat(failedCapture); !os.IsNotExist(err) {
		t.Fatalf("bootstrap ran despite unhealthy control plane: %v", err)
	}
}

func TestSelectionPersistsAndRejectsUnknownTask(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Deadline: time.Now().Add(time.Hour)}
	a.tasks = []task{{ID: "one"}, {ID: "two"}}
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/select", strings.NewReader(`{"id":"two"}`)))
	if w.Code != 204 || a.state.Selected != "two" {
		t.Fatalf("selection not persisted: %d %+v", w.Code, a.state)
	}
	w = httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/select", strings.NewReader(`{"id":"other"}`)))
	if w.Code != 400 || a.state.Selected != "two" {
		t.Fatalf("unknown selection accepted: %d", w.Code)
	}
}

func TestPerTaskMarksDoNotOverwriteOtherTasks(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Deadline: time.Now().Add(time.Hour)}
	a.tasks = []task{{ID: "one"}, {ID: "two"}}
	for _, body := range []string{`{"id":"one","flag":true}`, `{"id":"two","done":true}`} {
		w := httptest.NewRecorder()
		a.action(w, httptest.NewRequest("POST", "/api/mark", strings.NewReader(body)))
		if w.Code != 204 {
			t.Fatalf("mark failed: %d", w.Code)
		}
	}
	if !a.state.Flags["one"] || !a.state.Done["two"] {
		t.Fatalf("marks overwritten: %+v", a.state)
	}
}

func TestMarkPersistenceFailureDoesNotMutateState(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Deadline: time.Now().Add(time.Hour), Flags: map[string]bool{"one": false}}
	a.tasks = []task{{ID: "one"}}
	if _, err := a.db.Exec("DROP TABLE attempt"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/mark", strings.NewReader(`{"id":"one","flag":true}`)))
	if w.Code != 503 || a.state.Flags["one"] {
		t.Fatalf("unsaved mark leaked into state: %d %+v", w.Code, a.state.Flags)
	}
}

func TestLabOperationLockRejectsConcurrentOperation(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "lab-operation.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	a := &app{file: filepath.Join(dir, "state.db")}
	if out, err := a.run("true"); err == nil || !strings.Contains(out, "Another lab operation") {
		t.Fatal("concurrent lab operation was accepted")
	}
}

func TestConcurrentStartOnlyStartsOnce(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "ready", Host: "192.0.2.1"}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			a.action(w, httptest.NewRequest("POST", "/api/start", nil))
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	seen := map[int]int{}
	for code := range codes {
		seen[code]++
	}
	if seen[204] != 1 || seen[409] != 1 {
		t.Fatalf("unexpected results: %v", seen)
	}
}

func TestConfigureAccessUsesOnlyDiscoveredLabNodes(t *testing.T) {
	dir := t.TempDir()
	ssh := `#!/bin/sh
echo "$*" >> "$CALLS"
case "$*" in
  *"qm config 3000"*|*"qm config 3001"*|*"qm config 3002"*) echo 'description: painkiller-mvp' ;;
  *"qm agent 3000 network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.10"}]}]' ;;
  *"qm agent 3001 network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.11"}]}]' ;;
  *"qm agent 3002 network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.12"}]}]' ;;
  *"cat \"\$HOME/.ssh/painkiller_lab.pub\""*) echo 'ssh-ed25519 QUJDREVGRw== lab' ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	known := filepath.Join(dir, "known")
	if err := os.WriteFile(known, []byte("# test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "../../scripts/configure-exam-access.sh")
	calls := filepath.Join(dir, "calls")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "LAB_KNOWN_HOSTS="+known, "LAB_VM_START=3000", "LAB_VM_END=3002", "LAB_CONTROLLERS=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configure failed: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "milkman@192.168.254.11") || !strings.Contains(string(b), "milkman@192.168.254.12") {
		t.Fatalf("workers not configured: %s", b)
	}
}

func TestCreateUsesLinkedClonesAndWaitsForReady(t *testing.T) {
	for _, mode := range []string{"", "legacy", "prebaked"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			ssh := `#!/bin/bash
echo "$*" >> "$CALLS"
case "$*" in
  *"qm list"*) echo 'VMID NAME STATUS' ;;
  *"MemAvailable:"*) echo 65536 ;;
  *"qm config 9100"*|*"qm config 9010"*) printf 'template: 1\ndescription: painkiller-image-v1-1.36.1-8ec676e93017ce3e110c27485e25d1f32d541056\n' ;;
  *"qm config "*) echo 'description: painkiller-mvp' ;;
  *"qm guest exec "*"ssh_host_ed25519_key.pub"*)
    id=$(echo "$*" | sed -n 's/.*qm guest exec \([0-9]*\).*/\1/p'); material=$(printf '%s' "$id" | base64 -w0); printf '{"exited":1,"exitcode":0,"out-data":"ssh-ed25519 %s lab"}\n' "$material" ;;
  *"qm agent "*"network-get-interfaces"*)
    id=$(echo "$*" | sed -n 's/.*qm agent \([0-9]*\).*/\1/p')
    printf '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.%s"}]}]\n' "$((id-2990))" ;;
esac
`
			git := `#!/bin/sh
echo 'unexpected git dependency' >> "$CALLS"
exit 99
`
			for name, body := range map[string]string{"ssh": ssh, "git": git, "sleep": "#!/bin/sh\nexit 0\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			root := isolatedLabScripts(t)
			cmd := exec.Command("bash", filepath.Join(root, "scripts/create-lab.sh"), "create")
			cmd.Dir = dir
			calls := filepath.Join(dir, "calls")
			template := "9010"
			if mode != "legacy" {
				template = "9100"
			}
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "LAB_KNOWN_HOSTS="+filepath.Join(dir, "known"), "LAB_VM_START=3000", "LAB_VM_END=3002", "LAB_CONTROLLERS=1", "LAB_PROVISION_MODE="+mode, "LAB_TEMPLATE_ID=")
			preflight := exec.Command("bash", filepath.Join(root, "scripts/create-lab.sh"), "preflight")
			preflight.Dir, preflight.Env = cmd.Dir, cmd.Env
			if out, err := preflight.CombinedOutput(); err != nil {
				t.Fatalf("preflight failed: %v %s", err, out)
			}
			before, err := os.ReadFile(calls)
			if err != nil || strings.Contains(string(before), "qm clone") || strings.Contains(string(before), "qm set") || strings.Contains(string(before), "qm start") {
				t.Fatalf("preflight changed VMs: %s %v", before, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "known")); !os.IsNotExist(err) {
				t.Fatalf("preflight changed the host-key file: %v", err)
			}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("create failed: %v %s", err, out)
			}
			b, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			log := string(b)
			if strings.Contains(log, "unexpected git dependency") {
				t.Fatalf("create requires Git: %s", log)
			}
			for _, id := range []string{"3000", "3001", "3002"} {
				if !strings.Contains(log, "qm clone "+template+" "+id+" --name k8s-"+id+" --full 0 --description painkiller-mvp") {
					t.Fatalf("expected a linked clone for %s: %s", id, log)
				}
				if !strings.Contains(log, "qm start "+id) {
					t.Fatalf("VM %s was not started: %s", id, log)
				}
				if strings.Contains(log, "qm set "+id+" --net0") || strings.Contains(log, "qm reset "+id) || strings.Contains(log, "qm guest exec "+id+" -- sh -c 'set -e; rm -f /var/lib/dbus/machine-id") {
					t.Fatalf("VM %s was subjected to network/identity manipulation: %s", id, log)
				}
				if !strings.Contains(log, "qm guest exec "+id+" -- cat /etc/ssh/ssh_host_ed25519_key.pub") {
					t.Fatalf("host key for VM %s was not read via the guest agent: %s", id, log)
				}
				if strings.Contains(log, "qm stop "+id) || strings.Contains(log, "qm destroy "+id) {
					t.Fatalf("successful create stopped/destroyed VM %s: %s", id, log)
				}
			}
		})
	}
}

func TestRemoteRetriesTransientConnectionFailure(t *testing.T) {
	dir := t.TempDir()
	ssh := `#!/bin/sh
n=$(cat "$COUNT" 2>/dev/null || echo 0); n=$((n+1)); echo "$n" > "$COUNT"
if [ "$n" -le 2 ]; then
  echo 'ssh: Could not resolve hostname prox-k12.nowhereworks.local: Name or service not known' >&2
  exit 255
fi
case "$*" in
  *"qm config "*) echo 'description: painkiller-mvp' ;;
  *"qm agent "*"network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.10"}]}]' ;;
esac
`
	for name, body := range map[string]string{"ssh": ssh, "sleep": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "status")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "COUNT="+filepath.Join(dir, "count"), "LAB_VM_START=3000", "LAB_VM_END=3000", "LAB_CONTROLLERS=1")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "192.168.254.10") {
		t.Fatalf("transient failure not retried: %v %s", err, out)
	}
}

func TestRemoteDoesNotRetryCommandError(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	ssh := `#!/bin/sh
echo "$*" >> "$CALLS"
echo 'ERROR: configuration file does not exist' >&2
exit 2
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "status")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "LAB_VM_START=3000", "LAB_VM_END=3000", "LAB_CONTROLLERS=1")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "configuration file does not exist") {
		t.Fatalf("command error not surfaced: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "qm config"); n != 1 {
		t.Fatalf("command error was retried %d times: %s", n, b)
	}
}

func TestCreateWillNotOverwriteUnmanagedKnownHosts(t *testing.T) {
	dir := t.TempDir()
	known := filepath.Join(dir, "known_hosts")
	for name, body := range map[string]string{
		"ssh":         "#!/bin/sh\necho 'VMID NAME STATUS'\n",
		"git":         "#!/bin/sh\ncase \"$*\" in *'rev-parse HEAD'*) echo 8ec676e93017ce3e110c27485e25d1f32d541056;; esac\n",
		"known_hosts": "user data\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "create")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "LAB_KNOWN_HOSTS="+known, "LAB_VM_START=3000", "LAB_VM_END=3002", "LAB_CONTROLLERS=1", "LAB_PROVISION_MODE=legacy")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "Refusing to overwrite unmanaged") {
		t.Fatalf("unmanaged file accepted: %v %s", err, out)
	}
	b, err := os.ReadFile(known)
	if err != nil || string(b) != "user data\n" {
		t.Fatalf("file changed: %q %v", b, err)
	}
}

func TestCreateRefusesOccupiedIDBeforeCloning(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	ssh := "#!/bin/sh\necho \"$*\" >> \"$CALLS\"\ncase \"$*\" in *'qm list'*) printf 'VMID NAME STATUS\\n3000 existing-vm running\\n';; esac\n"
	git := "#!/bin/sh\ncase \"$*\" in *'rev-parse HEAD'*) echo 8ec676e93017ce3e110c27485e25d1f32d541056;; esac\n"
	for name, body := range map[string]string{"ssh": ssh, "git": git} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "../../scripts/create-lab.sh", "create")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "LAB_VM_START=3000", "LAB_VM_END=3002", "LAB_CONTROLLERS=1", "LAB_PROVISION_MODE=legacy")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "already exists") {
		t.Fatalf("occupied ID accepted: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "qm clone") || strings.Contains(string(b), "qm set") {
		t.Fatalf("mutation issued: %s", b)
	}
}

func TestProvisionConfiguresOnlyOwnedLabVMs(t *testing.T) {
	dir := t.TempDir()
	root := isolatedLabScripts(t)
	ssh := `#!/bin/sh
echo "$*" >> "$CALLS"
case "$*" in
 *"qm config 3000"*|*"qm config 3001"*|*"qm config 3002"*) echo 'description: painkiller-mvp' ;;
 *"qm agent 3000 network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.10"}]}]' ;;
 *"qm agent 3001 network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.11"}]}]' ;;
 *"qm agent 3002 network-get-interfaces"*) echo '[{"ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.168.254.12"}]}]' ;;
 *"qm guest exec "*"ssh_host_ed25519_key.pub"*) id=$(echo "$*" | sed -n 's/.*qm guest exec \([0-9]*\).*/\1/p'); material=$(printf '%s' "$id" | base64 -w0); printf '{"exited":1,"exitcode":0,"out-data":"ssh-ed25519 %s lab"}\n' "$material" ;;
 *"qm guest exec "*) echo '{"exited":1,"exitcode":0}' ;;
 *"kubectl get nodes --no-headers"*) printf 'node1\nnode2\nnode3\n' ;;
esac
`
	git := `#!/bin/sh
echo 'unexpected git dependency' >> "$CALLS"
exit 99
`
	for name, body := range map[string]string{"ssh": ssh, "git": git, "ansible-playbook": "#!/bin/sh\necho \"$*\" >> \"$CALLS\"\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(root, "scripts/create-lab.sh")
	cmd := exec.Command("bash", script, "provision")
	cmd.Dir = dir
	calls := filepath.Join(dir, "calls")
	if err := os.WriteFile(filepath.Join(dir, "known"), []byte("# Painkiller managed host keys\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls, "LAB_VM_START=3000", "LAB_VM_END=3002", "LAB_CONTROLLERS=1", "LAB_KNOWN_HOSTS="+filepath.Join(dir, "known"), "LAB_PROVISION_MODE=legacy")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("provision failed: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	log := string(b)
	if strings.Contains(log, "unexpected git dependency") || !strings.Contains(log, filepath.Join(root, "playbooks/kubeadm-cluster/kubeadm-cluster.yml")) {
		t.Fatalf("legacy provisioning did not use only local playbooks: %s", log)
	}
	for _, id := range []string{"3000", "3001", "3002"} {
		if !strings.Contains(log, "qm guest exec "+id+" -- sh -c 'set -e; id milkman") {
			t.Fatalf("sudo not configured for %s: %s", id, log)
		}
	}
	if !strings.Contains(log, `"local_kubeconfig_dir":"`+dir+`/data/kubeconfigs/exam.`) {
		t.Fatalf("kubeconfig output not isolated: %s", log)
	}
}

func TestReferenceAnswersRefuseUnmarkedCluster(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$*\" >> \"$CALLS\"\nprintf '%s\\n' '{\"metadata\":{\"labels\":{}}}'\n"
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "../../exam/CKA-001/reference-solutions.sh")
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+calls)
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "Refusing to modify") {
		t.Fatalf("unmarked cluster accepted: %v %s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "create") || strings.Contains(string(b), "apply") {
		t.Fatalf("mutating command was issued: %s", b)
	}
}

func TestSnapshotGradingIsImmutableAndPrivate(t *testing.T) {
	dir := t.TempDir()
	ssh := "#!/bin/sh\nprintf '%s' '{\"value\":\"correct\"}'\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := testApp(t)
	a.state = state{Phase: "grading", Host: "192.0.2.1"}
	a.tasks = []task{{ID: "one", Points: 4, Resource: "kubectl get thing -o json", Checks: []check{{Description: "value", Points: 4, Expression: `.value == "correct"`}}}}
	a.grade()
	if a.state.Results[0].Earned != 4 {
		t.Fatalf("unexpected score: %+v", a.state.Results)
	}
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\nprintf '%s' '{\"value\":\"wrong\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := a.db.QueryRow(`SELECT state FROM attempt WHERE id=1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var restored state
	if err := json.Unmarshal([]byte(stored), &restored); err != nil {
		t.Fatal(err)
	}
	a.state = restored
	a.grade()
	if a.state.Results[0].Earned != 4 {
		t.Fatal("regrade used changed live answer")
	}
	w := httptest.NewRecorder()
	a.status(w, httptest.NewRequest("GET", "/api/status", nil))
	if strings.Contains(w.Body.String(), "correct") || strings.Contains(w.Body.String(), "answers") {
		t.Fatal("status exposed private answer snapshot")
	}
}

func TestMissingNamespaceIsIncorrectNotInfrastructureError(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'Error from server (NotFound): namespaces missing' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := testApp(t)
	a.state = state{Phase: "grading", Host: "192.0.2.1"}
	a.tasks = []task{{ID: "one", Points: 4, Resource: "kubectl get pod test", Checks: []check{{Description: "exists", Points: 4, Expression: `.metadata.name == "test"`}}}}
	a.grade()
	if a.state.Results[0].Error != "" || a.state.Results[0].Earned != 0 {
		t.Fatalf("unexpected grading result: %+v", a.state.Results[0])
	}
}

func TestCaptureRetriesTransientFailure(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ ! -e \"$FLAG\" ]; then touch \"$FLAG\"; exit 255; fi\nprintf '%s' '{\"metadata\":{\"name\":\"test\"}}'\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FLAG", filepath.Join(dir, "first"))
	a := testApp(t)
	a.state = state{Phase: "grading", Host: "192.0.2.1"}
	a.tasks = []task{{ID: "one", Points: 4, Resource: "kubectl get pod test", Checks: []check{{Description: "exists", Points: 4, Expression: `.metadata.name == "test"`}}}}
	a.grade()
	if a.state.Results[0].Earned != 4 || a.state.Results[0].Error != "" {
		t.Fatalf("retry failed: %+v", a.state.Results[0])
	}
}

// fixtures maps every packaged task to one captured document that satisfies
// all of its checks. Real captures come from exam/CKA-001/tasks.json "resource" commands.
var fixtures = map[string]string{
	"namespace":      `{"metadata":{"name":"exam-sandbox"}}`,
	"node-label":     `{"items":[{"metadata":{"name":"k8s-3000"}}]}`,
	"node-placement": `{"items":[{"spec":{"taints":[{"key":"exam","value":"reserved","effect":"NoSchedule"}],"unschedulable":true}}]}`,
	"static-pod":     `{"items":[{"metadata":{"name":"static-web-k8s-3001"},"status":{"phase":"Running"}}]}`,
	"etcd-snapshot":  `{"hash":1476058848,"revision":856,"totalKey":867,"totalSize":1818624}`,
	"pod":            `{"spec":{"containers":[{"image":"nginx:1.27"}]}}`,
	"deployment":     `{"spec":{"replicas":2,"template":{"spec":{"containers":[{"image":"nginx:1.27"}]}}}}`,
	"rollout":        `{"metadata":{"annotations":{"deployment.kubernetes.io/revision":"3"}},"spec":{"template":{"spec":{"containers":[{"name":"web","image":"nginx:1.27"}]}}}}`,
	"multicontainer": `{"spec":{"initContainers":[{"name":"prepare","image":"busybox:1.36"}],"containers":[{"name":"app","image":"nginx:1.27"},{"name":"log-sidecar","image":"busybox:1.36"}]},"status":{"phase":"Running"}}`,
	"resources":      `{"spec":{"template":{"spec":{"containers":[{"resources":{"requests":{"cpu":"100m","memory":"128Mi"},"limits":{"cpu":"500m","memory":"256Mi"}}}]}}}}`,
	"limits":         `{"items":[{"kind":"ResourceQuota","metadata":{"name":"limit-pods"},"spec":{"hard":{"pods":"20"}}},{"kind":"LimitRange","metadata":{"name":"default-limits"},"spec":{"limits":[{"type":"Container","default":{"cpu":"200m","memory":"256Mi"},"defaultRequest":{"cpu":"100m","memory":"128Mi"}}]}}]}`,
	"priorityclass":  `{"value":100000,"globalDefault":false}`,
	"job":            `{"spec":{"template":{"spec":{"containers":[{"image":"busybox:1.36","command":["echo","hello"]}]}}}}`,
	"cronjob":        `{"spec":{"schedule":"*/5 * * * *","jobTemplate":{"spec":{"template":{"spec":{"containers":[{"image":"busybox:1.36","command":["echo","hello"]}]}}}}}}`,
	"configmap":      `{"data":{"mode":"production"}}`,
	"secret":         `{"data":{"username":"b3BlcmF0b3I="}}`,
	"serviceaccount": `{"metadata":{"name":"observer"}}`,
	"pvc-binding":    `{"items":[{"kind":"PersistentVolume","metadata":{"name":"exam-pv"},"spec":{"capacity":{"storage":"1Gi"},"accessModes":["ReadWriteOnce"],"storageClassName":"exam-local","hostPath":{"path":"/tmp/exam-data"}}},{"kind":"PersistentVolumeClaim","metadata":{"name":"exam-claim"},"spec":{"resources":{"requests":{"storage":"1Gi"}},"storageClassName":"exam-local","accessModes":["ReadWriteOnce"],"volumeName":"exam-pv"},"status":{"phase":"Bound"}}]}`,
	"mount-config":   `{"spec":{"volumes":[{"name":"config","configMap":{"name":"exam-material"}}],"containers":[{"name":"web","image":"nginx:1.27","volumeMounts":[{"name":"config","mountPath":"/etc/exam"}]}]},"status":{"phase":"Running"}}`,
	"service":        `{"spec":{"type":"ClusterIP","selector":{"app":"service-target"},"ports":[{"port":80,"targetPort":80}]}}`,
	"nodeport":       `{"spec":{"type":"NodePort","selector":{"app":"service-target"},"ports":[{"port":80,"targetPort":80,"nodePort":30080}]}}`,
	"ingress":        `{"spec":{"rules":[{"host":"exam.local","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"exam-backend","port":{"number":80}}}}]}}]}}`,
	"networkpolicy":  `{"spec":{"podSelector":{},"policyTypes":["Ingress"]}}`,
	"coredns":        `{"spec":{"replicas":3},"status":{"availableReplicas":3}}`,
	"repair":         `{"spec":{"template":{"spec":{"containers":[{"name":"web","image":"nginx:1.27"}]}}},"status":{"availableReplicas":1}}`,
	"svc-selector":   `{"items":[{"metadata":{"labels":{"kubernetes.io/service-name":"broken-endpoints"}},"endpoints":[{"addresses":["10.244.1.5"],"conditions":{"ready":true}}]}]}`,
	"crashloop":      `{"status":{"availableReplicas":1}}`,
	"rbac":           `{"items":[{"kind":"Role","metadata":{"name":"pod-reader"},"rules":[{"apiGroups":[""],"resources":["pods"],"verbs":["get","list","watch"]}]},{"kind":"RoleBinding","metadata":{"name":"read-pods"},"roleRef":{"kind":"Role","name":"pod-reader"},"subjects":[{"kind":"ServiceAccount","name":"binding-observer","namespace":"exam-workloads"}]}]}`,
	"cluster-rbac":   `{"items":[{"kind":"ClusterRole","metadata":{"name":"node-reader"},"rules":[{"apiGroups":[""],"resources":["nodes"],"verbs":["get","list","watch"]}]},{"kind":"ClusterRoleBinding","metadata":{"name":"node-reader"},"roleRef":{"kind":"ClusterRole","name":"node-reader"},"subjects":[{"kind":"ServiceAccount","name":"cluster-observer","namespace":"exam-workloads"}]}]}`,
}

func TestExamChecksAcceptCorrectFixtures(t *testing.T) {
	for _, task := range packagedTasks(t) {
		for _, c := range task.Checks {
			cmd := exec.Command("jq", "-e", "if . == null then false else ("+c.Expression+") end")
			cmd.Stdin = strings.NewReader(fixtures[task.ID])
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s / %s rejected correct fixture: %v %s", task.ID, c.Description, err, out)
			}
		}
	}
}

func packagedTasks(t *testing.T) []task {
	t.Helper()
	b, err := os.ReadFile("../../exam/CKA-001/tasks.json")
	if err != nil {
		t.Fatal(err)
	}
	var tasks []task
	if err = json.Unmarshal(b, &tasks); err != nil {
		t.Fatal(err)
	}
	return tasks
}

// scoreFixture evaluates every check of a task against one captured document,
// mirroring the grading path.
func scoreFixture(t *testing.T, task task, fixture string) int {
	t.Helper()
	score := 0
	for _, c := range task.Checks {
		cmd := exec.Command("jq", "-e", "try (("+c.Expression+") // false) catch false")
		cmd.Stdin = strings.NewReader(fixture)
		out, err := cmd.CombinedOutput()
		if err == nil {
			score += c.Points
			continue
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatalf("%s / %s is not a boolean expression: %v %s", task.ID, c.Description, err, out)
		}
	}
	return score
}

// TestFixturesScoreEveryTask guarantees each packaged task can reach its full
// score from a single captured document, and that multi-check tasks award
// partial credit for exactly the failing checks.
func TestFixturesScoreEveryTask(t *testing.T) {
	for _, task := range packagedTasks(t) {
		fixture, ok := fixtures[task.ID]
		if !ok {
			t.Errorf("task %s has no correct fixture", task.ID)
			continue
		}
		if score := scoreFixture(t, task, fixture); score != task.Points {
			t.Errorf("%s scored %d of %d", task.ID, score, task.Points)
		}
		if score := scoreFixture(t, task, `{}`); score != 0 {
			t.Errorf("%s scored %d on an empty document", task.ID, score)
		}
		if score := scoreFixture(t, task, `null`); score != 0 {
			t.Errorf("%s scored %d on a null document", task.ID, score)
		}
	}
}

func TestFixtureWinsPartialCredit(t *testing.T) {
	tasks := packagedTasks(t)
	cases := []struct {
		id, fixture string
		want        int
	}{
		// Image correct, replica count wrong: 2 of 4.
		{"deployment", `{"spec":{"replicas":1,"template":{"spec":{"containers":[{"image":"nginx:1.27"}]}}}}`, 2},
		// Image rolled out but not yet rolled back: 2 of 4.
		{"rollout", `{"metadata":{"annotations":{"deployment.kubernetes.io/revision":"2"}},"spec":{"template":{"spec":{"containers":[{"name":"web","image":"nginx:1.26"}]}}}}`, 2},
		// Volume and claim configured but not bound: 4 of 6.
		{"pvc-binding", `{"items":[{"kind":"PersistentVolume","metadata":{"name":"exam-pv"},"spec":{"capacity":{"storage":"1Gi"},"accessModes":["ReadWriteOnce"],"storageClassName":"exam-local","hostPath":{"path":"/tmp/exam-data"}}},{"kind":"PersistentVolumeClaim","metadata":{"name":"exam-claim"},"spec":{"resources":{"requests":{"storage":"1Gi"}},"storageClassName":"exam-local","accessModes":["ReadWriteOnce"]},"status":{"phase":"Pending"}}]}`, 4},
		// Ingress routes correctly but the path type is wrong: 4 of 5.
		{"ingress", `{"spec":{"rules":[{"host":"exam.local","http":{"paths":[{"path":"/","pathType":"ImplementationSpecific","backend":{"service":{"name":"exam-backend","port":{"number":80}}}}]}}]}}`, 4},
		// Service repaired but the endpoint is not ready yet: 0 of 4.
		{"svc-selector", `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":false}}]}]}`, 0},
	}
	for _, c := range cases {
		var found bool
		for _, task := range tasks {
			if task.ID != c.id {
				continue
			}
			found = true
			if score := scoreFixture(t, task, c.fixture); score != c.want {
				t.Errorf("%s scored %d, want %d", c.id, score, c.want)
			}
		}
		if !found {
			t.Errorf("task %s not found", c.id)
		}
	}
}
