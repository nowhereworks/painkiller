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

func TestCKAD001Definition(t *testing.T) {
	exams, err := loadExams("../../exam")
	if err != nil {
		t.Fatal(err)
	}
	for _, exam := range exams {
		if exam.ID != "CKAD-001" {
			continue
		}
		weights := []int{8, 2, 8, 1, 8, 2, 8, 1, 8, 8, 4, 4, 8, 4, 2, 8, 2, 4, 2, 4, 4}
		if len(exam.tasks) != len(weights) || exam.Minutes != 120 {
			t.Fatal("expected 21 questions and 120 minutes")
		}
		if exam.Lab.Controllers != 1 || exam.Lab.Workers != 2 || exam.Lab.CNI != "calico" || exam.Lab.Template != "9199" {
			t.Fatal("expected single-cluster base-image lab with enforced NetworkPolicies")
		}
		ids := map[string]bool{}
		total := 0
		for i, task := range exam.tasks {
			if task.QuestionID != fmt.Sprintf("CKAD-001-Q%d", i+1) || task.Points != weights[i] {
				t.Fatalf("source order/weight changed at Q%d", i+1)
			}
			if task.Resource != fmt.Sprintf("bash /opt/painkiller/ckad001/capture.sh q%d", i+1) {
				t.Fatalf("unexpected capture: %s", task.Resource)
			}
			if len(task.Checks) != 1 || task.Checks[0].Diagnostics != ".failures // []" {
				t.Fatalf("%s must expose capture diagnostics without splitting its score", task.QuestionID)
			}
			if strings.Contains(task.Solution, "exam/CKA-") {
				t.Fatal("cross-exam solution dependency")
			}
			cmd := exec.Command("bash", "-n")
			cmd.Stdin = strings.NewReader(task.Solution)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Q%d solution syntax: %v\n%s", i+1, err, out)
			}
			total += task.Points
			ids[task.ID] = true
		}
		if total != 100 {
			t.Fatalf("total points: %d", total)
		}
		b, err := os.ReadFile("../../exam/CKAD-001/e2e.json")
		if err != nil {
			t.Fatal(err)
		}
		var fixture liveFixture
		if err := json.Unmarshal(b, &fixture); err != nil {
			t.Fatal(err)
		}
		if fixture.Baseline != 0 || len(fixture.Mutations) != 21 {
			t.Fatal("expected zero baseline and one mutation per question")
		}
		for i, m := range fixture.Mutations {
			if !ids[m.Task] || m.Remove != fmt.Sprintf("bash /opt/painkiller/ckad001/mutate.sh remove q%d", i+1) ||
				m.Restore != fmt.Sprintf("bash /opt/painkiller/ckad001/mutate.sh restore q%d", i+1) {
				t.Fatalf("invalid/duplicate mutation: %s", m.Task)
			}
			delete(ids, m.Task)
		}
		return
	}
	t.Fatal("CKAD-001 missing")
}

func TestCKAD001MissingAnswers(t *testing.T) {
	for _, tool := range []string{"bash", "jq", "python3", "timeout"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable", tool)
		}
	}
	dir := t.TempDir()
	scripts := map[string]string{
		"kubectl": "#!/bin/bash\nif [[ $* == *jsonpath* ]]; then echo 192.0.2.10; else echo '{}'; fi\n",
		"helm":    "#!/bin/bash\necho '[]'\n",
		"sudo":    "#!/bin/bash\nexit 1\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for q := 1; q <= 21; q++ {
		cmd := exec.Command("bash", "../../exam/CKAD-001/assets/capture.sh", fmt.Sprintf("q%d", q))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Q%d capture failed instead of returning a missing answer: %v\n%s", q, err, out)
		}
		var result struct {
			OK       bool
			Failures []string
		}
		if err := json.Unmarshal(out, &result); err != nil || result.OK {
			t.Fatalf("Q%d should return false JSON, got %s (%v)", q, out, err)
		}
		if len(result.Failures) == 0 {
			t.Fatalf("Q%d missing answer needs actionable diagnostics: %s", q, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte("#!/bin/bash\nexit 42\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for q := 1; q <= 21; q++ {
		cmd := exec.Command("bash", "../../exam/CKAD-001/assets/capture.sh", fmt.Sprintf("q%d", q))
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("Q%d concealed API failure: %s", q, out)
		}
	}
}

func TestCKAD001QoSEvidence(t *testing.T) {
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable", tool)
		}
	}
	dir := t.TempDir()
	for name, script := range map[string]string{
		"kubectl": "#!/bin/bash\nif [[ $* == *'get pods '* ]]; then echo \"$CKAD001_PODS\"; else echo \"$CKAD001_POD\"; fi\n",
		"sudo":    "#!/bin/bash\nif [[ $* == *' cat '* ]]; then echo \"$CKAD001_REPORT\"; fi\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	pod := `{"metadata":{"name":"elastic-web"},"spec":{"containers":[{"name":"elastic-container","image":"nginx:1.27"}]},"status":{"phase":"Running","qosClass":"Burstable","conditions":[{"type":"Ready","status":"True"}]}}`
	t.Setenv("CKAD001_POD", pod)
	t.Setenv("CKAD001_PODS", `{"items":[`+pod+`,{"metadata":{"name":"other"},"status":{"qosClass":"BestEffort"}}]}`)
	for _, tc := range []struct {
		name, report string
		present, ok  bool
	}{
		{"complete, reordered", "NAME    QOS\nother BestEffort\nelastic-web   Burstable\n", true, true},
		{"missing row", "NAME QOS\nelastic-web Burstable", true, false},
		{"wrong class", "NAME QOS\nother Guaranteed\nelastic-web Burstable", true, false},
		{"extra row", "NAME QOS\nother BestEffort\nelastic-web Burstable\nextra BestEffort", true, false},
		{"missing Pod, matching report", "NAME QOS\nother BestEffort\nelastic-web Burstable", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CKAD001_REPORT", tc.report)
			if tc.present {
				t.Setenv("CKAD001_POD", pod)
			} else {
				t.Setenv("CKAD001_POD", "{}")
			}
			out, err := exec.Command("bash", "../../exam/CKAD-001/assets/capture.sh", "q13").CombinedOutput()
			if err != nil {
				t.Fatalf("capture: %v\n%s", err, out)
			}
			var result struct{ OK bool }
			if err := json.Unmarshal(out, &result); err != nil || result.OK != tc.ok {
				t.Fatalf("want %t, got %s (%v)", tc.ok, out, err)
			}
		})
	}
}

func TestCKAD001ThreeContainerEvidence(t *testing.T) {
	stubCommand(t, "kubectl", "#!/bin/sh\nprintf '%s' \"$CKAD001_POD\"\n")
	const pod = `{"metadata":{"name":"trio-runner"},"spec":{"containers":[
		{"name":"alpha","image":"busybox:1.28","env":[{"name":"ORDER","value":"LEAD"}]},
		{"name":"web","image":"nginx:1.27","ports":[{"containerPort":8081}]},
		{"name":"omega","image":"busybox:1.31.1","env":[{"name":"ORDER","value":"TAIL"}]}
	]},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}],
	"containerStatuses":[{"ready":true,"state":{"running":{}}},{"ready":true,"state":{"running":{}}},{"ready":true,"state":{"running":{}}}]}}`
	for _, tc := range []struct {
		name, data, failure string
	}{
		{"correct", pod, ""},
		{"wrong image", strings.Replace(pod, "busybox:1.28", "busybox:latest", 1), "Container alpha must use busybox:1.28"},
		{"wrong environment", strings.Replace(pod, "TAIL", "LEAD", 1), "Container omega must set ORDER=TAIL"},
		{"wrong port", strings.Replace(pod, "8081", "80", 1), "Container web must declare containerPort 8081"},
		{"wrong name", strings.Replace(pod, `"web"`, `"other"`, 1), "Container web must use nginx:1.27"},
		{"extra container", strings.Replace(pod, `"containers":[`, `"containers":[{"name":"extra"},`, 1), "Pod must contain exactly three containers"},
		{"starting", strings.Replace(pod, `"Running"`, `"Pending"`, 1), "Pod must be Running and Ready"},
		{"not ready", strings.ReplaceAll(pod, `"ready":true`, `"ready":false`), "All three containers must be running and ready"},
		{"missing", "{}", "Pod container-lab/trio-runner must exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CKAD001_POD", tc.data)
			out, err := exec.Command("bash", "../../exam/CKAD-001/assets/capture.sh", "q1").CombinedOutput()
			if err != nil {
				t.Fatalf("capture: %v\n%s", err, out)
			}
			var evidence struct {
				OK       bool
				Failures []string
			}
			if err := json.Unmarshal(out, &evidence); err != nil || evidence.OK != (tc.failure == "") {
				t.Fatalf("unexpected evidence: %s (%v)", out, err)
			}
			if tc.failure != "" && !strings.Contains(string(out), tc.failure) {
				t.Fatalf("missing failure %q: %s", tc.failure, out)
			}
			a := testApp(t)
			task := task{ID: "q1", Resource: "capture", Points: 8, Checks: []check{{
				Description: "Three correctly configured containers are running",
				Points:      8, Expression: ".ok == true", Diagnostics: ".failures // []",
			}}}
			result := a.evaluateTask("", "", task, answerSnapshot{Data: out})
			if result.Passed != evidence.OK || result.Error != "" {
				t.Fatalf("unexpected grading: %+v", result)
			}
			if len(result.Checks[0].Details) != len(evidence.Failures) {
				t.Fatalf("lost diagnostics: %+v", result)
			}
		})
	}
}

func TestCKAD001CloudInitWait(t *testing.T) {
	for _, tool := range []string{"bash", "python3", "timeout"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable", tool)
		}
	}
	dir := t.TempDir()
	for name, script := range map[string]string{
		"cloud-init": "#!/bin/bash\nif [[ $* == *json* ]]; then echo \"$CKAD001_INIT_STATUS\"; fi\nexit \"$CKAD001_INIT_RC\"\n",
		"sudo":       "#!/bin/bash\nshift\nexec \"$@\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct {
		name, rc, status string
		ok               bool
	}{
		{"normal", "0", `{"status":"done","errors":[]}`, true},
		{"recoverable deprecation", "2", `{"status":"done","errors":[]}`, true},
		{"fatal exit", "1", `{"status":"done","errors":[]}`, false},
		{"unfinished", "2", `{"status":"running","errors":[]}`, false},
		{"errors", "2", `{"status":"done","errors":["failed module"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CKAD001_INIT_RC", tc.rc)
			t.Setenv("CKAD001_INIT_STATUS", tc.status)
			out, err := exec.Command("bash", "../../exam/CKAD-001/assets/wait-cloud-init.sh").CombinedOutput()
			if (err == nil) != tc.ok {
				t.Fatalf("want success %t: %v\n%s", tc.ok, err, out)
			}
		})
	}
}

func TestCKAD001MutationCommands(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq unavailable")
	}
	dir := t.TempDir()
	for name, script := range map[string]string{
		"kubectl": `#!/bin/bash
set -e
if [[ $* == *"get namespace"* ]]; then
  echo '{"metadata":{"labels":{"painkiller-shell/exam":"CKAD-001"}}}'
fi
while (( $# )); do
  if [[ $1 == -p ]]; then
    shift
    printf '%s' "$1" | jq -e 'type == "object"' >/dev/null
    printf '%s' "$1" > "$CKAD001_PATCH"
  fi
  shift
done
`,
		"bash": "#!/bin/bash\necho '{\"ok\":true}'\n",
		"sudo": "#!/bin/bash\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMPDIR", dir)
	patch := filepath.Join(dir, "patch.json")
	t.Setenv("CKAD001_PATCH", patch)
	for q := 1; q <= 21; q++ {
		for _, action := range []string{"remove", "restore"} {
			out, err := exec.Command(bash, "../../exam/CKAD-001/assets/mutate.sh", action, fmt.Sprintf("q%d", q)).CombinedOutput()
			if err != nil {
				t.Fatalf("Q%d %s: %v\n%s", q, action, err, out)
			}
			if q == 16 {
				b, err := os.ReadFile(patch)
				if err != nil {
					t.Fatal(err)
				}
				var value struct{ Data map[string][]byte }
				if err := json.Unmarshal(b, &value); err != nil {
					t.Fatal(err)
				}
				want := "wrong"
				if action == "restore" {
					want = "ledger"
				}
				if string(value.Data["DB_User"]) != want {
					t.Fatalf("Secret mutation must not add a trailing newline: %s", b)
				}
			}
		}
	}
}
