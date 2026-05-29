package terminal

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"painkiller-shell/internal/attempts"
	"painkiller-shell/internal/models"
	"painkiller-shell/internal/store"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Gateway struct {
	store    *store.Store
	attempts *attempts.Service
	logger   *slog.Logger
}

func NewGateway(store *store.Store, attemptsSvc *attempts.Service, logger *slog.Logger) *Gateway {
	return &Gateway{
		store:    store,
		attempts: attemptsSvc,
		logger:   logger,
	}
}

func (g *Gateway) RegisterRoutes(r chi.Router) {
	r.Get("/{token}", g.handleWebSocket)
}

type resizeMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (g *Gateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	session, err := g.store.Sessions().GetByTerminalToken(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	attempt, err := g.store.Attempts().GetByID(r.Context(), session.AttemptID)
	if err != nil {
		http.Error(w, "attempt not found", http.StatusNotFound)
		return
	}

	if attempt.Status != models.AttemptStatusEnvironmentReady && attempt.Status != models.AttemptStatusRunning && attempt.Status != models.AttemptStatusTerminalOpened {
		http.Error(w, "attempt not active", http.StatusForbidden)
		return
	}

	env, err := g.store.Environments().GetByID(r.Context(), session.EnvironmentID)
	if err != nil {
		http.Error(w, "environment not found", http.StatusNotFound)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		g.logger.Error("failed to upgrade websocket", "error", err)
		return
	}
	defer ws.Close()

	if session.FirstOpenedAt == nil {
		now := time.Now()
		if err := g.store.Sessions().UpdateFirstOpenedAt(r.Context(), session.ID, now); err != nil {
			g.logger.Error("failed to update first_opened_at", "error", err)
		}
		_ = g.attempts.TransitionAttempt(r.Context(), session.AttemptID, models.AttemptStatusTerminalOpened)
		_ = g.attempts.TransitionAttempt(r.Context(), session.AttemptID, models.AttemptStatusRunning)
	}

	sshClient, err := DialSSH(r.Context(), *env.WorkstationIP, 22, "root", env.SSHPrivateKey)
	if err != nil {
		g.logger.Error("failed to dial SSH", "error", err)
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "SSH connection failed"))
		return
	}
	defer sshClient.Close()

	sshSession, err := NewSSHSession(sshClient, 80, 24)
	if err != nil {
		g.logger.Error("failed to create SSH session", "error", err)
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "SSH session failed"))
		return
	}
	defer sshSession.Close()

	stdin, err := sshSession.StdinPipe()
	if err != nil {
		g.logger.Error("failed to get stdin pipe", "error", err)
		return
	}

	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		g.logger.Error("failed to get stdout pipe", "error", err)
		return
	}

	stderr, err := sshSession.StderrPipe()
	if err != nil {
		g.logger.Error("failed to get stderr pipe", "error", err)
		return
	}

	if err := sshSession.Shell(); err != nil {
		g.logger.Error("failed to start shell", "error", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}

			var msg resizeMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "resize" {
				_ = sshSession.WindowChange(msg.Rows, msg.Cols)
				continue
			}

			if _, err := stdin.Write(data); err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				return
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(ws.UnderlyingConn(), stderr)
	}()

	wg.Wait()
}
