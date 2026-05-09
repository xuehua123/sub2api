package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTrimAnthropicCompatResponsesInputToLatestTurnKeepsToolResultWithFollowupMessage(t *testing.T) {
	t.Parallel()

	items := []apicompat.ResponsesInputItem{
		{
			Type: "message",
			Role: "user",
			Content: json.RawMessage(
				`[{"type":"input_text","text":"first"}]`,
			),
		},
		{
			Type:      "function_call",
			CallID:    "toolu_123",
			Name:      "Bash",
			Arguments: `{"command":"ls"}`,
		},
		{
			Type:   "function_call_output",
			CallID: "toolu_123",
			Output: "ok",
		},
		{
			Type: "message",
			Role: "user",
			Content: json.RawMessage(
				`[{"type":"input_text","text":"continue"}]`,
			),
		},
	}
	raw, err := json.Marshal(items)
	require.NoError(t, err)

	req := &apicompat.ResponsesRequest{Input: raw}
	trimAnthropicCompatResponsesInputToLatestTurn(req)

	var got []apicompat.ResponsesInputItem
	require.NoError(t, json.Unmarshal(req.Input, &got))
	require.Len(t, got, 3)
	require.Equal(t, "function_call", got[0].Type)
	require.Equal(t, "toolu_123", got[0].CallID)
	require.Equal(t, "function_call_output", got[1].Type)
	require.Equal(t, "toolu_123", got[1].CallID)
	require.Equal(t, "message", got[2].Type)
	require.Equal(t, "user", got[2].Role)
}

func TestForwardAsAnthropicCompatContinuationKeepsToolResultWithFollowupMessage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAICompatSSECompletedResponse("resp_first_tool", "gpt-5.5"),
		openAICompatSSECompletedResponse("resp_second_tool", "gpt-5.5"),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
		},
	}

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"list files"}],"stream":false}`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	require.Equal(t, "resp_first_tool", firstResult.ResponseID)

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"list files"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_123","name":"Bash","input":{"command":"ls"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":"ok"},{"type":"text","text":"continue"}]}],"tools":[{"name":"Bash","description":"run shell","input_schema":{"type":"object","properties":{"command":{"type":"string"}}}}],"stream":false}`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "resp_first_tool", gjson.GetBytes(upstream.bodies[1], "previous_response_id").String())
	require.Equal(t, "toolu_123", gjson.GetBytes(upstream.bodies[1], `input.#(type=="function_call").call_id`).String())
	require.Equal(t, "toolu_123", gjson.GetBytes(upstream.bodies[1], `input.#(type=="function_call_output").call_id`).String())
	require.Equal(t, "continue", gjson.GetBytes(upstream.bodies[1], `input.#(role=="user").content.0.text`).String())
}
