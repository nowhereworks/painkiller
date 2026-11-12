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
	ID, Title, Instructions, Check, Solution string
	Points                                   int
	Checks                                   []check
}
type check struct {
	Description string
	Points      int
	Command     string
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
	Phase    string          `json:"phase"`
	Deadline time.Time       `json:"deadline,omitempty"`
	Results  []result        `json:"results,omitempty"`
	Flags    map[string]bool `json:"flags,omitempty"`
	Done     map[string]bool `json:"done,omitempty"`
	Message  string          `json:"message,omitempty"`
	Host     string          `json:"host,omitempty"`
}
type app struct {
	mu        sync.Mutex
	state     state
	tasks     []task
	terminals map[*os.File]*exec.Cmd
	file      string
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
	json.NewEncoder(w).Encode(a.state)
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
	c := exec.Command(name, args...)
	c.Stdout = io.MultiWriter(f, &b)
	c.Stderr = c.Stdout
	err = c.Run()
	text := b.String()
	if len(text) > 8192 {
		text = text[len(text)-8192:]
	}
	return text, err
}
func (a *app) prepare() {
	if err := os.WriteFile(filepath.Join(filepath.Dir(a.file), "operation.log"), nil, 0600); err != nil {
		a.mu.Lock()
		a.state.Phase = "error"
		a.state.Message = err.Error()
		if e := a.save(); e != nil {
			log.Print(e)
		}
		a.mu.Unlock()
		return
	}
	out, err := a.run("scripts/create-lab.sh", "create")
	if err == nil {
		out, err = a.run("scripts/create-lab.sh", "provision")
	}
	var host string
	if err == nil {
		var s struct {
			Controllers string `json:"controller_ips"`
		}
		var b []byte
		b, err = exec.Command("scripts/create-lab.sh", "status").Output()
		if err == nil {
			err = json.Unmarshal(b, &s)
			host = strings.Split(s.Controllers, ",")[0]
			if host == "" {
				err = errors.New("missing controller IP")
			}
		}
	}
	if err == nil {
		var setup *os.File
		setup, err = os.Open("exam/setup.yaml")
		if err == nil {
			defer setup.Close()
			cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", env("LAB_SSH_USER", "milkman")+"@"+host, "kubectl apply -f -")
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
		a.state.Message = fmt.Sprintf("%v: %s", err, out)
	} else {
		a.state = state{Phase: "ready", Host: host}
	}
	a.saveBackground()
}
func (a *app) grade() {
	a.mu.Lock()
	host := a.state.Host
	a.mu.Unlock()
	user := env("LAB_SSH_USER", "milkman")
	results := make([]result, 0, len(a.tasks))
	for _, t := range a.tasks {
		if health, err := output("timeout", "30s", "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", user+"@"+host, "kubectl get --raw=/readyz"); err != nil {
			results = append(results, result{ID: t.ID, Error: fmt.Sprintf("cluster unavailable: %v %s", err, strings.TrimSpace(health))})
			continue
		}
		checks := t.Checks
		if len(checks) == 0 {
			checks = []check{{Description: "Task requirements", Points: t.Points, Command: t.Check}}
		}
		res := result{ID: t.ID}
		for _, c := range checks {
			out, err := output("timeout", "30s", "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", user+"@"+host, "bash -o pipefail -c "+shellQuote(c.Command))
			cr := checkResult{Description: c.Description, Points: c.Points}
			if err == nil {
				cr.Earned = c.Points
				res.Earned += c.Points
			} else {
				var e *exec.ExitError
				if !strings.Contains(out, "(NotFound)") && (!errors.As(err, &e) || e.ExitCode() != 1 || strings.TrimSpace(out) != "") {
					cr.Error = fmt.Sprintf("check failed: %v %s", err, strings.TrimSpace(out))
					res.Error = "One or more checks could not be graded"
				}
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
	var next func()
	switch strings.TrimPrefix(r.URL.Path, "/api/") {
	case "prepare":
		if a.state.Phase != "idle" {
			http.Error(w, "lab is not idle", 409)
			return
		}
		a.state.Phase = "preparing"
		next = a.prepare
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
		next = func() {
			out, err := a.run("scripts/create-lab.sh", "destroy")
			a.mu.Lock()
			defer a.mu.Unlock()
			if err != nil {
				a.state.Phase = "error"
				a.state.Message = fmt.Sprintf("%v: %s", err, out)
			} else {
				a.state = state{Phase: "idle"}
			}
			a.saveBackground()
		}
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
	default:
		http.NotFound(w, r)
		return
	}
	if err := a.save(); err != nil {
		a.state = before
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
	cmd := exec.Command("ssh", "-tt", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", env("LAB_SSH_USER", "milkman")+"@"+host)
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
	b, err := os.ReadFile("exam/tasks.json")
	if err != nil {
		log.Fatal(err)
	}
	a := &app{file: env("PAINKILLER_STATE", "data/state.db"), terminals: map[*os.File]*exec.Cmd{}, state: state{Phase: "idle"}}
	if err = json.Unmarshal(b, &a.tasks); err != nil {
		log.Fatal(err)
	}
	if err = validateTasks(a.tasks); err != nil {
		log.Fatal(err)
	}
	minutes, err := strconv.Atoi(env("PAINKILLER_MINUTES", "120"))
	if err != nil || minutes < 1 || minutes > 1440 {
		log.Fatal("PAINKILLER_MINUTES must be between 1 and 1440")
	}
	a.duration = time.Duration(minutes) * time.Minute
	if err = os.MkdirAll(filepath.Dir(a.file), 0700); err != nil {
		log.Fatal(err)
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
	if a.state.Phase == "preparing" || a.state.Phase == "resetting" {
		a.state.Phase = "error"
		a.state.Message = "Service restarted during a lab operation. Inspect the lab and reset it."
		if err = a.save(); err != nil {
			log.Fatal(err)
		}
	}
	if a.state.Phase == "grading" {
		go a.grade()
	}
	go func() {
		for range time.Tick(time.Second) {
			a.mu.Lock()
			a.expire()
			a.mu.Unlock()
		}
	}()
	http.HandleFunc("/api/status", a.status)
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
	http.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.expire()
		type publicTask struct {
			ID, Title, Instructions string
			Points                  int
			Solution                string `json:",omitempty"`
		}
		ts := []publicTask{}
		if a.state.Phase == "running" || a.state.Phase == "completed" {
			for _, t := range a.tasks {
				p := publicTask{ID: t.ID, Title: t.Title, Instructions: t.Instructions, Points: t.Points}
				if a.state.Phase == "completed" {
					p.Solution = t.Solution
				}
				ts = append(ts, p)
			}
		}
		json.NewEncoder(w).Encode(ts)
	})
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
			if c.Description == "" || c.Command == "" || c.Points <= 0 {
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
