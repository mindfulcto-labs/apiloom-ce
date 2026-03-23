package ai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatRequest_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
	}{
		{
			name: "minimal request",
			json: `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`,
		},
		{
			name: "full request with all optional fields",
			json: `{"model":"gpt-4","messages":[{"role":"system","content":"You are helpful"},{"role":"user","content":"Hi"}],"temperature":0.7,"max_tokens":100,"stream":true,"top_p":0.9,"frequency_penalty":0.5,"presence_penalty":0.5,"stop":["END"],"user":"user-123","stream_options":{"include_usage":true}}`,
		},
		{
			name: "request with tools",
			json: `{"model":"gpt-4","messages":[{"role":"user","content":"What is the weather?"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather","parameters":{"type":"object"}}}],"tool_choice":"auto"}`,
		},
		{
			name: "zero temperature is preserved",
			json: `{"model":"gpt-4","messages":[{"role":"user","content":"Hi"}],"temperature":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var req ChatRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			require.NoError(t, err, "unmarshal should succeed")

			out, err := json.Marshal(req)
			require.NoError(t, err, "marshal should succeed")

			// Unmarshal both to maps and compare to ignore field order.
			var original, roundTripped map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.json), &original))
			require.NoError(t, json.Unmarshal(out, &roundTripped))
			assert.Equal(t, original, roundTripped)
		})
	}
}

func TestChatResponse_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	input := `{
		"id": "chatcmpl-abc123",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "gpt-4",
		"choices": [{
			"index": 0,
			"message": {"role": "assistant", "content": "Hello!"},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`

	var resp ChatResponse
	err := json.Unmarshal([]byte(input), &resp)
	require.NoError(t, err)

	assert.Equal(t, "chatcmpl-abc123", resp.ID)
	assert.Equal(t, "chat.completion", resp.Object)
	assert.Equal(t, int64(1700000000), resp.Created)
	assert.Equal(t, "gpt-4", resp.Model)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.Equal(t, "Hello!", stringVal(resp.Choices[0].Message.Content))
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
	assert.Equal(t, int64(10), resp.Usage.PromptTokens)
	assert.Equal(t, int64(5), resp.Usage.CompletionTokens)
	assert.Equal(t, int64(15), resp.Usage.TotalTokens)

	// Round-trip
	out, err := json.Marshal(resp)
	require.NoError(t, err)

	var roundTripped ChatResponse
	require.NoError(t, json.Unmarshal(out, &roundTripped))
	assert.Equal(t, resp, roundTripped)
}

func TestChatMessage_Roles(t *testing.T) {
	t.Parallel()

	roles := []string{"system", "user", "assistant", "tool"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			msg := ChatMessage{Role: role, Content: "test"}
			data, err := json.Marshal(msg)
			require.NoError(t, err)

			var decoded ChatMessage
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, role, decoded.Role)
		})
	}
}

func TestChatMessage_ToolCalls(t *testing.T) {
	t.Parallel()

	input := `{
		"role": "assistant",
		"content": null,
		"tool_calls": [{
			"id": "call_abc",
			"type": "function",
			"function": {"name": "get_weather", "arguments": "{\"city\":\"NYC\"}"}
		}]
	}`

	var msg ChatMessage
	err := json.Unmarshal([]byte(input), &msg)
	require.NoError(t, err)
	require.Len(t, msg.ToolCalls, 1)
	assert.Equal(t, "call_abc", msg.ToolCalls[0].ID)
	assert.Equal(t, "function", msg.ToolCalls[0].Type)
	assert.Equal(t, "get_weather", msg.ToolCalls[0].Function.Name)
	assert.Equal(t, `{"city":"NYC"}`, msg.ToolCalls[0].Function.Arguments)
}

func TestChatMessage_ToolCallID(t *testing.T) {
	t.Parallel()

	input := `{
		"role": "tool",
		"content": "72 degrees",
		"tool_call_id": "call_abc"
	}`

	var msg ChatMessage
	err := json.Unmarshal([]byte(input), &msg)
	require.NoError(t, err)
	assert.Equal(t, "tool", msg.Role)
	assert.Equal(t, "call_abc", msg.ToolCallID)
}

func TestUsage_Fields(t *testing.T) {
	t.Parallel()

	u := Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	data, err := json.Marshal(u)
	require.NoError(t, err)

	var decoded Usage
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, u, decoded)
}

func TestModel_Fields(t *testing.T) {
	t.Parallel()

	m := Model{
		Provider:            "openai",
		ModelName:           "gpt-4",
		DisplayName:         "GPT-4",
		InputPricePerToken:  0.00003,
		OutputPricePerToken: 0.00006,
		Capabilities:        []string{"chat", "function_calling"},
		IsActive:            true,
		Weight:              10,
	}

	data, err := json.Marshal(m)
	require.NoError(t, err)

	var decoded Model
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, m, decoded)
}

func TestStreamChunk_DeltaField(t *testing.T) {
	t.Parallel()

	input := `{
		"id": "chatcmpl-abc123",
		"object": "chat.completion.chunk",
		"created": 1700000000,
		"model": "gpt-4",
		"choices": [{
			"index": 0,
			"delta": {"role": "assistant", "content": "Hello"},
			"finish_reason": null
		}]
	}`

	var chunk StreamChunk
	err := json.Unmarshal([]byte(input), &chunk)
	require.NoError(t, err)
	assert.Equal(t, "chat.completion.chunk", chunk.Object)
	require.Len(t, chunk.Choices, 1)
	assert.Equal(t, "Hello", stringVal(chunk.Choices[0].Delta.Content))
}

// stringVal extracts string from interface{} Content field.
func stringVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
