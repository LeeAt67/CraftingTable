// Package httpapi 提供 CraftingTable 共用的 HTTP 协议适配。
package httpapi

import (
	"encoding/json"
	"net/http"
)

type successEnvelope[T any] struct {
	Data T `json:"data"`
}

type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody 是 CraftingTable API 的稳定错误结构。
type ErrorBody struct {
	Code      string         `json:"code"`
	Kind      string         `json:"kind"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	RequestID string         `json:"requestId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// OK 写入成功响应信封。
func OK[T any](w http.ResponseWriter, status int, data T) {
	writeJSON(w, status, successEnvelope[T]{Data: data})
}

// Fail 写入错误响应信封，并在未显式提供时补上请求 ID。
func Fail(w http.ResponseWriter, r *http.Request, status int, body ErrorBody) {
	if body.RequestID == "" {
		body.RequestID = RequestID(r)
	}
	writeJSON(w, status, errorEnvelope{Error: body})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
