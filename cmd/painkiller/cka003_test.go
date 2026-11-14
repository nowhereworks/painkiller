package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCKA003Definition(t *testing.T) {
	exams, err := loadExams("../../exam")
	if err != nil {
		t.Fatal(err)
	}
	for _, exam := range exams {
		if exam.ID != "CKA-003" {
			continue
		}
		if len(exam.tasks) != 16 {
			t.Fatalf("got %d questions, want 16", len(exam.tasks))
		}
		weights := []int{6, 8, 8, 8, 6, 6, 8, 4, 8, 8, 4, 8, 8, 4, 4, 2}
		ids := make(map[string]bool)
		for i, task := range exam.tasks {
			if task.QuestionID != fmt.Sprintf("CKA-003-Q%d", i+1) || task.Points != weights[i] {
				t.Fatalf("source order or weight changed at question %d", i+1)
			}
			if task.Resource != fmt.Sprintf("bash /opt/painkiller/cka003/capture.sh q%d", i+1) {
				t.Fatalf("unexpected capture for %s", task.ID)
			}
			ids[task.ID] = true
			cmd := exec.Command("bash", "-n")
			cmd.Stdin = strings.NewReader(task.Solution)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("invalid solution %s: %v\n%s", task.ID, err, out)
			}
		}
		data, err := os.ReadFile("../../exam/CKA-003/e2e.json")
		if err != nil {
			t.Fatal(err)
		}
		var fixture liveFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatal(err)
		}
		if fixture.Baseline != 0 || len(fixture.Mutations) != 16 {
			t.Fatal("expected zero baseline and one mutation per question")
		}
		for _, m := range fixture.Mutations {
			if !ids[m.Task] || m.Remove == "" || m.Restore == "" {
				t.Fatalf("invalid or duplicate mutation for %s", m.Task)
			}
			delete(ids, m.Task)
			for _, command := range []string{m.Remove, m.Restore} {
				cmd := exec.Command("bash", "-n")
				cmd.Stdin = strings.NewReader(command)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("invalid mutation %s: %v\n%s", m.Task, err, out)
				}
			}
		}
		return
	}
	t.Fatal("CKA-003 missing from catalog")
}

// No cluster required: an absent answer is a valid failed capture, not an error.
// A broken API must instead fail capture, never silently become an absent answer.
func TestCKA003MissingAnswers(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq required for exam capture checks")
	}
	if err := exec.Command("python3", "-c", "import yaml").Run(); err != nil {
		t.Skip("Python YAML support required for chart capture checks")
	}
	dir := t.TempDir()
	stubs := map[string]string{
		"kubectl": "#!/bin/sh\ncase \"$*\" in\n*exec*) echo 'expected connection failure' >&2; exit 1;;\n*'auth can-i'*) echo 'Warning: resource is not namespace scoped' >&2; echo no; exit 1;;\n*jsonpath*) echo controller;;\n*) echo '{}';;\nesac\n",
		"helm":    "#!/bin/sh\necho '[]'\n",
		"curl":    "#!/bin/sh\necho 'expected HTTP probe failure' >&2\nexit 7\n",
		"sudo":    "#!/bin/sh\nexit 1\n",
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	for i := 1; i <= 16; i++ {
		t.Run(fmt.Sprintf("q%d", i), func(t *testing.T) {
			cmd := exec.Command("bash", "../../exam/CKA-003/assets/capture.sh", fmt.Sprintf("q%d", i))
			data, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("missing answer failed capture: %v", err)
			}
			var result struct{ OK bool }
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result.OK {
				t.Fatal("missing answer passed")
			}
		})
	}
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("bash", "../../exam/CKA-003/assets/capture.sh", "q1").Run(); err == nil {
		t.Fatal("API failure silently became an absent answer")
	}
}
