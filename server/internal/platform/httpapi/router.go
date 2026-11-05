package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Router 组合 API 路由和通用 HTTP 中间件。
type Router struct {
	mux    *http.ServeMux
	logger *slog.Logger
}

// NewRouter 创建带健康检查的 HTTP 路由。
func NewRouter(logger *slog.Logger) *Router {
	router := &Router{
		mux:    http.NewServeMux(),
		logger: logger,
	}
	router.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		OK(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return router
}

// Handle 注册一个业务路由。
func (r *Router) Handle(pattern string, handler http.HandlerFunc) {
	r.mux.HandleFunc(pattern, handler)
}

// Handler 返回完成中间件组装后的处理器。
func (r *Router) Handler() http.Handler {
	var handler http.Handler = r.mux
	handler = r.recoverPanics(handler)
	handler = r.logRequests(handler)
	handler = r.withRequestID(handler)
	return handler
}

func (r *Router) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestID := newRequestID()
		ctx := contextWithRequestID(request.Context(), requestID)
		request = request.WithContext(ctx)
		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, request)
	})
}

func contextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func (r *Router) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				r.logger.Error(
					"http handler panic",
					"request_id", RequestID(request),
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				Fail(w, request, http.StatusInternalServerError, ErrorBody{
					Code:      "INTERNAL_ERROR",
					Kind:      "unknown",
					Message:   "请求失败",
					Retryable: true,
				})
			}
		}()
		next.ServeHTTP(w, request)
	})
}

func (r *Router) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, request)

		attributes := []any{
			"request_id", RequestID(request),
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		}
		if recorder.status >= http.StatusInternalServerError {
			r.logger.Error("http request failed", attributes...)
			return
		}
		r.logger.Info("http request", attributes...)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(data)
}

func (r *statusRecorder) Flush() {
	flusher, ok := r.ResponseWriter.(http.Flusher)
	if !ok {
		panic(fmt.Sprintf("response writer %T does not implement http.Flusher", r.ResponseWriter))
	}
	flusher.Flush()
}
