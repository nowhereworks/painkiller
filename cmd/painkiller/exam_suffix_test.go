package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuffixedExamCatalog(t *testing.T) {
	exams, err := loadExams("../../exam")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, exam := range exams {
		if exam.ID == "CKS-001B" {
			found = true
			if len(exam.tasks) != 16 || exam.Lab.Controllers != 1 || exam.Lab.Workers != 0 {
				t.Fatal("unexpected CKS-001B contract")
			}
		}
	}
	if !found {
		t.Fatal("CKS-001B missing from catalog")
	}
}

func TestSuffixedExamValidation(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"CKS-001b", "CKS-001BB", "CKS-001-B", "CKS-01B"} {
		dir := filepath.Join(root, id)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "test.json"), []byte(
			`{"id":"`+id+`","title":"Invalid","minutes":120}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadExams(root); err == nil || !strings.Contains(err.Error(), "invalid test metadata") {
			t.Fatalf("%s: expected invalid metadata, got %v", id, err)
		}
		// Only remove this test's own temporary fixture.
		if err := os.Remove(filepath.Join(dir, "test.json")); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(dir); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSuffixedExamWrapper(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"ZZZ-999B", "Unknown exam:"}, {"ZZZ-999BB", "Usage:"}, {"ZZZ-999b", "Usage:"},
	} {
		cmd := exec.Command("bash", "../../scripts/test-exam.sh", tc.id)
		cmd.Env = append(os.Environ(), "LAB_VM_START=3000")
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), tc.want) {
			t.Fatalf("%s: %v: %s", tc.id, err, out)
		}
	}
}
