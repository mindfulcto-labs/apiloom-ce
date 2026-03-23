package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
)

// WriteProblem writes an RFC 7807 Problem Details JSON response.
func WriteProblem(w http.ResponseWriter, status int, errType, detail string) {
	pe := domain.NewProblemError(status, errType, detail)
	b, err := json.Marshal(pe)
	if err != nil {
		http.Error(w, `{"type":"urn:flexgateway:error:internal","title":"Internal Server Error","status":500,"detail":"failed to marshal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	w.Write(b) //nolint:errcheck
}

// ProblemFromError maps a domain sentinel error to a ProblemError with the appropriate
// HTTP status code.
func ProblemFromError(err error) *domain.ProblemError {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return domain.NewProblemError(http.StatusNotFound, "not-found", err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return domain.NewProblemError(http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, domain.ErrInvalidConfig):
		return domain.NewProblemError(http.StatusBadRequest, "invalid-config", err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		return domain.NewProblemError(http.StatusUnauthorized, "unauthorized", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return domain.NewProblemError(http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, domain.ErrRateLimited):
		return domain.NewProblemError(http.StatusTooManyRequests, "rate-limited", err.Error())
	case errors.Is(err, domain.ErrBudgetExceeded):
		return domain.NewProblemError(http.StatusTooManyRequests, "budget-exceeded", err.Error())
	default:
		return domain.NewProblemError(http.StatusInternalServerError, "internal", "internal server error")
	}
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, "internal", "failed to marshal response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(b) //nolint:errcheck
}
