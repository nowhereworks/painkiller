package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type liveFixture struct {
	Baseline  int `json:"baseline"`
	Mutations []struct {
		Task, Remove, Restore string
	} `json:"mutations"`
}

func TestLiveExam(t *testing.T) {
	id := os.Getenv("PAINKILLER_LIVE_EXAM")
	if id == "" {
		t.Skip("live exam validation is opt-in; use scripts/test-exam.sh")
	}
	dir := os.Getenv("PAINKILLER_LIVE_DIR")
	if !filepath.IsAbs(dir) || os.Getenv("LAB_MARKER") == "" || os.Getenv("LAB_VM_START") == "" || os.Getenv("LAB_KNOWN_HOSTS") != filepath.Join(dir, "known_hosts") {
		t.Fatal("use scripts/test-exam.sh with an explicit disposable VM range")
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	exams, err := loadExams("exam")
	if err != nil {
		t.Fatal(err)
	}
	var selected *examDefinition
	for i := range exams {
		if exams[i].ID == id {
			selected = &exams[i]
		}
	}
	if selected == nil {
		t.Fatalf("unknown exam %q", id)
	}
	b, err := os.ReadFile(filepath.Join(selected.dir, "e2e.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture liveFixture
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Mutations) == 0 {
		t.Fatal("exam must define at least one scoring mutation")
	}
	tasks := make(map[string]task)
	for _, task := range selected.tasks {
		tasks[task.ID] = task
	}
	for _, m := range fixture.Mutations {
		if _, ok := tasks[m.Task]; !ok || m.Remove == "" || m.Restore == "" {
			t.Fatalf("invalid mutation for task %q", m.Task)
		}
	}
	config, err := resolveLab(*selected)
	if err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	a.file = filepath.Join(dir, "state.db")
	a.lockDir = "data"
	a.exams = exams
	a.tasks = selected.tasks
	a.state = state{Phase: "preparing", TestID: id, Lab: config}
	t.Logf("Lab VM range %s–%s; marker %s; artifacts %s", config.Env["LAB_VM_START"], config.Env["LAB_VM_END"], config.Env["LAB_MARKER"], dir)
	t.Log("On failure, inspect operation.log and VM ownership before manual cleanup; no automatic cleanup is attempted.")
	a.prepare()
	if a.state.Phase != "ready" {
		t.Fatalf("preparation failed: %s; see %s", a.state.Message, filepath.Join(dir, "operation.log"))
	}
	ssh := func(command string, input []byte) {
		t.Helper()
		args := append(a.labSSHArgs(), a.labValue("LAB_SSH_USER", "milkman")+"@"+a.state.Host, command)
		cmd := exec.Command("timeout", append([]string{"600s", "ssh"}, args...)...)
		cmd.Stdin = strings.NewReader(string(input))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("student command failed: %v\n%s", err, output)
		}
	}
	// Production grading reuses successful snapshots; each checkpoint must start fresh.
	score := func() map[string]int {
		t.Helper()
		a.state.Answers = nil
		a.state.Results = nil
		a.grade()
		if a.state.Phase != "completed" {
			t.Fatalf("grading failed: %s", a.state.Message)
		}
		scores := make(map[string]int)
		for _, result := range a.state.Results {
			if result.Error != "" {
				t.Fatalf("grading infrastructure error for %s: %s", result.ID, result.Error)
			}
			scores[result.ID] = result.Earned
		}
		return scores
	}
	total := func(scores map[string]int) int {
		n := 0
		for _, points := range scores {
			n += points
		}
		return n
	}
	deadline := time.Now().Add(60 * time.Second)
	stable := 0
	for stable < 2 {
		if total(score()) == fixture.Baseline {
			stable++
		} else {
			stable = 0
		}
		if stable < 2 {
			if time.Now().After(deadline) {
				t.Fatalf("baseline did not stabilize at %d", fixture.Baseline)
			}
			time.Sleep(time.Second)
		}
	}
	t.Logf("Baseline: %d", fixture.Baseline)
	solution, err := os.ReadFile(filepath.Join(selected.dir, "reference-solutions.sh"))
	if err != nil {
		t.Fatal(err)
	}
	ssh("bash -s", solution)
	full := score()
	if total(full) != 100 {
		t.Fatalf("reference score %d, expected 100: %v", total(full), full)
	}
	t.Log("Reference solutions: 100/100")
	for _, m := range fixture.Mutations {
		ssh("bash -o pipefail -c "+shellQuote(m.Remove), nil)
		got := score()
		for taskID, want := range full {
			if taskID == m.Task {
				want -= tasks[taskID].Points
			}
			if got[taskID] != want {
				t.Fatalf("mutation %s: task %s scored %d, expected %d", m.Task, taskID, got[taskID], want)
			}
		}
		ssh("bash -o pipefail -c "+shellQuote(m.Restore), nil)
		if restored := score(); total(restored) != 100 {
			t.Fatalf("restoring %s scored %d: %v", m.Task, total(restored), restored)
		}
		t.Logf("Mutation %s: lost %d points and restored", m.Task, tasks[m.Task].Points)
	}
	a.reset()
	if a.state.Phase != "idle" {
		t.Fatalf("cleanup failed: %s; inspect the marked VMs", a.state.Message)
	}
	t.Log("Lab destroyed successfully")
}
