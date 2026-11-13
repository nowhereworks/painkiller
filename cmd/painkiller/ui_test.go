package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAttemptDurationPersistsAcrossConfigurationChanges(t *testing.T) {
	a := testApp(t)
	a.duration = 90 * time.Minute
	a.state.Phase = "ready"
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/start", nil))
	if w.Code != 204 || a.state.DurationSeconds != 5400 {
		t.Fatalf("unexpected duration: %d %+v", w.Code, a.state)
	}
	var data []byte
	if err := a.db.QueryRow(`SELECT state FROM attempt WHERE id=1`).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var saved state
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	a.state = saved
	a.duration = 30 * time.Minute
	w = httptest.NewRecorder()
	a.status(w, httptest.NewRequest("GET", "/api/status", nil))
	var public state
	if err := json.Unmarshal(w.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	if public.DurationSeconds != 5400 || !public.Deadline.Equal(saved.Deadline) {
		t.Fatalf("persisted timing changed: %+v", public)
	}
}

func TestLegacyAttemptDurationFallback(t *testing.T) {
	for _, duration := range []time.Duration{0, 45 * time.Minute} {
		a := testApp(t)
		a.duration = duration
		a.state = state{Phase: "running", Deadline: time.Now().Add(time.Minute)}
		w := httptest.NewRecorder()
		a.status(w, httptest.NewRequest("GET", "/api/status", nil))
		var public state
		if err := json.Unmarshal(w.Body.Bytes(), &public); err != nil {
			t.Fatal(err)
		}
		want := int64(duration / time.Second)
		if want == 0 {
			want = 7200
		}
		if public.DurationSeconds != want || a.state.DurationSeconds != 0 {
			t.Fatalf("unexpected legacy fallback: public=%d stored=%d", public.DurationSeconds, a.state.DurationSeconds)
		}
	}
}
