package flex

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Raw(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"key":"value"}`)
	c := Config{Raw: raw}
	assert.JSONEq(t, `{"key":"value"}`, string(c.Raw))
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	schema := ConfigSchema(json.RawMessage(`{
		"type": "object",
		"required": ["name", "timeout"],
		"properties": {
			"name": {"type": "string"},
			"timeout": {"type": "integer"},
			"enabled": {"type": "boolean"}
		}
	}`))

	tests := []struct {
		name    string
		config  json.RawMessage
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config with all fields",
			config:  json.RawMessage(`{"name":"test","timeout":30,"enabled":true}`),
			wantErr: false,
		},
		{
			name:    "valid config with required fields only",
			config:  json.RawMessage(`{"name":"test","timeout":30}`),
			wantErr: false,
		},
		{
			name:    "missing required field name",
			config:  json.RawMessage(`{"timeout":30}`),
			wantErr: true,
			errMsg:  "name",
		},
		{
			name:    "missing required field timeout",
			config:  json.RawMessage(`{"name":"test"}`),
			wantErr: true,
			errMsg:  "timeout",
		},
		{
			name:    "wrong type for name",
			config:  json.RawMessage(`{"name":123,"timeout":30}`),
			wantErr: true,
			errMsg:  "name",
		},
		{
			name:    "wrong type for timeout",
			config:  json.RawMessage(`{"name":"test","timeout":"slow"}`),
			wantErr: true,
			errMsg:  "timeout",
		},
		{
			name:    "wrong type for boolean",
			config:  json.RawMessage(`{"name":"test","timeout":30,"enabled":"yes"}`),
			wantErr: true,
			errMsg:  "enabled",
		},
		{
			name:    "invalid JSON",
			config:  json.RawMessage(`{broken`),
			wantErr: true,
		},
		{
			name:    "empty object with required fields",
			config:  json.RawMessage(`{}`),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateConfig(schema, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateConfig_InvalidSchema(t *testing.T) {
	t.Parallel()

	schema := ConfigSchema(json.RawMessage(`{broken`))
	err := ValidateConfig(schema, json.RawMessage(`{"key":"value"}`))
	require.Error(t, err)
}

func TestValidateConfig_NoRequiredFields(t *testing.T) {
	t.Parallel()

	schema := ConfigSchema(json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		}
	}`))

	// Should pass with empty object when no required fields
	err := ValidateConfig(schema, json.RawMessage(`{}`))
	require.NoError(t, err)

	// Should pass with correct type
	err = ValidateConfig(schema, json.RawMessage(`{"name":"hello"}`))
	require.NoError(t, err)

	// Should fail with wrong type
	err = ValidateConfig(schema, json.RawMessage(`{"name":123}`))
	require.Error(t, err)
}

func TestValidateConfig_ArrayType(t *testing.T) {
	t.Parallel()

	schema := ConfigSchema(json.RawMessage(`{
		"type": "object",
		"properties": {
			"tags": {"type": "array"}
		}
	}`))

	err := ValidateConfig(schema, json.RawMessage(`{"tags":["a","b"]}`))
	require.NoError(t, err)

	err = ValidateConfig(schema, json.RawMessage(`{"tags":"not-array"}`))
	require.Error(t, err)
}

func TestValidateConfig_NumberType(t *testing.T) {
	t.Parallel()

	schema := ConfigSchema(json.RawMessage(`{
		"type": "object",
		"properties": {
			"rate": {"type": "number"}
		}
	}`))

	err := ValidateConfig(schema, json.RawMessage(`{"rate":1.5}`))
	require.NoError(t, err)

	err = ValidateConfig(schema, json.RawMessage(`{"rate":10}`))
	require.NoError(t, err)

	err = ValidateConfig(schema, json.RawMessage(`{"rate":"fast"}`))
	require.Error(t, err)
}
