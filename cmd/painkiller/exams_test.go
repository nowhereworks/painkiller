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

func examFixture(t *testing.T) []examDefinition {
	t.Helper()
	root := t.TempDir()
	for _, id := range []string{"CKA-001", "CKA-002"} {
		dir := filepath.Join(root, id)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			"test.json": `{"id":"` + id + `","title":"` + id + `","description":"Fixture","minutes":60,"categories":["Fixture"]}`,
			"tasks.json": `[{"id":"` + id + `-task","question_id":"` + id + `-Q1","category":"Fixture","title":"Fixture","instructions":"Create a namespace",
				"solution":"kubectl create namespace fixture","points":100,"resource":"kubectl get namespace fixture -o json",
				"checks":[{"description":"Namespace exists","points":100,"expression":".metadata.name == \"fixture\""}]}]`,
			"setup.yaml":             "kind: List\nitems: []\n",
			"reference-solutions.sh": "#!/bin/sh\ntrue\n",
			"check-ready.sh":         "#!/bin/sh\ntrue\n",
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	exams, err := loadExams(root)
	if err != nil {
		t.Fatal(err)
	}
	return exams
}

func TestQuestionMetadata(t *testing.T) {
	exam := examFixture(t)[0]
	if err := validateQuestionMetadata(exam); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*examDefinition)
	}{
		{"missing ID", func(e *examDefinition) { e.tasks[0].QuestionID = "" }},
		{"wrong exam", func(e *examDefinition) { e.tasks[0].QuestionID = "CKA-002-Q1" }},
		{"invalid number", func(e *examDefinition) { e.tasks[0].QuestionID = e.ID + "-Q0" }},
		{"duplicate ID", func(e *examDefinition) { e.tasks = append(e.tasks, e.tasks[0]) }},
		{"missing category", func(e *examDefinition) { e.tasks[0].Category = "" }},
		{"unknown category", func(e *examDefinition) { e.tasks[0].Category = "Other" }},
		{"duplicate category", func(e *examDefinition) { e.Categories = append(e.Categories, e.Categories[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copyExam := exam
			copyExam.tasks = append([]task(nil), exam.tasks...)
			copyExam.Categories = append([]string(nil), exam.Categories...)
			tc.edit(&copyExam)
			if err := validateQuestionMetadata(copyExam); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestExamIDSuffix(t *testing.T) {
	root := t.TempDir()
	id := "CKS-001B"
	dir := filepath.Join(root, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"test.json": `{"id":"CKS-001B","title":"Fixture","minutes":60,"categories":["Fixture"]}`,
		"tasks.json": `[{"id":"task","question_id":"CKS-001B-Q1","category":"Fixture","title":"Fixture","instructions":"Create a namespace","solution":"kubectl create namespace fixture","points":100,"resource":"kubectl get namespace fixture -o json","checks":[{"description":"Namespace exists","points":100,"expression":".metadata.name == \"fixture\""}]}]`,
		"setup.yaml": "kind: List\nitems: []\n", "reference-solutions.sh": "#!/bin/sh\ntrue\n",
		"check-ready.sh": "#!/bin/sh\ntrue\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := loadExams(root); err != nil {
		t.Fatalf("suffixed exam should load: %v", err)
	}
	for _, invalid := range []string{"CKS-001b", "CKS-001BB", "CKS-001-B", "CKS-01B"} {
		invalidDir := filepath.Join(root, invalid)
		if err := os.Rename(dir, invalidDir); err != nil {
			t.Fatal(err)
		}
		dir = invalidDir
		if err := os.WriteFile(filepath.Join(dir, "test.json"), []byte(`{"id":"`+invalid+`","title":"Fixture","minutes":60}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadExams(root); err == nil {
			t.Fatalf("invalid ID %q was accepted", invalid)
		}
	}
}

func TestLiveExamWrapperIDSuffix(t *testing.T) {
	for _, tc := range []struct {
		id, want string
	}{
		{"ZZZ-999B", "Unknown exam:"},
		{"ZZZ-999BB", "Usage:"},
		{"ZZZ-999b", "Usage:"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			cmd := exec.Command("bash", "../../scripts/test-exam.sh", tc.id)
			cmd.Env = append(os.Environ(), "LAB_VM_START=3000")
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.want) {
				t.Fatalf("unexpected result: %s, %v", output, err)
			}
		})
	}
}

func TestQuestionMetadataInTaskList(t *testing.T) {
	a := testApp(t)
	a.tasks = examFixture(t)[0].tasks
	a.state.Phase = "completed"
	w := httptest.NewRecorder()
	a.taskList(w, httptest.NewRequest("GET", "/api/tasks", nil))
	var tasks []struct{ ID, QuestionID, Category string }
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil || len(tasks) != 1 {
		t.Fatalf("invalid tasks: %s %v", w.Body.String(), err)
	}
	if tasks[0].ID != "CKA-001-task" || tasks[0].QuestionID != "CKA-001-Q1" || tasks[0].Category != "Fixture" {
		t.Fatalf("incorrect metadata: %+v", tasks[0])
	}
}

func TestExamCatalogAndSelection(t *testing.T) {
	t.Setenv("PAINKILLER_MINUTES", "")
	a := testApp(t)
	a.exams = examFixture(t)
	w := httptest.NewRecorder()
	a.testList(w, httptest.NewRequest("GET", "/api/tests", nil))
	var public []examDefinition
	if err := json.Unmarshal(w.Body.Bytes(), &public); err != nil || len(public) != 2 {
		t.Fatalf("invalid catalog: %s %v", w.Body.String(), err)
	}
	for _, private := range []string{"solution", "resource", "checks", "dir", "tasks"} {
		if strings.Contains(w.Body.String(), `"`+private+`"`) {
			t.Fatalf("private field exposed: %s", private)
		}
	}
	for _, exam := range a.exams {
		if err := a.selectExam(exam.ID); err != nil {
			t.Fatal(err)
		}
		a.state.TestID = exam.ID
		if a.tasks[0].ID != exam.ID+"-task" || a.duration != time.Hour ||
			a.setupPath() != filepath.Join(exam.dir, "setup.yaml") {
			t.Fatalf("wrong test loaded: %+v", a.tasks)
		}
	}
	t.Setenv("PAINKILLER_MINUTES", "45")
	if err := a.selectExam("CKA-002"); err != nil || a.duration != 45*time.Minute {
		t.Fatalf("duration override: %v %v", a.duration, err)
	}
}

func TestPrepareRejectsInvalidTestSelection(t *testing.T) {
	a := testApp(t)
	a.exams = examFixture(t)
	for _, body := range []string{`{}`, `{"test_id":"CKA-999"}`, `{"test_id":"../CKA-001"}`, `{`, ``} {
		w := httptest.NewRecorder()
		a.action(w, httptest.NewRequest("POST", "/api/prepare", strings.NewReader(body)))
		if w.Code != 400 || a.state.Phase != "idle" || a.state.TestID != "" {
			t.Fatalf("accepted %q: %d %+v", body, w.Code, a.state)
		}
	}
	for _, phase := range []string{"preparing", "ready", "running", "grading", "completed", "error", "resetting"} {
		a.state = state{Phase: phase, TestID: "CKA-001", Deadline: time.Now().Add(time.Hour)}
		w := httptest.NewRecorder()
		a.action(w, httptest.NewRequest("POST", "/api/prepare", strings.NewReader(`{"test_id":"CKA-002"}`)))
		if w.Code != 409 || a.state.TestID != "CKA-001" {
			t.Fatalf("switched active test in %s: %d", phase, w.Code)
		}
	}
}

func TestPrepareSelectionRollsBackOnPersistenceFailure(t *testing.T) {
	a := testApp(t)
	a.exams = examFixture(t)
	if err := a.selectExam("CKA-001"); err != nil {
		t.Fatal(err)
	}
	a.db.Close()
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/prepare", strings.NewReader(`{"test_id":"CKA-002"}`)))
	if w.Code != 503 || a.state.Phase != "idle" || a.state.TestID != "" || a.tasks[0].ID != "CKA-001-task" {
		t.Fatalf("selection not rolled back: %d %+v", w.Code, a.state)
	}
}

func TestRestoreSelectedAndLegacyExams(t *testing.T) {
	for _, id := range []string{"", "CKA-002"} {
		a := testApp(t)
		a.exams = examFixture(t)
		a.state = state{Phase: "completed", TestID: id}
		migrated, err := a.restoreExam()
		want := id
		if want == "" {
			want = "CKA-001"
		}
		if err != nil || migrated != (id == "") || a.state.TestID != want || a.tasks[0].ID != want+"-task" {
			t.Fatalf("restore failed: migrated=%v err=%v state=%+v", migrated, err, a.state)
		}
		if err := a.save(); err != nil {
			t.Fatal(err)
		}
		var saved string
		if err := a.db.QueryRow("SELECT state FROM attempt WHERE id=1").Scan(&saved); err != nil {
			t.Fatal(err)
		}
		var restored state
		if err := json.Unmarshal([]byte(saved), &restored); err != nil || restored.TestID != want {
			t.Fatalf("test ID not persisted: %s %v", saved, err)
		}
	}
	a := testApp(t)
	a.exams = examFixture(t)
	a.state = state{Phase: "ready", TestID: "CKA-999"}
	if _, err := a.restoreExam(); err == nil {
		t.Fatal("missing saved test silently replaced")
	}
}

func TestExamDefinitionsFailClosed(t *testing.T) {
	for _, name := range []string{"test.json", "tasks.json", "setup.yaml", "reference-solutions.sh", "check-ready.sh"} {
		t.Run(name, func(t *testing.T) {
			exams := examFixture(t)
			if err := os.Remove(filepath.Join(exams[0].dir, name)); err != nil {
				t.Fatal(err)
			}
			if _, err := loadExams(filepath.Dir(exams[0].dir)); err == nil {
				t.Fatal("missing required file accepted")
			}
		})
	}
	exams := examFixture(t)
	if err := os.WriteFile(filepath.Join(exams[0].dir, "test.json"), []byte(`{"id":"CKA-002","title":"Mismatch","minutes":60}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadExams(filepath.Dir(exams[0].dir)); err == nil {
		t.Fatal("folder/ID mismatch accepted")
	}
}

func TestPackagedExams(t *testing.T) {
	exams, err := loadExams("../../exam")
	if err != nil {
		t.Fatal(err)
	}
	for _, exam := range exams {
		if exam.ID == "CKA-001" && len(exam.tasks) == 29 {
			return
		}
	}
	t.Fatal("original CKA-001 test missing or changed")
}

func TestSelectedExamGradingIsolation(t *testing.T) {
	a := testApp(t)
	a.exams = examFixture(t)
	if err := a.selectExam("CKA-002"); err != nil {
		t.Fatal(err)
	}
	a.state = state{Phase: "grading", TestID: "CKA-002", Answers: map[string]answerSnapshot{
		"CKA-001-task": {Data: json.RawMessage(`null`)},
		"CKA-002-task": {Data: json.RawMessage(`{"metadata":{"name":"fixture"}}`)},
	}}
	a.grade()
	if len(a.state.Results) != 1 || a.state.Results[0].ID != "CKA-002-task" ||
		a.state.Results[0].Earned != 100 || a.state.Results[0].Error != "" || a.state.TestID != "CKA-002" {
		t.Fatalf("wrong test graded: %+v", a.state)
	}
}

func TestPrepareBindsSelectedExamThroughReadyAndReset(t *testing.T) {
	a := testApp(t)
	a.exams = examFixture(t)
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("scripts", 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"scripts/create-lab.sh":            "#!/bin/sh\ncase \"$1\" in status) echo '{\"controller_ips\":\"192.0.2.1\"}';; esac\n",
		"scripts/configure-exam-access.sh": "#!/bin/sh\ntrue\n",
	} {
		if err := os.WriteFile(name, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	stubCommand(t, "ssh", "#!/bin/sh\ncat > setup-applied\n")
	a.file = filepath.Join(dir, "state.db")
	a.lockDir = dir
	setup := "# Selected CKA-002\nkind: List\nitems: []\n"
	if err := os.WriteFile(filepath.Join(a.exams[1].dir, "setup.yaml"), []byte(setup), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/prepare", strings.NewReader(`{"test_id":"CKA-002"}`)))
	if w.Code != 204 {
		t.Fatalf("prepare rejected: %d %s", w.Code, w.Body.String())
	}
	waitPhase(t, a, "ready")
	if a.state.TestID != "CKA-002" || a.tasks[0].ID != "CKA-002-task" {
		t.Fatalf("selected test lost: %+v", a.state)
	}
	applied, err := os.ReadFile("setup-applied")
	if err != nil || string(applied) != setup {
		t.Fatalf("wrong setup applied: %q %v", applied, err)
	}
	w = httptest.NewRecorder()
	a.action(w, httptest.NewRequest("POST", "/api/reset", nil))
	if w.Code != 204 {
		t.Fatalf("reset rejected: %d", w.Code)
	}
	waitPhase(t, a, "idle")
	if a.state.TestID != "" {
		t.Fatalf("reset kept test binding: %+v", a.state)
	}
}
