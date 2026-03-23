package domain

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ProblemError is an RFC 7807 Problem Details error that carries HTTP status
// and renders as application/problem+json. Use errors.As to extract from
// wrapped errors.
type ProblemError struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// NewProblemError creates a ProblemError with the given HTTP status, error type
// identifier, and human-readable detail message. The Type field is formatted as
// a URN: urn:flexgateway:error:{errType}.
func NewProblemError(status int, errType string, detail string) *ProblemError {
	return &ProblemError{
		Type:   "urn:flexgateway:error:" + errType,
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	}
}

// Error implements the error interface with a human-readable message.
func (e *ProblemError) Error() string {
	return fmt.Sprintf("%s: %s", http.StatusText(e.Status), e.Detail)
}

// StatusCode returns the HTTP status code for this error.
func (e *ProblemError) StatusCode() int {
	return e.Status
}

// MarshalJSON produces RFC 7807 JSON with type, title, status, and detail fields.
func (e *ProblemError) MarshalJSON() ([]byte, error) {
	type problemJSON struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}
	return json.Marshal(problemJSON{
		Type:   e.Type,
		Title:  e.Title,
		Status: e.Status,
		Detail: e.Detail,
	})
}
