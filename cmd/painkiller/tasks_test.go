package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestTaskListWalkthroughs(t *testing.T) {
	for _, phase := range []string{"idle", "ready", "running", "completed"} {
		t.Run(phase, func(t *testing.T) {
			a := testApp(t)
			a.state = state{Phase: phase, Deadline: time.Now().Add(time.Hour)}
			a.tasks = []task{{ID: "one", Title: "Example", Instructions: "Question", Solution: "kubectl get pods", Explanation: "Explain", Steps: []string{"Run", "Verify"}, Resource: "private capture", Checks: []check{{Expression: "private check"}}}}
			w := httptest.NewRecorder()
			a.taskList(w, httptest.NewRequest("GET", "/api/tasks", nil))
			var tasks []map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
				t.Fatal(err)
			}
			if phase != "running" && phase != "completed" {
				if len(tasks) != 0 {
					t.Fatal("tasks exposed before exam start")
				}
				return
			}
			if len(tasks) != 1 {
				t.Fatalf("got %d tasks", len(tasks))
			}
			for _, key := range []string{"Solution", "Explanation", "Steps"} {
				if tasks[0][key] == nil {
					t.Errorf("missing %s", key)
				}
			}
			for _, key := range []string{"Resource", "Check", "Checks"} {
				if tasks[0][key] != nil {
					t.Errorf("exposed internal %s", key)
				}
			}
		})
	}
}

func TestPackagedWalkthroughs(t *testing.T) {
	b, err := os.ReadFile("../../exam/CKA-001/tasks.json")
	if err != nil {
		t.Fatal(err)
	}
	var tasks []task
	if err := json.Unmarshal(b, &tasks); err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if len(task.Steps) < 2 {
			t.Errorf("task %s lacks a step-by-step walkthrough", task.ID)
		}
		for _, step := range task.Steps {
			if step == "" {
				t.Errorf("task %s has an empty step", task.ID)
			}
		}
	}
}
