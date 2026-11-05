package integration

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentSessionApiPersistsAcrossServerRestart(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "api-restart.sqlite")
	first := startServerWithDatabase(t, "", databasePath)

	status, created := requestJSON[struct {
		Data struct {
			Session struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"session"`
		} `json:"data"`
	}](
		t,
		http.MethodPost,
		first.URL("/api/v1/agent/session/create"),
		`{"clientRequestId":"api-restart-session"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create status = %d", status)
	}
	first.Close()

	second := startServerWithDatabase(t, "", databasePath)
	defer second.Close()
	status, listed := requestJSON[struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}](
		t,
		http.MethodGet,
		second.URL("/api/v1/agent/session/list"),
		"",
	)
	if status != http.StatusOK || len(listed.Data) != 1 {
		t.Fatalf("list status=%d body=%#v", status, listed)
	}
	if listed.Data[0].ID != created.Data.Session.ID {
		t.Fatalf("session = %q, want %q", listed.Data[0].ID, created.Data.Session.ID)
	}
}

func TestAgentRunStreamReplaysAfterCursor(t *testing.T) {
	provider := startFakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(
			w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n\n",
		)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(
			w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"B\"}}]}\n\n",
		)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	server := startServer(t, provider.URL+"/v1")
	_, runID := createSessionAndRun(t, server, "cursor replay")
	response, err := http.Get(
		server.URL("/api/v1/agent/run/stream?runId=" + runID + "&after=0"),
	)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	cursor := ""
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			_ = response.Body.Close()
			t.Fatalf("read first delta: %v", readErr)
		}
		if strings.HasPrefix(line, "id: ") {
			cursor = strings.TrimSpace(strings.TrimPrefix(line, "id: "))
		}
		if strings.Contains(line, `"delta":"A"`) {
			break
		}
	}
	_ = response.Body.Close()
	if cursor == "" {
		t.Fatal("first delta had no cursor")
	}

	response, err = http.Get(
		server.URL(
			"/api/v1/agent/run/stream?runId=" + runID + "&after=" + cursor,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `"type":"start"`) ||
		strings.Contains(text, `"delta":"A"`) ||
		!strings.Contains(text, `"delta":"B"`) ||
		!strings.Contains(text, "data: [DONE]") {
		t.Fatalf("stream = %s", text)
	}
}

func TestAgentRunStreamReturnsCursorExpiredAfterJournalRemoval(t *testing.T) {
	provider := startFakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(
			w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n",
		)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	server := startServer(t, provider.URL+"/v1")
	_, runID := createSessionAndRun(t, server, "cursor expiry")
	waitForRunTerminal(t, server, runID)
	removeRunJournalAfterTerminal(t, server, runID)

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/agent/run/stream?runId="+runID+"&after=1"),
		"",
	)
	if status != http.StatusConflict ||
		failure.Error.Code != "AGENT_STREAM_CURSOR_EXPIRED" {
		t.Fatalf("status=%d error=%#v", status, failure.Error)
	}
}

var integrationRequestSequence atomic.Int64

func createSessionAndRun(
	t *testing.T,
	server *testServer,
	prompt string,
) (string, string) {
	t.Helper()
	sequence := integrationRequestSequence.Add(1)
	status, created := requestJSON[struct {
		Data struct {
			Session struct {
				ID string `json:"id"`
			} `json:"session"`
		} `json:"data"`
	}](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/session/create"),
		fmt.Sprintf(`{"clientRequestId":"session-%d"}`, sequence),
	)
	if status != http.StatusOK {
		t.Fatalf("create session status = %d", status)
	}
	status, started := requestJSON[struct {
		Data struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/message/send"),
		fmt.Sprintf(`{
			"sessionId":%q,
			"clientRequestId":"send-%d",
			"message":{
				"id":"message-%d",
				"role":"user",
				"parts":[{"type":"text","text":%q}]
			}
		}`, created.Data.Session.ID, sequence, sequence, prompt),
	)
	if status != http.StatusOK {
		t.Fatalf("start run status = %d", status)
	}
	return created.Data.Session.ID, started.Data.RunID
}

func waitForRunTerminal(
	t *testing.T,
	server *testServer,
	runID string,
) {
	t.Helper()
	response, err := http.Get(
		server.URL("/api/v1/agent/run/stream?runId=" + runID + "&after=0"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "data: [DONE]") {
		t.Fatalf("run did not terminate: %s", body)
	}
}

func removeRunJournalAfterTerminal(
	t *testing.T,
	server *testServer,
	runID string,
) {
	t.Helper()
	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/test/agent/run/remove-journal"),
		fmt.Sprintf(`{"runId":%q}`, runID),
	)
	if status != http.StatusOK {
		t.Fatalf("remove journal status=%d error=%#v", status, failure.Error)
	}
}
