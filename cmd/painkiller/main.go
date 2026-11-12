package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type task struct {
	ID, Title, Instructions, Check, Solution string
	Points                                   int
}
type result struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
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
}

func (a *app) save() {
	b, err := json.MarshalIndent(a.state, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(a.file), 0700)
	}
	if err == nil {
		var f *os.File
		f, err = os.CreateTemp(filepath.Dir(a.file), ".state-*")
		if err == nil {
			name := f.Name()
			defer os.Remove(name)
			if err = f.Chmod(0600); err == nil {
				_, err = f.Write(b)
			}
			if e := f.Close(); err == nil {
				err = e
			}
			if err == nil {
				err = os.Rename(name, a.file)
			}
		}
	}
	if err != nil {
		log.Printf("save state: %v", err)
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
		a.save()
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
func (a *app) prepare() {
	out, err := output("scripts/create-lab.sh", "create")
	if err == nil {
		out, err = output("scripts/create-lab.sh", "provision")
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
	a.save()
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
		out, err := output("timeout", "30s", "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", user+"@"+host, "bash -c "+shellQuote(t.Check))
		res := result{ID: t.ID, Passed: err == nil}
		if err != nil {
			var e *exec.ExitError
			if !errors.As(err, &e) || e.ExitCode() != 1 {
				res.Error = fmt.Sprintf("check failed: %v %s", err, strings.TrimSpace(out))
			}
		}
		results = append(results, res)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Results = results
	a.state.Phase = "completed"
	a.save()
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
	switch strings.TrimPrefix(r.URL.Path, "/api/") {
	case "prepare":
		if a.state.Phase != "idle" {
			http.Error(w, "lab is not idle", 409)
			return
		}
		a.state.Phase = "preparing"
		a.save()
		go a.prepare()
	case "start":
		if a.state.Phase != "ready" {
			http.Error(w, "lab is not ready", 409)
			return
		}
		a.state.Phase = "running"
		a.state.Deadline = time.Now().Add(120 * time.Minute)
		a.state.Flags = map[string]bool{}
		a.state.Done = map[string]bool{}
		a.save()
	case "submit":
		if a.state.Phase != "running" {
			http.Error(w, "exam is not running", 409)
			return
		}
		a.state.Phase = "grading"
		a.closeTerminals()
		a.save()
		go a.grade()
	case "reset":
		if a.state.Phase == "running" || a.state.Phase == "preparing" || a.state.Phase == "grading" || a.state.Phase == "resetting" {
			http.Error(w, "operation in progress", 409)
			return
		}
		a.state.Phase = "resetting"
		a.save()
		go func() {
			out, err := output("scripts/create-lab.sh", "destroy")
			a.mu.Lock()
			defer a.mu.Unlock()
			if err != nil {
				a.state.Phase = "error"
				a.state.Message = fmt.Sprintf("%v: %s", err, out)
			} else {
				a.state = state{Phase: "idle"}
			}
			a.save()
		}()
	case "marks":
		if a.state.Phase != "running" {
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
		a.save()
	default:
		http.NotFound(w, r)
		return
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
	if a.state.Phase != "running" {
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
	a := &app{file: env("PAINKILLER_STATE", "data/state.json"), terminals: map[*os.File]*exec.Cmd{}, state: state{Phase: "idle"}}
	if err = json.Unmarshal(b, &a.tasks); err != nil {
		log.Fatal(err)
	}
	if b, err = os.ReadFile(a.file); err == nil {
		if err = json.Unmarshal(b, &a.state); err != nil {
			log.Fatal(err)
		}
	} else if !os.IsNotExist(err) {
		log.Fatal(err)
	}
	if a.state.Phase == "preparing" || a.state.Phase == "resetting" {
		a.state.Phase = "error"
		a.state.Message = "Service restarted during a lab operation. Inspect the lab and reset it."
		a.save()
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
	http.Handle("/", http.FileServer(http.Dir("web")))
	addr := env("PAINKILLER_LISTEN", "127.0.0.1:8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
