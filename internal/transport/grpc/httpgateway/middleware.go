package httpgateway

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/nglogic/go-application-guide/internal/app"
)

// HandlerWithTraceID wraps handler with middleware generating trace id for each request.
// If trace id is present in headers, it will be preserved. We try to discover incoming trace ids based on w3 standard:
// https://www.w3.org/TR/trace-context/#trace-context-http-headers-format
func HandlerWithTraceID(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("trace-id")
		if traceID == "" {
			traceID = fmt.Sprintf("%x", uuid.New())
		}

		ctx := app.CtxWithTraceID(r.Context(), traceID)
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}

// HandlerWithLogCtx wraps handler with middleware adding request information to context for logging.
func HandlerWithLogCtx(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = app.CtxWithLogField(ctx, "http.url", r.URL.String())
		ctx = app.CtxWithLogField(ctx, "http.method", r.Method)

		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}
