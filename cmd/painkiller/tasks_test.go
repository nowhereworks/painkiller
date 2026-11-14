package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNodeReferences(t *testing.T) {
	nodes := map[string]labNode{
		"controller3": {IP: "192.0.2.3", Hostname: "k8s-3002"},
		"worker2":     {IP: "192.0.2.5"},
	}
	got := renderNodeReferences("Join {{controller3}}; repair {{worker2}}.", nodes)
	for _, want := range []string{"`k8s-3002`", "`192.0.2.3`", "`ssh 192.0.2.3`", "`ssh 192.0.2.5`"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	fallback := renderNodeReferences("{{controller3}} {{worker2}}", nil)
	if strings.Contains(fallback, "{{") || !strings.Contains(fallback, "painkiller_known_hosts") || !strings.Contains(fallback, "zone=east") {
		t.Fatalf("bad legacy fallback: %s", fallback)
	}
	if strings.Contains(renderNodeReferences("{{worker2}}", map[string]labNode{"worker2": {IP: "bad;command"}}), "bad;command") {
		t.Fatal("invalid address rendered")
	}
}

func TestTaskListRendersNodeReferences(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Deadline: time.Now().Add(time.Hour), Nodes: map[string]labNode{
		"controller3": {IP: "192.0.2.3", Hostname: "k8s-3002"},
	}}
	a.tasks = []task{{ID: "join", Instructions: "Connect to {{controller3}}", Steps: []string{"Use {{controller3}}"}}}
	w := httptest.NewRecorder()
	a.taskList(w, httptest.NewRequest("GET", "/api/tasks", nil))
	var tasks []struct {
		Instructions string
		Steps        []string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || len(tasks[0].Steps) != 1 || !strings.Contains(tasks[0].Instructions, "`ssh 192.0.2.3`") || !strings.Contains(tasks[0].Steps[0], "`k8s-3002`") {
		t.Fatalf("node references not rendered: %+v", tasks)
	}
}

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
