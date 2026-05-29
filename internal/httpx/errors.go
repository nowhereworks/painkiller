package httpx

import (
	"net/http"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func WriteError(w http.ResponseWriter, status int, message string) {
	_ = WriteJSON(w, status, ErrorResponse{Error: message})
}

func BadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, message)
}

func Unauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, message)
}

func Forbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, message)
}

func NotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, message)
}

func InternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, message)
}
