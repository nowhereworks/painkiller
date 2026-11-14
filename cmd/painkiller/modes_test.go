package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLearningOvertimeRemainsInteractive(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Mode: "learning", Deadline: time.Now().Add(-time.Minute)}
	a.tasks = []task{{ID: "one"}}
	a.expire()
	for _, action := range []struct{ path, body string }{
		{"select", `{"id":"one"}`},
		{"mark", `{"id":"one","done":true}`},
	} {
		w := httptest.NewRecorder()
		a.action(w, httptest.NewRequest("POST", "/api/"+action.path, strings.NewReader(action.body)))
		if w.Code != 204 {
			t.Fatalf("%s: %d %s", action.path, w.Code, w.Body.String())
		}
	}
	if !a.running() || a.state.Selected != "one" || !a.state.Done["one"] {
		t.Fatalf("learning attempt changed unexpectedly: %+v", a.state)
	}
}

func TestVerifyFreshAndDoesNotChangeAttempt(t *testing.T) {
	stubCommand(t, "ssh", "#!/bin/sh\ncat \"$VERIFY_INPUT\"\n")
	file := t.TempDir() + "/answer.json"
	t.Setenv("VERIFY_INPUT", file)
	a := testApp(t)
	a.state = state{Phase: "running", Mode: "learning", Host: "host", Deadline: time.Now().Add(-time.Minute), Selected: "other"}
	a.tasks = []task{{ID: "one", Resource: "read-only", Points: 1, Checks: []check{{Description: "Exists", Points: 1, Expression: ".exists"}}}}
	for _, tc := range []struct {
		data string
		want int
	}{{`{"exists":false}`, 0}, {`{"exists":true}`, 1}} {
		if err := os.WriteFile(file, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		a.action(w, httptest.NewRequest("POST", "/api/verify", strings.NewReader(`{"id":"one"}`)))
		var got result
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Earned != tc.want {
			t.Fatalf("verify: %d %s", w.Code, w.Body.String())
		}
	}
	if a.state.Answers != nil || a.state.Results != nil || a.state.Selected != "other" || a.state.Phase != "running" {
		t.Fatalf("verification mutated attempt: %+v", a.state)
	}
}

func TestVerifyRejectsSimulationAndUnknownQuestion(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Mode: "simulation", Deadline: time.Now().Add(time.Hour)}
	for _, tc := range []struct {
		mode string
		want int
	}{{"simulation", 409}, {"learning", 400}} {
		a.state.Mode = tc.mode
		w := httptest.NewRecorder()
		a.action(w, httptest.NewRequest("POST", "/api/verify", strings.NewReader(`{"id":"unknown"}`)))
		if w.Code != tc.want {
			t.Fatalf("%s: got %d", tc.mode, w.Code)
		}
	}
}

func TestLearningModePersistsAndCanSubmitOvertime(t *testing.T) {
	a := testApp(t)
	a.state = state{Phase: "running", Mode: "learning", Deadline: time.Now().Add(-time.Minute)}
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := a.db.QueryRow("SELECT state FROM attempt WHERE id=1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var restored state
	if err := json.Unmarshal(raw, &restored); err != nil || restored.Mode != "learning" {
		t.Fatalf("mode not persisted: %+v %v", restored, err)
	}
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/submit", strings.NewReader("{}")))
	if w.Code != 204 {
		t.Fatalf("overtime submission: %d %s", w.Code, w.Body.String())
	}
	waitPhase(t, a, "completed")
}

func TestVerifyCaptureErrorIsNotAnIncorrectAnswer(t *testing.T) {
	stubCommand(t, "ssh", "#!/bin/sh\nexit 255\n")
	a := testApp(t)
	a.state = state{Phase: "running", Mode: "learning", Deadline: time.Now().Add(time.Hour)}
	a.tasks = []task{{ID: "one", Resource: "read-only", Points: 1}}
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/verify", strings.NewReader(`{"id":"one"}`)))
	var got result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Error == "" || len(got.Checks) != 0 {
		t.Fatalf("capture error: %d %s", w.Code, w.Body.String())
	}
}
