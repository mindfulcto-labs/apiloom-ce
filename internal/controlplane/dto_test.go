package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/cto-venkat/apiloom-ce/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListParams_DefaultLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		limit    int
		expected int
	}{
		{name: "zero defaults to 20", limit: 0, expected: 20},
		{name: "negative defaults to 20", limit: -1, expected: 20},
		{name: "within range preserved", limit: 50, expected: 50},
		{name: "exceeds max capped to 100", limit: 200, expected: 100},
		{name: "exactly 100 preserved", limit: 100, expected: 100},
		{name: "exactly 1 preserved", limit: 1, expected: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lp := ListParams{Limit: tt.limit}
			assert.Equal(t, tt.expected, lp.EffectiveLimit())
		})
	}
}

func TestListParams_ParseFromQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		query          url.Values
		expectedCursor string
		expectedLimit  int
	}{
		{
			name:           "empty query",
			query:          url.Values{},
			expectedCursor: "",
			expectedLimit:  20,
		},
		{
			name:           "cursor and limit",
			query:          url.Values{"cursor": {"abc"}, "limit": {"50"}},
			expectedCursor: "abc",
			expectedLimit:  50,
		},
		{
			name:           "invalid limit falls back to default",
			query:          url.Values{"limit": {"notanumber"}},
			expectedCursor: "",
			expectedLimit:  20,
		},
		{
			name:           "limit exceeding max is capped",
			query:          url.Values{"limit": {"500"}},
			expectedCursor: "",
			expectedLimit:  100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lp := ParseListParams(tt.query)
			assert.Equal(t, tt.expectedCursor, lp.Cursor)
			assert.Equal(t, tt.expectedLimit, lp.EffectiveLimit())
		})
	}
}

func TestPagedResponse(t *testing.T) {
	t.Parallel()

	resp := PagedResponse[string]{
		Data:       []string{"a", "b"},
		NextCursor: "cursor123",
		HasMore:    true,
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(b, &parsed))
	assert.Equal(t, "cursor123", parsed["next_cursor"])
	assert.Equal(t, true, parsed["has_more"])
	assert.Len(t, parsed["data"], 2)
}

func TestPagedResponse_EmptyCursorOmitted(t *testing.T) {
	t.Parallel()

	resp := PagedResponse[string]{
		Data:    []string{},
		HasMore: false,
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(b, &parsed))
	_, hasCursor := parsed["next_cursor"]
	assert.False(t, hasCursor, "next_cursor should be omitted when empty")
}

func TestCreateRouteRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     CreateRouteRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid request",
			req:     CreateRouteRequest{Name: "my-route", Config: json.RawMessage(`{"path":"/"}`)},
			wantErr: false,
		},
		{
			name:    "name too short",
			req:     CreateRouteRequest{Name: "ab", Config: json.RawMessage(`{}`)},
			wantErr: true,
			errMsg:  "name",
		},
		{
			name:    "name too long",
			req:     CreateRouteRequest{Name: string(make([]byte, 101)), Config: json.RawMessage(`{}`)},
			wantErr: true,
			errMsg:  "name",
		},
		{
			name:    "missing config",
			req:     CreateRouteRequest{Name: "my-route"},
			wantErr: true,
			errMsg:  "config",
		},
		{
			name:    "invalid json config",
			req:     CreateRouteRequest{Name: "my-route", Config: json.RawMessage(`not json`)},
			wantErr: true,
			errMsg:  "config",
		},
		{
			name:    "empty name",
			req:     CreateRouteRequest{Name: "", Config: json.RawMessage(`{}`)},
			wantErr: true,
			errMsg:  "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateRouteRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     UpdateRouteRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     UpdateRouteRequest{Name: "updated", Config: json.RawMessage(`{}`)},
			wantErr: false,
		},
		{
			name:    "missing name",
			req:     UpdateRouteRequest{Config: json.RawMessage(`{}`)},
			wantErr: true,
		},
		{
			name:    "missing config",
			req:     UpdateRouteRequest{Name: "updated"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateBackendRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     CreateBackendRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     CreateBackendRequest{Name: "backend-1", Config: json.RawMessage(`{"url":"http://localhost"}`)},
			wantErr: false,
		},
		{
			name:    "name too short",
			req:     CreateBackendRequest{Name: "ab", Config: json.RawMessage(`{}`)},
			wantErr: true,
		},
		{
			name:    "missing config",
			req:     CreateBackendRequest{Name: "backend-1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateConsumerRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     CreateConsumerRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     CreateConsumerRequest{Name: "consumer-1"},
			wantErr: false,
		},
		{
			name:    "name too short",
			req:     CreateConsumerRequest{Name: "ab"},
			wantErr: true,
		},
		{
			name:    "name too long",
			req:     CreateConsumerRequest{Name: string(make([]byte, 101))},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateAIModelRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     CreateAIModelRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid",
			req:     CreateAIModelRequest{Provider: "openai", ModelName: "gpt-4"},
			wantErr: false,
		},
		{
			name:    "missing provider",
			req:     CreateAIModelRequest{ModelName: "gpt-4"},
			wantErr: true,
			errMsg:  "provider",
		},
		{
			name:    "missing model_name",
			req:     CreateAIModelRequest{Provider: "openai"},
			wantErr: true,
			errMsg:  "model_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateTokenBudgetRequest_Validate(t *testing.T) {
	t.Parallel()

	consumerID := uuid.New()

	tests := []struct {
		name    string
		req     CreateTokenBudgetRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid with consumer_id",
			req:     CreateTokenBudgetRequest{ConsumerID: &consumerID, Period: "day"},
			wantErr: false,
		},
		{
			name:    "valid with team",
			req:     CreateTokenBudgetRequest{Team: "engineering", Period: "month"},
			wantErr: false,
		},
		{
			name:    "invalid period",
			req:     CreateTokenBudgetRequest{ConsumerID: &consumerID, Period: "week"},
			wantErr: true,
			errMsg:  "period",
		},
		{
			name:    "missing consumer_id and team",
			req:     CreateTokenBudgetRequest{Period: "hour"},
			wantErr: true,
			errMsg:  "consumer_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWriteProblem(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	WriteProblem(w, http.StatusNotFound, "not-found", "route abc not found")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem domain.ProblemError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	assert.Equal(t, 404, problem.Status)
	assert.Contains(t, problem.Type, "not-found")
	assert.Equal(t, "route abc not found", problem.Detail)
}

func TestProblemFromError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "not found", err: domain.ErrNotFound, wantStatus: 404},
		{name: "already exists", err: domain.ErrAlreadyExists, wantStatus: 409},
		{name: "invalid config", err: domain.ErrInvalidConfig, wantStatus: 400},
		{name: "unauthorized", err: domain.ErrUnauthorized, wantStatus: 401},
		{name: "forbidden", err: domain.ErrForbidden, wantStatus: 403},
		{name: "wrapped not found", err: errors.Join(errors.New("ctx"), domain.ErrNotFound), wantStatus: 404},
		{name: "unknown error", err: errors.New("boom"), wantStatus: 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pe := ProblemFromError(tt.err)
			assert.Equal(t, tt.wantStatus, pe.Status)
		})
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]string{"id": "123"})

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "123", body["id"])
}

func TestRouteResponse_FromDomain(t *testing.T) {
	t.Parallel()

	route := domain.RouteConfig{
		ID:       uuid.New(),
		Name:     "test-route",
		Version:  3,
		Config:   json.RawMessage(`{"path":"/"}`),
		Checksum: "abc123",
		IsActive: true,
	}

	resp := RouteResponseFromDomain(route)
	assert.Equal(t, route.ID, resp.ID)
	assert.Equal(t, route.Name, resp.Name)
	assert.Equal(t, route.Version, resp.Version)
	assert.Equal(t, route.Checksum, resp.Checksum)
}

func TestBackendResponse_FromDomain(t *testing.T) {
	t.Parallel()

	backend := domain.BackendConfig{
		ID:   uuid.New(),
		Name: "test-backend",
	}

	resp := BackendResponseFromDomain(backend)
	assert.Equal(t, backend.ID, resp.ID)
	assert.Equal(t, backend.Name, resp.Name)
}

func TestConsumerResponse_FromDomain(t *testing.T) {
	t.Parallel()

	consumer := domain.Consumer{
		ID:       uuid.New(),
		Name:     "test-consumer",
		IsActive: true,
	}

	resp := ConsumerResponseFromDomain(consumer)
	assert.Equal(t, consumer.ID, resp.ID)
	assert.Equal(t, consumer.Name, resp.Name)
	assert.Equal(t, consumer.IsActive, resp.IsActive)
}

func TestAPIKeyResponse_FromDomain(t *testing.T) {
	t.Parallel()

	key := domain.APIKey{
		ID:         uuid.New(),
		ConsumerID: uuid.New(),
		KeyPrefix:  "abcd1234",
		IsActive:   true,
	}

	resp := APIKeyResponseFromDomain(key)
	assert.Equal(t, key.ID, resp.ID)
	assert.Equal(t, key.ConsumerID, resp.ConsumerID)
	assert.Equal(t, key.KeyPrefix, resp.KeyPrefix)
	// KeyHash should never appear in response
	assert.Empty(t, resp.KeyHash)
}
