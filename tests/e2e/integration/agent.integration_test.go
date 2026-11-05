package integration

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAgentStreamRelaysDeltasWithoutLeakingKey(t *testing.T) {
	provider := startFakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer integration-secret" {
			t.Errorf("Authorization = %q", authorization)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		_, _ = io.WriteString(
			w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n",
		)
		flusher.Flush()
		_, _ = io.WriteString(
			w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"，Leeat\"}}]}\n\n",
		)
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	server := startServer(t, provider.URL+"/v1")

	response, err := http.Post(
		server.URL("/api/v1/agent/stream"),
		"application/json",
		strings.NewReader(`{"messages":[{"role":"user","content":"你好"}]}`),
	)
	if err != nil {
		t.Fatalf("send agent request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read agent response: %v", err)
	}

	text := string(body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.StatusCode, text)
	}
	for _, expected := range []string{
		"event: delta\ndata: {\"text\":\"你好\"}",
		"event: delta\ndata: {\"text\":\"，Leeat\"}",
		"event: done\ndata: {\"status\":\"completed\"}",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("response missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "integration-secret") {
		t.Fatal("response leaked the provider key")
	}
}

func TestAgentStreamCancelsProviderWhenClientDisconnects(t *testing.T) {
	providerCancelled := make(chan struct{}, 1)
	provider := startFakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(
			w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"开始\"}}]}\n\n",
		)
		flusher.Flush()

		select {
		case <-r.Context().Done():
			providerCancelled <- struct{}{}
		case <-time.After(3 * time.Second):
		}
	})
	server := startServer(t, provider.URL+"/v1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		server.URL("/api/v1/agent/stream"),
		strings.NewReader(`{"messages":[{"role":"user","content":"测试断开"}]}`),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}

	buffer := make([]byte, 128)
	if _, err := response.Body.Read(buffer); err != nil {
		t.Fatalf("read first stream event: %v", err)
	}
	cancel()
	_ = response.Body.Close()

	select {
	case <-providerCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("provider request was not cancelled after the client disconnected")
	}
}

func TestAgentStreamReturnsStableNotConfiguredError(t *testing.T) {
	server := startServer(t, "")

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/stream"),
		`{"messages":[{"role":"user","content":"你好"}]}`,
	)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	if failure.Error.Code != "AGENT_NOT_CONFIGURED" ||
		failure.Error.Kind != "server" ||
		failure.Error.Retryable {
		t.Fatalf("error = %#v", failure.Error)
	}
}
