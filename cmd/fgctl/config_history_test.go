package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigHistoryCmd_RequiresOrg(t *testing.T) {
	oldOrg := orgID
	orgID = ""
	defer func() { orgID = oldOrg }()

	rootCmd.SetArgs([]string{"config", "history"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--org")
}

func TestConfigRollbackCmd_RequiresToVersion(t *testing.T) {
	oldOrg := orgID
	orgID = "00000000-0000-0000-0000-000000000001"
	defer func() { orgID = oldOrg }()

	rootCmd.SetArgs([]string{"config", "rollback"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "to-version")
}

func TestConfigHistoryCmd_PrintsTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/config/versions", r.URL.Path)
		assert.Equal(t, "test-org-id", r.Header.Get("X-Organization-ID"))

		resp := configVersionListResponse{
			Data: []configVersionItem{
				{ID: "abc", Version: 2, Author: "admin", Description: "added route", CreatedAt: time.Now()},
				{ID: "def", Version: 1, Author: "system", Description: "initial", CreatedAt: time.Now()},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	oldURL := apiURL
	oldOrg := orgID
	apiURL = srv.URL
	orgID = "test-org-id"
	defer func() {
		apiURL = oldURL
		orgID = oldOrg
	}()

	rootCmd.SetArgs([]string{"config", "history"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestConfigRollbackCmd_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/config/rollback", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req map[string]int
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, 1, req["to_version"])

		resp := configVersionFull{
			ID:      "abc",
			Version: 3,
			Author:  "rollback:v1",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	oldURL := apiURL
	oldOrg := orgID
	apiURL = srv.URL
	orgID = "test-org-id"
	rollbackToVersion = 0 // Reset
	defer func() {
		apiURL = oldURL
		orgID = oldOrg
	}()

	rootCmd.SetArgs([]string{"config", "rollback", "--to-version=1"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{name: "short string", s: "hello", max: 10, want: "hello"},
		{name: "exact length", s: "hello", max: 5, want: "hello"},
		{name: "long string", s: "hello world", max: 8, want: "hello .."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, truncate(tt.s, tt.max))
		})
	}
}
