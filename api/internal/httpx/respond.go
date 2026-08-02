package httpx

import (
	"encoding/json"
	"net/http"

	"flatmatch/api/internal/logctx"
)

type Code string

const (
	CodeBadRequest   Code = "bad_request"
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
	CodeNotFound     Code = "not_found"
	CodeInternal     Code = "internal"
)

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logctx.From(r.Context()).Error("write response body", "error", err)
	}
}

func Error(w http.ResponseWriter, r *http.Request, status int, code Code, message string, cause error) {
	log := logctx.From(r.Context())
	attrs := []any{"code", code, "status", status}
	if cause != nil {
		attrs = append(attrs, "error", cause)
	}
	if status >= http.StatusInternalServerError {
		log.Error(message, attrs...)
	} else {
		log.Info(message, attrs...)
	}

	JSON(w, r, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message}})
}

func Internal(w http.ResponseWriter, r *http.Request, cause error) {
	Error(w, r, http.StatusInternalServerError, CodeInternal, "internal error", cause)
}

func Unauthorized(w http.ResponseWriter, r *http.Request, message string, cause error) {
	Error(w, r, http.StatusUnauthorized, CodeUnauthorized, message, cause)
}
