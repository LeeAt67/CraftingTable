package agent_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/leeat/craftingtable/server/testkit"
)

func TestOpenAICompatibleProviderUsesWireFieldNames(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Errorf("path = %q", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Errorf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		},
	))
	defer server.Close()

	provider := testkit.NewOpenAICompatible(
		testkit.OpenAICompatibleConfig{
			BaseURL: server.URL + "/v1",
			APIKey:  "test-key",
			Model:   "e2e-fake",
		},
	)
	body, err := provider.Stream(context.Background(), testkit.ModelRequest{
		Messages: []testkit.ProviderMessage{
			{Role: "user", Content: "你好"},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer body.Close()

	if requestBody["messages"] == nil {
		t.Fatalf("request body = %#v", requestBody)
	}
	messages := requestBody["messages"].([]any)
	message := messages[0].(map[string]any)
	if message["role"] != "user" || message["content"] != "你好" {
		t.Fatalf("message wire shape = %#v", message)
	}
	if _, exists := message["Role"]; exists {
		t.Fatalf("message contains Go field name: %#v", message)
	}
}

func TestOpenAICompatibleProviderSendsToolsAndFollowUpMessages(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Errorf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		},
	))
	defer server.Close()

	provider := testkit.NewOpenAICompatible(
		testkit.OpenAICompatibleConfig{
			BaseURL: server.URL + "/v1",
			APIKey:  "test-key",
			Model:   "tool-model",
		},
	)
	body, err := provider.Stream(context.Background(), testkit.ModelRequest{
		Messages: []testkit.ProviderMessage{
			{Role: "user", Content: "找一个任务"},
			{
				Role: "assistant",
				ToolCalls: []testkit.ProviderToolCall{{
					ID:   "call-1",
					Type: "function",
					Function: testkit.ProviderFunctionCall{
						Name:      "today_search",
						Arguments: json.RawMessage(`{"query":"任务"}`),
					},
				}},
			},
			{
				Role:       "tool",
				Content:    `{"content":[{"type":"json","json":[]}]}`,
				ToolCallID: "call-1",
			},
		},
		Tools: []testkit.ProviderToolDefinition{{
			Type: "function",
			Function: testkit.ProviderFunctionTool{
				Name:        "today_search",
				Description: "Search today.",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		}},
		ToolChoice: "auto",
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer body.Close()

	if requestBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %#v", requestBody["tool_choice"])
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", requestBody["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Fatalf("tool wire shape = %#v", tool)
	}
	messages := requestBody["messages"].([]any)
	assistant := messages[1].(map[string]any)
	if assistant["tool_calls"] == nil {
		t.Fatalf("assistant message = %#v", assistant)
	}
	toolMessage := messages[2].(map[string]any)
	if toolMessage["role"] != "tool" || toolMessage["tool_call_id"] != "call-1" {
		t.Fatalf("tool message = %#v", toolMessage)
	}
}

func TestOpenAICompatibleProviderOmitsToolFieldsForTextOnlyRequest(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Errorf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		},
	))
	defer server.Close()

	provider := testkit.NewOpenAICompatible(
		testkit.OpenAICompatibleConfig{
			BaseURL: server.URL + "/v1",
			APIKey:  "test-key",
			Model:   "text-model",
		},
	)
	body, err := provider.Stream(context.Background(), testkit.ModelRequest{
		Messages: []testkit.ProviderMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer body.Close()

	if _, exists := requestBody["tools"]; exists {
		t.Fatalf("text-only request included tools: %#v", requestBody)
	}
	if _, exists := requestBody["tool_choice"]; exists {
		t.Fatalf("text-only request included tool_choice: %#v", requestBody)
	}
}
