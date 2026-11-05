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
	body, err := provider.Stream(context.Background(), []testkit.ProviderMessage{
		{Role: "user", Content: "你好"},
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
