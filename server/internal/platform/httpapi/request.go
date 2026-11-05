package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const (
	maxJSONBodyBytes = 1 << 20
	requestIDKey     = contextKey("request_id")
	requestIDHeader  = "X-Request-ID"
)

type contextKey string

var (
	// ErrInvalidJSON 表示请求体不是单个合法 JSON 对象。
	ErrInvalidJSON = errors.New("invalid JSON request body")
	// ErrBodyTooLarge 表示请求体超过 API 允许的上限。
	ErrBodyTooLarge = errors.New("JSON request body too large")
)

// ReadJSON 读取有大小上限且只包含一个 JSON 值的请求体。
func ReadJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return ErrBodyTooLarge
		}
		return errors.Join(ErrInvalidJSON, err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidJSON
	}
	return nil
}

// RequestID 返回当前请求的关联标识。
func RequestID(r *http.Request) string {
	value, _ := r.Context().Value(requestIDKey).(string)
	return value
}

func newRequestID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}
