package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real capture scripts and production grader without a cluster.
func TestVerificationDiagnosticsAndProbeErrors(t *testing.T) {
	stubCommand(t, "kubectl", `#!/bin/bash
if [[ $* == *exec* ]]; then
  [[ ${PROBE_MODE:-} != transport-error && ${PROBE_MODE:-} != unready ]] || { echo 'Error from server (NotFound): probe Pod not found' >&2; exit 42; }
  while [[ $1 != -- ]]; do shift; done
  shift
  exec "$@"
fi
printf '%s\n' "$CAPTURE_OBJECT"
`)
	stubCommand(t, "wget", "#!/bin/sh\n[ \"$PROBE_MODE\" != mismatch ] || exit 1\nprintf '%s' 'Welcome to nginx'\n")
	stubCommand(t, "curl", "#!/bin/sh\n[ \"$PROBE_MODE\" != mismatch ] || exit 7\nprintf '%s' 'Welcome to nginx'\n")
	stubCommand(t, "cat", "#!/bin/sh\n[ \"$PROBE_MODE\" != mismatch ] || exit 1\nprintf '%s' 'workshop-token-42'\n")
	stubCommand(t, "printenv", "#!/bin/sh\n[ \"$PROBE_MODE\" != mismatch ] || exit 1\nprintf 'sql02\\nledger\\npractice456\\n'\n")
	stubCommand(t, "sudo", "#!/bin/sh\nexit 1\n")
	stubCommand(t, "ssh", "#!/bin/sh\nexec bash -c \"$CAPTURE_COMMAND\"\n")
	const ready = `"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}`
	cases := []struct {
		exam, question, object string
		probe                  bool
	}{
		{"CKAD-001", "q2", `{"provisioner":"kubernetes.io/no-provisioner","volumeBindingMode":"WaitForFirstConsumer","allowVolumeExpansion":true}`, false},
		{"CKAD-001", "q4", `{"spec":{"containers":[{"name":"ubuntu-agent","image":"ubuntu:24.04","command":["sleep","7200"]}]},` + ready + `}`, false},
		{"CKAD-001", "q8", `{"metadata":{"labels":{"app":"test"}},"spec":{"type":"ClusterIP","selector":{"app":"test"},"ports":[{"port":6445,"targetPort":80,"protocol":"TCP"}]}}`, true},
		{"CKAD-001", "q12", `{"spec":{"type":"ExternalName","externalName":"dns.google","ports":[{"port":53,"protocol":"UDP"}]}}`, false},
		{"CKAD-001", "q15", `{"data":{"APP_MODE":"practice","APP_REGION":"harbor","CACHE_HOST":"cache02","CACHE_TTL":"180"}}`, false},
		{"CKAD-001", "q16", `{"spec":{"containers":[{"name":"client","image":"busybox:1.36","command":["sh","-c","test \"$DB_Host\" = sql02 && test \"$DB_User\" = ledger && test \"$DB_Password\" = practice456 && exec sleep 86400"],"envFrom":[{"secretRef":{"name":"ledger-settings"}}]}]},"data":{"DB_Host":"c3FsMDI=","DB_User":"bGVkZ2Vy","DB_Password":"cHJhY3RpY2U0NTY="},` + ready + `}`, true},
		{"CKAD-001", "q17", `{"metadata":{"name":"quota-object-cap"},"spec":{"hard":{"resourcequotas":"1"}}}`, false},
		{"CKAD-001", "q18", `{"spec":{"volumes":[{"name":"credential-volume","secret":{"secretName":"vault-settings"}}],"containers":[{"name":"reader","image":"busybox:1.36","command":["sh","-c","sleep 86400"],"volumeMounts":[{"name":"credential-volume","mountPath":"/etc/credentials","readOnly":true}]}]},` + ready + `}`, true},
		{"CKAD-001", "q21", `{"spec":{"containers":[{"image":"redis:7.4","livenessProbe":{"exec":{"command":["ls"]},"initialDelaySeconds":8}}]},` + ready + `}`, false},
		{"CKA-003", "q1", `{"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"quay-frontend"},"minReplicas":3,"maxReplicas":9,"metrics":[{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":75}}}]}}`, false},
		{"CKA-003", "q3", `{"metadata":{"generation":2},"spec":{"replicas":3},"status":{"replicas":3,"observedGeneration":2,"updatedReplicas":3,"readyReplicas":3,"availableReplicas":3}}`, false},
		{"CKA-003", "q15", `{"spec":{"type":"ClusterIP","selector":{"app":"status"},"ports":[{"port":8080,"targetPort":80}]},` + ready + `}`, true},
		{"CKA-003", "q16", `{"provisioner":"kubernetes.io/no-provisioner","volumeBindingMode":"WaitForFirstConsumer","allowVolumeExpansion":true}`, false},
	}
	exams, err := loadExams("../../exam")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		modes := []string{"correct", "missing"}
		if tc.probe {
			modes = append(modes, "mismatch", "transport-error")
		}
		if tc.exam == "CKAD-001" && (tc.question == "q16" || tc.question == "q18") {
			modes = append(modes, "unready")
		}
		for _, mode := range modes {
			t.Run(tc.exam+"/"+tc.question+"/"+mode, func(t *testing.T) {
				t.Setenv("CAPTURE_OBJECT", tc.object)
				t.Setenv("PROBE_MODE", mode)
				if mode == "missing" {
					t.Setenv("CAPTURE_OBJECT", "{}")
				}
				if mode == "unready" {
					t.Setenv("CAPTURE_OBJECT", strings.Replace(tc.object, `"type":"Ready","status":"True"`, `"type":"Ready","status":"False"`, 1))
				}
				out, err := exec.Command("bash", filepath.Join("../../exam", tc.exam, "assets/capture.sh"), tc.question).CombinedOutput()
				if mode == "transport-error" {
					if err == nil {
						t.Fatalf("exec failure became an incorrect answer: %s", out)
					}
					if strings.Contains(string(out), "(NotFound)") {
						t.Fatalf("probe errors must not look like an absent answer to captureAnswer: %s", out)
					}
					t.Setenv("CAPTURE_COMMAND", "bash "+filepath.Join("../../exam", tc.exam, "assets/capture.sh")+" "+tc.question)
					a := testApp(t)
					snapshot := a.captureAnswer("", "", task{Resource: "probe"})
					if snapshot.Error == "" {
						t.Fatalf("captureAnswer swallowed the supporting probe failure: %+v", snapshot)
					}
					return
				}
				if err != nil {
					t.Fatalf("capture failed: %v\n%s", err, out)
				}
				var evidence struct {
					OK       bool
					Failures []string
				}
				if err := json.Unmarshal(out, &evidence); err != nil {
					t.Fatalf("invalid capture: %v\n%s", err, out)
				}
				want := mode == "correct"
				if evidence.OK != want || (len(evidence.Failures) == 0) != want {
					t.Fatalf("score/diagnostics disagree for %s: %s", mode, out)
				}
				for _, exam := range exams {
					if exam.ID != tc.exam {
						continue
					}
					for i, task := range exam.tasks {
						if fmt.Sprintf("q%d", i+1) != tc.question {
							continue
						}
						a := testApp(t)
						got := a.evaluateTask("", "", task, answerSnapshot{Data: out})
						if got.Passed != want || got.Error != "" || len(got.Checks[0].Details) != len(evidence.Failures) {
							t.Fatalf("grading lost diagnostics or changed score: %+v", got)
						}
					}
				}
			})
		}
	}
}

func TestCKA002RBACCapture(t *testing.T) {
	stubCommand(t, "kubectl", `#!/bin/bash
[[ ${RBAC_MODE:-} != api-error ]] || exit 42
case "$*" in
  *'get role pod-log-reader'*) echo '{"rules":[{"apiGroups":[""],"resources":["pods","pods/log"],"verbs":["get","list"]}]}' ;;
  *'get rolebinding read-pod-logs'*) echo '{"roleRef":{"kind":"Role","name":"pod-log-reader"},"subjects":[{"kind":"ServiceAccount","name":"auditor","namespace":"exam-rbac"}]}' ;;
  *'auth can-i get pods --subresource=log -n exam-rbac'*) [[ ${RBAC_MODE:-} != auth-error ]] || exit 42; [[ ${RBAC_MODE:-} != denied ]] || { echo no; exit 1; }; echo yes ;;
  *'auth can-i'*) [[ ${RBAC_MODE:-} != auth-error ]] || exit 42; echo no; exit 1 ;;
  *) exit 42 ;;
esac
`)
	exams, err := loadExams("../../exam")
	if err != nil {
		t.Fatal(err)
	}
	for _, exam := range exams {
		if exam.ID != "CKA-002" {
			continue
		}
		task := exam.tasks[2]
		for _, mode := range []string{"correct", "denied", "api-error", "auth-error"} {
			t.Run(mode, func(t *testing.T) {
				t.Setenv("RBAC_MODE", mode)
				out, err := exec.Command("bash", "-c", task.Resource).CombinedOutput()
				if mode == "api-error" || mode == "auth-error" {
					if err == nil {
						t.Fatalf("API error became an RBAC denial: %s", out)
					}
					return
				}
				if err != nil {
					t.Fatalf("capture: %v\n%s", err, out)
				}
				a := testApp(t)
				got := a.evaluateTask("", "", task, answerSnapshot{Data: out})
				want := task.Points
				if mode == "denied" {
					want--
				}
				if got.Earned != want || got.Error != "" {
					t.Fatalf("RBAC score: %+v", got)
				}
			})
		}
		return
	}
	t.Fatal("CKA-002 missing")
}

// Use real kubectl parsing against a local fake API, never the user's cluster.
func TestKubectlLogSubresourceSpelling(t *testing.T) {
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		t.Skip("kubectl unavailable")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			fmt.Fprint(w, `{"kind":"APIGroupList","groups":[]}`)
		case "/api/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"pods","namespaced":true,"kind":"Pod","verbs":["get","list"]}]}`)
		case "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
			var review struct {
				Spec struct {
					ResourceAttributes struct{ Resource, Subresource string }
				}
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			var allowed bool
			if strings.Contains(r.Header.Get("Content-Type"), "protobuf") {
				// ResourceAttributes protobuf fields 5 (resource) and 6
				// (subresource), with exact length-delimited string values.
				allowed = bytes.Contains(body, []byte("\x2a\x04pods")) && bytes.Contains(body, []byte("\x32\x03log"))
			} else {
				if err := json.Unmarshal(body, &review); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				attrs := review.Spec.ResourceAttributes
				allowed = attrs.Resource == "pods" && attrs.Subresource == "log"
			}
			fmt.Fprintf(w, `{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","status":{"allowed":%t}}`, allowed)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	for _, resource := range []string{"pods --subresource=log", "pods/log", "pods/logs"} {
		args := []string{"--kubeconfig=/dev/null", "--server=" + server.URL,
			"--cache-dir=" + t.TempDir(), "--request-timeout=5s", "auth", "can-i", "get"}
		out, err := exec.Command(kubectl, append(args, strings.Fields(resource)...)...).CombinedOutput()
		want := resource == "pods --subresource=log"
		if (err == nil) != want || !strings.Contains(string(out), map[bool]string{true: "yes", false: "no"}[want]) {
			t.Fatalf("%s: %v\n%s", resource, err, out)
		}
	}
}
