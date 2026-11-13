package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"painkiller/web"
)

type task struct {
	ID, Title, Instructions, Check, Solution, Explanation string
	Resource                                              string
	Points                                                int
	Checks                                                []check
	Steps                                                 []string
}
type check struct {
	Description string
	Points      int
	Command     string
	Expression  string
}
type answerSnapshot struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}
type checkResult struct {
	Description string `json:"description"`
	Points      int    `json:"points"`
	Earned      int    `json:"earned"`
	Error       string `json:"error,omitempty"`
}
type result struct {
	ID     string        `json:"id"`
	Passed bool          `json:"passed"`
	Error  string        `json:"error,omitempty"`
	Earned int           `json:"earned"`
	Checks []checkResult `json:"checks,omitempty"`
}
type state struct {
	TestID          string                    `json:"test_id,omitempty"`
	Lab             *labConfig                `json:"lab,omitempty"`
	Phase           string                    `json:"phase"`
	Deadline        time.Time                 `json:"deadline,omitempty"`
	DurationSeconds int64                     `json:"duration_seconds,omitempty"`
	Results         []result                  `json:"results,omitempty"`
	Flags           map[string]bool           `json:"flags,omitempty"`
	Done            map[string]bool           `json:"done,omitempty"`
	Message         string                    `json:"message,omitempty"`
	FailedOperation string                    `json:"failed_operation,omitempty"`
	Host            string                    `json:"host,omitempty"`
	Selected        string                    `json:"selected,omitempty"`
	Answers         map[string]answerSnapshot `json:"answers,omitempty"`
}
type app struct {
	exams     []examDefinition
	mu        sync.Mutex
	state     state
	tasks     []task
	terminals map[*os.File]*exec.Cmd
	file      string
	lockDir   string
	db        *sql.DB
	duration  time.Duration
}

func (a *app) save() error {
	b, err := json.MarshalIndent(a.state, "", "  ")
	if err != nil {
		return err
	}
	_, err = a.db.Exec(`INSERT INTO attempt (id, state) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET state=excluded.state`, string(b))
	return err
}
func (a *app) saveBackground() {
	if err := a.save(); err != nil {
		log.Printf("persist background operation: %v", err)
		a.state.Phase = "error"
		a.state.FailedOperation = "persistence"
		a.state.Message = "Could not persist operation result; check disk and restart before proceeding."
	}
}
func (a *app) closeTerminals() {
	for f, c := range a.terminals {
		_ = c.Process.Kill()
		_ = f.Close()
		delete(a.terminals, f)
	}
}
func (a *app) expire() {
	if a.state.Phase == "running" && !time.Now().Before(a.state.Deadline) {
		a.state.Phase = "grading"
		a.closeTerminals()
		if err := a.save(); err != nil {
			log.Printf("persist expiration: %v", err)
			a.state.Phase = "running" // fail closed and retry on the next tick
			return
		}
		go a.grade()
	}
}
func (a *app) status(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.expire()
	public := a.state
	// Older saved attempts have no duration; use the configured duration as a fallback.
	if public.DurationSeconds == 0 {
		public.DurationSeconds = int64(a.duration / time.Second)
		if public.DurationSeconds == 0 {
			public.DurationSeconds = 120 * 60
		}
	}
	public.Answers = nil // snapshots can contain secrets
	public.Lab = nil     // operational configuration is server-only
	json.NewEncoder(w).Encode(public)
}
func output(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	b, e := c.CombinedOutput()
	return string(b), e
}
func (a *app) run(name string, args ...string) (string, error) {
	f, err := os.OpenFile(filepath.Join(filepath.Dir(a.file), "operation.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var b bytes.Buffer
	// The lock is inherited by child processes after the server exits.
	lockDir := a.lockDir
	if lockDir == "" {
		lockDir = filepath.Dir(a.file)
	}
	command := append([]string{"-n", "-E", "75", filepath.Join(lockDir, "lab-operation.lock"), name}, args...)
	c := a.labCommand("flock", command...)
	c.Stdout = io.MultiWriter(f, &b)
	c.Stderr = c.Stdout
	err = c.Run()
	text := b.String()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 75 {
		return "Another lab operation is still running; retry after it finishes.", err
	}
	if len(text) > 8192 {
		text = text[len(text)-8192:]
	}
	return text, err
}
func (a *app) prepare() {
	a.prepareLab(true)
}
func (a *app) prepareLab(create bool) {
	logPath := filepath.Join(filepath.Dir(a.file), "operation.log")
	var logErr error
	if create {
		logErr = os.WriteFile(logPath, nil, 0600)
	} else {
		var f *os.File
		f, logErr = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if logErr == nil {
			_, logErr = fmt.Fprintln(f, "\n--- Retrying preparation ---")
			if e := f.Close(); logErr == nil {
				logErr = e
			}
		}
	}
	if logErr != nil {
		a.mu.Lock()
		a.state.Phase = "error"
		a.state.FailedOperation = "prepare"
		a.state.Message = logErr.Error()
		if e := a.save(); e != nil {
			log.Print(e)
		}
		a.mu.Unlock()
		return
	}
	var out string
	var err error
	preparation, err := a.preparationPath()
	if err == nil && preparation != "" {
		out, err = a.run("timeout", "180s", "bash", preparation, "preflight")
	}
	// Record mutation eligibility only after an all-range read-only preflight.
	// A failed occupied-ID preflight must not let reset destroy an older lab
	// that happens to carry the same operator-configured marker.
	needsCreate := create || (a.state.Lab != nil && !a.state.Lab.CreationStarted)
	if err == nil && needsCreate {
		out, err = a.run("scripts/create-lab.sh", "preflight")
		if err == nil && a.state.Lab != nil {
			a.mu.Lock()
			a.state.Lab.CreationStarted = true
			err = a.save()
			if err != nil {
				a.state.Lab.CreationStarted = false
			}
			a.mu.Unlock()
		}
		if err == nil {
			out, err = a.run("scripts/create-lab.sh", "create")
		}
	} else if err == nil {
		out, err = a.run("scripts/create-lab.sh", "status")
	}
	if err == nil && preparation != "" {
		out, err = a.run("timeout", "1800s", "bash", preparation, "infrastructure")
	}
	if err == nil {
		out, err = a.run("scripts/create-lab.sh", "provision")
	}
	if err == nil {
		out, err = a.run("scripts/configure-exam-access.sh")
	}
	var host string
	if err == nil {
		var s struct {
			Controllers string `json:"controller_ips"`
		}
		var b []byte
		b, err = a.labCommand("scripts/create-lab.sh", "status").Output()
		if err == nil {
			err = json.Unmarshal(b, &s)
			host = strings.Split(s.Controllers, ",")[0]
			if host == "" {
				err = errors.New("missing controller IP")
			}
		}
	}
	if err == nil && preparation != "" {
		out, err = a.run("timeout", "2400s", "bash", preparation, "fixtures")
	}
	if err == nil && preparation == "" {
		var setup *os.File
		setup, err = os.Open(a.setupPath())
		if err == nil {
			defer setup.Close()
			args := append([]string{"120s", "ssh"}, a.labSSHArgs()...)
			args = append(args, a.labValue("LAB_SSH_USER", "milkman")+"@"+host, "kubectl apply -f -")
			cmd := exec.Command("timeout", args...)
			cmd.Stdin = setup
			var b []byte
			b, err = cmd.CombinedOutput()
			out = string(b)
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.state.Phase = "error"
		a.state.FailedOperation = "prepare"
		a.state.Message = fmt.Sprintf("%v: %s", err, out)
	} else {
		a.state = state{Phase: "ready", Host: host, TestID: a.state.TestID, Lab: a.state.Lab}
	}
	a.saveBackground()
}

// captureConcurrency bounds simultaneous answer captures. Captures run in
// parallel so that the interval in which live cluster state is read stays as
// short as possible, while staying below the SSH server's connection limit.
const captureConcurrency = 6

func (a *app) captureAnswer(user, host string, t task) answerSnapshot {
	args := append([]string{"30s", "ssh"}, a.labSSHArgs()...)
	args = append(args, user+"@"+host, t.Resource)
	out, err := output("timeout", args...)
	snap := answerSnapshot{Data: json.RawMessage("null")}
	switch {
	case err != nil && strings.Contains(out, "(NotFound)"):
		// A deleted namespace or object is an incorrect answer, not a grader outage.
	case err != nil:
		snap.Error = fmt.Sprintf("capture failed: %v %s", err, strings.TrimSpace(out))
	case strings.TrimSpace(out) == "":
	case !json.Valid([]byte(out)):
		snap.Error = "capture returned invalid JSON"
	default:
		snap.Data = json.RawMessage(out)
	}
	return snap
}

// captureAnswers reads every pending resource concurrently and stores the
// result immediately. Successfully captured answers are never re-read; only
// failed captures are retried (once). Returns false when persistence fails.
func (a *app) captureAnswers(user, host string, pending []task, answers map[string]answerSnapshot) bool {
	for pass := 0; pass < 2 && len(pending) > 0; pass++ {
		if pass > 0 {
			time.Sleep(time.Second)
		}
		captured := make([]answerSnapshot, len(pending))
		slots := make(chan struct{}, captureConcurrency)
		var wg sync.WaitGroup
		for i, t := range pending {
			wg.Add(1)
			go func(i int, t task) {
				defer wg.Done()
				slots <- struct{}{}
				defer func() { <-slots }()
				captured[i] = a.captureAnswer(user, host, t)
			}(i, t)
		}
		wg.Wait()
		var failed []task
		for i, t := range pending {
			answers[t.ID] = captured[i]
			if captured[i].Error != "" {
				failed = append(failed, t)
			}
		}
		a.mu.Lock()
		a.state.Answers = answers
		saveErr := a.save()
		a.mu.Unlock()
		if saveErr != nil {
			log.Printf("persist answer snapshot: %v", saveErr)
			a.mu.Lock()
			a.state.Phase = "error"
			a.state.FailedOperation = "persistence"
			a.state.Message = "Could not persist captured answers; restart to retry grading."
			a.mu.Unlock()
			return false
		}
		pending = failed
	}
	return true
}

func (a *app) reset() {
	var out string
	var err error
	if a.state.Lab == nil || a.state.Lab.CreationStarted {
		out, err = a.run("scripts/create-lab.sh", "destroy")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.state.Phase = "error"
		a.state.FailedOperation = "reset"
		a.state.Message = fmt.Sprintf("%v: %s", err, out)
	} else {
		a.state = state{Phase: "idle"}
	}
	a.saveBackground()
}

// resume continues an attempt after the server restarted. Lab operations that
// were interrupted mid-flight cannot be trusted, so they become retryable
// errors; grading is safe to restart because captured answers are immutable.
func (a *app) resume() error {
	if a.state.Phase == "preparing" || a.state.Phase == "resetting" {
		a.state.FailedOperation = "prepare"
		if a.state.Phase == "resetting" {
			a.state.FailedOperation = "reset"
		}
		a.state.Phase = "error"
		a.state.Message = "Service restarted during a lab operation. Inspect the lab, then retry or reset it."
		if err := a.save(); err != nil {
			return err
		}
	}
	if a.state.Phase == "grading" {
		go a.grade()
	}
	return nil
}

func (a *app) grade() {
	a.mu.Lock()
	host := a.state.Host
	answers := a.state.Answers
	if answers == nil {
		answers = make(map[string]answerSnapshot)
	}
	a.mu.Unlock()
	user := a.labValue("LAB_SSH_USER", "milkman")
	// Capture every answer before evaluating any check. Captures run
	// concurrently, so the cluster is read within a few seconds; checks then
	// evaluate the immutable snapshots and never re-read live state.
	var pending []task
	for _, t := range a.tasks {
		if t.Resource == "" {
			continue
		}
		if saved, ok := answers[t.ID]; ok && saved.Error == "" {
			continue
		}
		pending = append(pending, t)
	}
	if !a.captureAnswers(user, host, pending, answers) {
		return
	}
	results := make([]result, 0, len(a.tasks))
	for _, t := range a.tasks {
		if t.Resource != "" && answers[t.ID].Error != "" {
			results = append(results, result{ID: t.ID, Error: answers[t.ID].Error})
			continue
		}
		if t.Resource == "" {
			args := append([]string{"30s", "ssh"}, a.labSSHArgs()...)
			args = append(args, user+"@"+host, "kubectl get --raw=/readyz")
			if health, err := output("timeout", args...); err != nil {
				results = append(results, result{ID: t.ID, Error: fmt.Sprintf("cluster unavailable: %v %s", err, strings.TrimSpace(health))})
				continue
			}
		}
		checks := t.Checks
		if len(checks) == 0 {
			checks = []check{{Description: "Task requirements", Points: t.Points, Command: t.Check}}
		}
		res := result{ID: t.ID}
		for _, c := range checks {
			var out string
			var err error
			if t.Resource != "" {
				// "// false" makes a missing path score zero instead of
				// producing no output, which jq reports as an error.
				cmd := exec.Command("timeout", "30s", "jq", "-e", "try (("+c.Expression+") // false) catch false")
				cmd.Stdin = bytes.NewReader(answers[t.ID].Data)
				var b []byte
				b, err = cmd.CombinedOutput()
				out = strings.TrimSpace(string(b))
			} else {
				args := append([]string{"30s", "ssh"}, a.labSSHArgs()...)
				args = append(args, user+"@"+host, "bash -o pipefail -c "+shellQuote(c.Command))
				out, err = output("timeout", args...)
			}
			cr := checkResult{Description: c.Description, Points: c.Points}
			if err == nil {
				cr.Earned = c.Points
				res.Earned += c.Points
			} else if checkUnavailable(err, out) {
				cr.Error = fmt.Sprintf("check failed: %v %s", err, strings.TrimSpace(out))
				res.Error = "One or more checks could not be graded"
			}
			res.Checks = append(res.Checks, cr)
		}
		res.Passed = res.Earned == t.Points && res.Error == ""
		results = append(results, res)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Results = results
	a.state.Phase = "completed"
	a.saveBackground()
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// checkUnavailable reports whether a failed check points at a grader or
// infrastructure problem rather than an incorrect answer. Both jq (-e) and
// shell checks exit with status 1 for "answer does not match".
func checkUnavailable(err error, out string) bool {
	if err == nil || strings.Contains(out, "(NotFound)") {
		return false
	}
	var exit *exec.ExitError
	return !errors.As(err, &exit) || exit.ExitCode() != 1
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func (a *app) action(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.expire()
	before := a.state
	beforeTasks, beforeDuration := a.tasks, a.duration
	var next func()
	switch strings.TrimPrefix(r.URL.Path, "/api/") {
	case "prepare":
		if a.state.Phase != "idle" {
			http.Error(w, "lab is not idle", 409)
			return
		}
		var selection struct {
			TestID string `json:"test_id"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&selection) != nil {
			http.Error(w, "invalid test selection", 400)
			return
		}
		if err := a.selectExam(selection.TestID); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		for _, exam := range a.exams {
			if exam.ID != selection.TestID {
				continue
			}
			config, err := resolveLab(exam)
			if err != nil {
				a.tasks, a.duration = beforeTasks, beforeDuration
				http.Error(w, err.Error(), 400)
				return
			}
			a.state.Lab = config
		}
		a.state.TestID = selection.TestID
		a.state.Phase = "preparing"
		next = a.prepare
	case "retry":
		if a.state.Phase != "error" || (a.state.FailedOperation != "prepare" && a.state.FailedOperation != "reset") {
			http.Error(w, "operation is not retryable", 409)
			return
		}
		failed := a.state.FailedOperation
		a.state.FailedOperation = ""
		a.state.Message = ""
		if failed == "reset" {
			a.state.Phase = "resetting"
			next = a.reset
		} else {
			a.state.Phase = "preparing"
			next = func() { a.prepareLab(false) }
		}
	case "start":
		if a.state.Phase != "ready" {
			http.Error(w, "lab is not ready", 409)
			return
		}
		a.state.Phase = "running"
		duration := a.duration
		if duration == 0 {
			duration = 120 * time.Minute
		}
		a.state.Deadline = time.Now().Add(duration)
		a.state.DurationSeconds = int64(duration / time.Second)
		a.state.Flags = map[string]bool{}
		a.state.Done = map[string]bool{}
	case "submit":
		if a.state.Phase != "running" || !time.Now().Before(a.state.Deadline) {
			http.Error(w, "exam is not running", 409)
			return
		}
		a.state.Phase = "grading"
		next = a.grade
	case "regrade":
		if a.state.Phase != "completed" {
			http.Error(w, "results are not ready", 409)
			return
		}
		hasError := false
		for _, r := range a.state.Results {
			if r.Error != "" {
				hasError = true
			}
		}
		if !hasError {
			http.Error(w, "no grading errors", 409)
			return
		}
		a.state.Phase = "grading"
		next = a.grade
	case "reset":
		if a.state.Phase == "running" || a.state.Phase == "preparing" || a.state.Phase == "grading" || a.state.Phase == "resetting" {
			http.Error(w, "operation in progress", 409)
			return
		}
		a.state.Phase = "resetting"
		a.state.Message = ""
		a.state.FailedOperation = ""
		next = a.reset
	case "marks":
		if a.state.Phase != "running" || !time.Now().Before(a.state.Deadline) {
			http.Error(w, "exam is not running", 409)
			return
		}
		var marks struct{ Flags, Done map[string]bool }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&marks) != nil {
			http.Error(w, "invalid marks", 400)
			return
		}
		a.state.Flags = marks.Flags
		a.state.Done = marks.Done
	case "mark":
		if a.state.Phase != "running" || !time.Now().Before(a.state.Deadline) {
			http.Error(w, "exam is not running", 409)
			return
		}
		var mark struct {
			ID         string
			Flag, Done bool
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&mark) != nil {
			http.Error(w, "invalid mark", 400)
			return
		}
		found := false
		for _, task := range a.tasks {
			if task.ID == mark.ID {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "unknown task", 400)
			return
		}
		flags := make(map[string]bool, len(a.state.Flags)+1)
		for k, v := range a.state.Flags {
			flags[k] = v
		}
		flags[mark.ID] = mark.Flag
		done := make(map[string]bool, len(a.state.Done)+1)
		for k, v := range a.state.Done {
			done[k] = v
		}
		done[mark.ID] = mark.Done
		a.state.Flags = flags
		a.state.Done = done
	case "select":
		if a.state.Phase != "running" || !time.Now().Before(a.state.Deadline) {
			http.Error(w, "exam is not running", 409)
			return
		}
		var selection struct{ ID string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&selection) != nil {
			http.Error(w, "invalid selection", 400)
			return
		}
		found := false
		for _, task := range a.tasks {
			if task.ID == selection.ID {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "unknown task", 400)
			return
		}
		a.state.Selected = selection.ID
	default:
		http.NotFound(w, r)
		return
	}
	if err := a.save(); err != nil {
		a.state = before
		a.tasks, a.duration = beforeTasks, beforeDuration
		http.Error(w, "could not persist attempt state", http.StatusServiceUnavailable)
		log.Printf("persist action: %v", err)
		return
	}
	if a.state.Phase == "grading" && before.Phase == "running" {
		a.closeTerminals()
	}
	if next != nil {
		go next()
	}
	w.WriteHeader(http.StatusNoContent)
}

var upgrader = websocket.Upgrader{CheckOrigin: sameOrigin}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, e := url.Parse(origin)
	return e == nil && u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https")
}
func (a *app) terminal(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.expire()
	if a.state.Phase != "running" || !time.Now().Before(a.state.Deadline) {
		a.mu.Unlock()
		http.Error(w, "exam is not running", 409)
		return
	}
	host := a.state.Host
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.mu.Unlock()
		return
	}
	args := append([]string{"-tt"}, a.labSSHArgs()...)
	args = append(args, a.labValue("LAB_SSH_USER", "milkman")+"@"+host)
	cmd := exec.Command("ssh", args...)
	f, err := pty.Start(cmd)
	if err != nil {
		a.mu.Unlock()
		ws.Close()
		return
	}
	a.terminals[f] = cmd
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.terminals, f)
		a.mu.Unlock()
		f.Close()
		cmd.Process.Kill()
		cmd.Wait()
		ws.Close()
	}()
	go func() {
		b := make([]byte, 4096)
		for {
			n, e := f.Read(b)
			if n > 0 {
				if ws.WriteMessage(websocket.BinaryMessage, b[:n]) != nil {
					return
				}
			}
			if e != nil {
				ws.Close()
				return
			}
		}
	}()
	for {
		typ, b, e := ws.ReadMessage()
		if e != nil {
			return
		}
		if typ == websocket.TextMessage {
			var msg struct{ Cols, Rows uint16 }
			if json.Unmarshal(b, &msg) == nil && msg.Cols > 0 && msg.Rows > 0 {
				_ = pty.Setsize(f, &pty.Winsize{Cols: msg.Cols, Rows: msg.Rows})
			}
		} else if typ == websocket.BinaryMessage {
			if _, e = f.Write(b); e != nil {
				return
			}
		}
	}
}
func main() {
	exams, err := loadExams("exam")
	if err != nil {
		log.Fatal(err)
	}
	a := &app{exams: exams, file: env("PAINKILLER_STATE", "data/state.db"), lockDir: env("PAINKILLER_LAB_LOCK_DIR", "data"), terminals: map[*os.File]*exec.Cmd{}, state: state{Phase: "idle"}}
	minutes, err := strconv.Atoi(env("PAINKILLER_MINUTES", "120"))
	if err != nil || minutes < 1 || minutes > 1440 {
		log.Fatal("PAINKILLER_MINUTES must be between 1 and 1440")
	}
	a.duration = time.Duration(minutes) * time.Minute
	if err = os.MkdirAll(filepath.Dir(a.file), 0700); err != nil {
		log.Fatal(err)
	}
	if err = os.MkdirAll(a.lockDir, 0700); err != nil {
		log.Fatal(err)
	}
	labLock, err := os.OpenFile(filepath.Join(a.lockDir, "lab-server.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer labLock.Close()
	if err = syscall.Flock(int(labLock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Fatal("another server controls this lab: ", err)
	}
	lock, err := os.OpenFile(a.file+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Fatal("another server holds the state lock: ", err)
	}
	f, err := os.OpenFile(a.file, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		log.Fatal(err)
	}
	if err = f.Chmod(0600); err != nil {
		log.Fatal(err)
	}
	f.Close()
	a.db, err = sql.Open("sqlite3", a.file)
	if err != nil {
		log.Fatal(err)
	}
	defer a.db.Close()
	if _, err = a.db.Exec(`CREATE TABLE IF NOT EXISTS attempt (id INTEGER PRIMARY KEY CHECK (id=1), state TEXT NOT NULL)`); err != nil {
		log.Fatal(err)
	}
	var stored string
	err = a.db.QueryRow(`SELECT state FROM attempt WHERE id=1`).Scan(&stored)
	if err == nil {
		if err = json.Unmarshal([]byte(stored), &a.state); err != nil {
			log.Fatal(err)
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		// Preserve the previous release's attempt when upgrading the default installation.
		if a.file == "data/state.db" {
			if legacy, e := os.ReadFile("data/state.json"); e == nil {
				if e = json.Unmarshal(legacy, &a.state); e != nil {
					log.Fatal(e)
				}
			} else if !os.IsNotExist(e) {
				log.Fatal(e)
			}
		}
		if err = a.save(); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal(err)
	}
	migrated, err := a.restoreExam()
	if err != nil {
		log.Fatal(err)
	}
	if normalizeLegacyScores(&a.state, a.tasks) || migrated {
		if err = a.save(); err != nil {
			log.Fatal(err)
		}
	}
	if err = a.resume(); err != nil {
		log.Fatal(err)
	}
	go func() {
		for range time.Tick(time.Second) {
			a.mu.Lock()
			a.expire()
			a.mu.Unlock()
		}
	}()
	http.HandleFunc("/api/status", a.status)
	http.HandleFunc("/api/tests", a.testList)
	http.HandleFunc("/api/log", func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open(filepath.Join(filepath.Dir(a.file), "operation.log"))
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			http.Error(w, "log unavailable", 500)
			return
		}
		defer f.Close()
		if stat, e := f.Stat(); e == nil && stat.Size() > 32768 {
			_, _ = f.Seek(-32768, io.SeekEnd)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.Copy(w, io.LimitReader(f, 32768))
	})
	http.HandleFunc("/api/tasks", a.taskList)
	http.HandleFunc("/api/terminal", a.terminal)
	http.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "origin required", 403)
			return
		}
		a.action(w, r)
	})
	http.Handle("/", http.FileServer(http.FS(web.Files)))
	addr := env("PAINKILLER_LISTEN", "127.0.0.1:8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func (a *app) taskList(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.expire()
	type publicTask struct {
		ID, Title, Instructions string
		Points                  int
		Solution                string   `json:",omitempty"`
		Explanation             string   `json:",omitempty"`
		Steps                   []string `json:",omitempty"`
	}
	ts := []publicTask{}
	if a.state.Phase == "running" || a.state.Phase == "completed" {
		for _, t := range a.tasks {
			ts = append(ts, publicTask{ID: t.ID, Title: t.Title, Instructions: t.Instructions, Points: t.Points, Solution: t.Solution, Explanation: t.Explanation, Steps: t.Steps})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ts)
}

func normalizeLegacyScores(s *state, tasks []task) bool {
	changed := false
	for i := range s.Results {
		r := &s.Results[i]
		if !r.Passed || r.Earned != 0 {
			continue
		}
		for _, t := range tasks {
			if t.ID == r.ID {
				r.Earned = t.Points
				changed = true
				break
			}
		}
	}
	return changed
}

func validateTasks(tasks []task) error {
	seen := map[string]bool{}
	total := 0
	for _, t := range tasks {
		if t.ID == "" || seen[t.ID] || t.Points <= 0 || t.Title == "" || t.Instructions == "" || t.Solution == "" {
			return fmt.Errorf("invalid task %q", t.ID)
		}
		seen[t.ID] = true
		total += t.Points
		if len(t.Checks) == 0 {
			if t.Check == "" {
				return fmt.Errorf("task %s has no check", t.ID)
			}
			continue
		}
		n := 0
		for _, c := range t.Checks {
			if c.Description == "" || (t.Resource == "" && c.Command == "") || (t.Resource != "" && c.Expression == "") || c.Points <= 0 {
				return fmt.Errorf("invalid check for %s", t.ID)
			}
			n += c.Points
		}
		if n != t.Points {
			return fmt.Errorf("check points do not sum to task %s", t.ID)
		}
	}
	if total != 100 {
		return fmt.Errorf("exam points total %d, expected 100", total)
	}
	return nil
}
