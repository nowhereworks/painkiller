package main

import (
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
	return &app{file: filepath.Join(t.TempDir(), "state.json"), state: state{Phase: "idle"}, terminals: map[*os.File]*exec.Cmd{}}
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
	b, err := os.ReadFile(a.file)
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
