package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type runHandleResponse struct {
	RunID string `json:"runId"`
}

type runHandleEnvelope struct {
	Data runHandleResponse `json:"data"`
}

type sessionCreateEnvelope struct {
	Data struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	} `json:"data"`
}

func TestAgentSendIdempotencyErrorsExposeRunState(t *testing.T) {
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
	server := startServerWithDatabase(
		t,
		provider.URL+"/v1",
		filepath.Join(t.TempDir(), "idempotency-errors.sqlite"),
	)
	status, created := requestJSON[sessionCreateEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/session/create"),
		`{"clientRequestId":"idempotency-session"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create session status = %d", status)
	}

	sendBody := mustJSON(t, map[string]any{
		"sessionId":       created.Data.Session.ID,
		"clientRequestId": "idempotency-send",
		"message": map[string]any{
			"id":   "idempotency-user",
			"role": "user",
			"parts": []map[string]string{
				{"type": "text", "text": "original"},
			},
		},
	})
	status, started := requestJSON[runHandleEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/message/send"),
		sendBody,
	)
	if status != http.StatusOK {
		t.Fatalf("start run status = %d", status)
	}

	status, busy := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/message/send"),
		sendBody,
	)
	if status != http.StatusConflict ||
		busy.Error.Code != "AGENT_SESSION_BUSY" ||
		busy.Error.Details["runId"] != started.Data.RunID ||
		busy.Error.Details["status"] != "running" {
		t.Fatalf("busy status=%d error=%#v", status, busy.Error)
	}

	conflictBody := mustJSON(t, map[string]any{
		"sessionId":       created.Data.Session.ID,
		"clientRequestId": "idempotency-send",
		"message": map[string]any{
			"id":   "different-user",
			"role": "user",
			"parts": []map[string]string{
				{"type": "text", "text": "different"},
			},
		},
	})
	status, conflict := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/message/send"),
		conflictBody,
	)
	if status != http.StatusConflict ||
		conflict.Error.Code != "AGENT_REQUEST_ID_CONFLICT" ||
		conflict.Error.Details["clientRequestId"] != "idempotency-send" {
		t.Fatalf("conflict status=%d error=%#v", status, conflict.Error)
	}

	close(release)
	waitForRunTerminal(t, server, started.Data.RunID)

	status, processed := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/message/send"),
		sendBody,
	)
	if status != http.StatusConflict ||
		processed.Error.Code != "AGENT_REQUEST_ALREADY_PROCESSED" ||
		processed.Error.Details["runId"] != started.Data.RunID ||
		processed.Error.Details["status"] != "completed" {
		t.Fatalf("processed status=%d error=%#v", status, processed.Error)
	}
}

func TestAgentSendRejectsMessageWhosePartsExceedTotalLimit(t *testing.T) {
	server := startServer(t, "")
	status, created := requestJSON[sessionCreateEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/session/create"),
		`{"clientRequestId":"total-limit-session"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create session status = %d", status)
	}

	parts := make([]map[string]string, 0, 20)
	for range 20 {
		parts = append(parts, map[string]string{
			"type": "text",
			"text": strings.Repeat("a", 6_000),
		})
	}
	body := mustJSON(t, map[string]any{
		"sessionId":       created.Data.Session.ID,
		"clientRequestId": "total-limit-send",
		"message": map[string]any{
			"id":    "total-limit-user",
			"role":  "user",
			"parts": parts,
		},
	})
	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/agent/message/send"),
		body,
	)
	if status != http.StatusBadRequest || failure.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("status=%d error=%#v", status, failure.Error)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(body)
}
