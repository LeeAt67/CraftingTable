package integration

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentRunStreamSendsHeartbeatWithoutWaitingForOutput(t *testing.T) {
	release := make(chan struct{})
	provider := startFakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(
			w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n",
		)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	server := startServerWithEnvironment(
		t,
		provider.URL+"/v1",
		filepath.Join(t.TempDir(), "stream-heartbeat.sqlite"),
		map[string]string{
			"CRAFTINGTABLE_AGENT_STREAM_HEARTBEAT": "20ms",
		},
	)
	sessionID, runID := createSessionAndRun(t, server, "heartbeat")
	defer close(release)
	defer func() {
		_, _ = requestJSON[struct {
			Data any `json:"data"`
		}](
			t,
			http.MethodPost,
			server.URL("/api/v1/agent/run/cancel"),
			fmt.Sprintf(`{"sessionId":%q,"runId":%q}`, sessionID, runID),
		)
	}()

	requestCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodGet,
		server.URL("/api/v1/agent/run/stream?runId="+runID+"&after=0"),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	var stream strings.Builder
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("stream ended without heartbeat: %v\n%s", readErr, stream.String())
		}
		stream.WriteString(line)
		if strings.TrimSpace(line) == ": keep-alive" {
			return
		}
		if requestCtx.Err() != nil {
			t.Fatalf("stream did not emit a heartbeat:\n%s", stream.String())
		}
	}
}
