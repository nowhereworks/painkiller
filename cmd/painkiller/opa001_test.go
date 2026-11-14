package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestOPA001Contract(t *testing.T) {
	exams, err := loadExams("../../exam")
	if err != nil {
		t.Fatal(err)
	}
	for _, exam := range exams {
		if exam.ID != "OPA-001" {
			continue
		}
		if exam.Lab == nil || exam.Lab.Controllers != 1 || exam.Lab.Workers != 0 ||
			exam.Lab.Template != "9199" || exam.Minutes != 150 || len(exam.tasks) != 16 {
			t.Fatalf("wrong OPA-001 contract: %+v", exam)
		}
		total := 0
		for i, task := range exam.tasks {
			total += task.Points
			if task.QuestionID != fmt.Sprintf("OPA-001-Q%d", i+1) || task.Explanation == "" {
				t.Fatalf("incomplete OPA task %d", i+1)
			}
		}
		b, err := os.ReadFile("../../exam/OPA-001/e2e.json")
		if err != nil {
			t.Fatal(err)
		}
		var fixture liveFixture
		if err := json.Unmarshal(b, &fixture); err != nil {
			t.Fatal(err)
		}
		if total != 100 || fixture.Baseline != 0 || len(fixture.Mutations) != 16 {
			t.Fatal("OPA-001 requires 100 points, zero baseline, and all 16 scoring mutations")
		}
		return
	}
	t.Fatal("OPA-001 missing from catalog")
}

func TestOPA001Capture(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	cmd := exec.Command("python3", "-B", "../../exam/OPA-001/tests/test_capture.py", "-v")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("OPA-001 capture tests: %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
