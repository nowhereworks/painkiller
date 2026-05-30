package auth

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"painkiller-shell/internal/httpx"
	"painkiller-shell/internal/store"
)

type Handler struct {
	service    *Service
	jwtManager *JWTManager
	store      *store.Store
}

func NewHandler(service *Service, jwtManager *JWTManager, store *store.Store) *Handler {
	return &Handler{
		service:    service,
		jwtManager: jwtManager,
		store:      store,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		httpx.BadRequest(w, "email and password are required")
		return
	}

	user, err := h.service.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		if err == ErrEmailExists {
			httpx.BadRequest(w, "email already exists")
			return
		}
		httpx.InternalError(w, "registration failed")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, registerResponse{
		ID:    user.ID.String(),
		Email: user.Email,
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		httpx.BadRequest(w, "email and password are required")
		return
	}

	token, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if err == ErrInvalidCredentials {
			httpx.Unauthorized(w, "invalid credentials")
			return
		}
		httpx.InternalError(w, "login failed")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400,
	})

	_ = httpx.WriteJSON(w, http.StatusOK, loginResponse{Token: token})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type meResponse struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

func (h *Handler) HandleMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		httpx.Unauthorized(w, "unauthorized")
		return
	}

	user, err := h.store.Users().GetByID(r.Context(), userID)
	if err != nil {
		httpx.InternalError(w, "failed to fetch user")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, meResponse{
		ID:      user.ID.String(),
		Email:   user.Email,
		IsAdmin: user.IsAdmin,
	})
}
