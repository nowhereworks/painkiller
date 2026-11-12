package main

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestPackagedExam(t *testing.T) {
	b, err := os.ReadFile("../../exam/tasks.json")
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
	if len(tasks) != 17 {
		t.Fatalf("got %d questions", len(tasks))
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
