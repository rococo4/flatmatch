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

	CodeInvalidField      Code = "invalid_field"
	CodeProfileIncomplete Code = "profile_incomplete"
	CodeTooLarge          Code = "too_large"
	CodeUnsupportedMedia  Code = "unsupported_media"

	CodeSearchLimit  Code = "search_limit"
	CodeSearchClosed Code = "search_closed"

	CodeLikeLimit   Code = "like_limit"
	CodeNeedSearch  Code = "need_search"
	CodeNotApproved Code = "not_approved"

	CodePointLimit   Code = "point_limit"
	CodeDuplicateURL Code = "duplicate_url"
	CodeNeedEnter    Code = "need_enter"

	CodePointNotActive Code = "point_not_active"
	CodePointNotPaused Code = "point_not_paused"
	CodeNotVotable     Code = "not_votable"
)

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    Code     `json:"code"`
	Message string   `json:"message"`
	Fields  []string `json:"fields,omitempty"`
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
	ErrorFields(w, r, status, code, message, nil, cause)
}

func ErrorFields(w http.ResponseWriter, r *http.Request, status int, code Code, message string, fields []string, cause error) {
	log := logctx.From(r.Context())
	attrs := []any{"code", code, "status", status}
	if len(fields) > 0 {
		attrs = append(attrs, "fields", fields)
	}
	if cause != nil {
		attrs = append(attrs, "error", cause)
	}
	if status >= http.StatusInternalServerError {
		log.Error(message, attrs...)
	} else {
		log.Info(message, attrs...)
	}

	JSON(w, r, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message, Fields: fields}})
}

func BadRequest(w http.ResponseWriter, r *http.Request, code Code, message string, fields []string, cause error) {
	ErrorFields(w, r, http.StatusBadRequest, code, message, fields, cause)
}

func Internal(w http.ResponseWriter, r *http.Request, cause error) {
	Error(w, r, http.StatusInternalServerError, CodeInternal, "internal error", cause)
}

func Unauthorized(w http.ResponseWriter, r *http.Request, message string, cause error) {
	Error(w, r, http.StatusUnauthorized, CodeUnauthorized, message, cause)
}
