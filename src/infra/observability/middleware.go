package observability

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

type statusLoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusLoggingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusLoggingResponseWriter) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func IngressTelemetryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		propagator := otel.GetTextMapPropagator()
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		tenantID := r.Header.Get("X-Tenant-Id")
		if tenantID != "" {
			ctx = WithTenantID(ctx, tenantID)
		}

		ctx, span := GetTracer().StartServerSpan(ctx, r.Method+" "+r.URL.Path,
			attribute.String("http.method", r.Method),
			attribute.String("http.target", r.URL.Path),
			attribute.String("tenant_id", tenantID),
		)
		defer span.End()

		sc := span.SpanContext()
		if sc.IsValid() {
			w.Header().Set("X-Trace-Id", sc.TraceID().String())
			w.Header().Set("X-Span-Id", sc.SpanID().String())
		}

		wrappedWriter := &statusLoggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrappedWriter, r.WithContext(ctx))

		durationMs := time.Since(start).Milliseconds()
		span.SetAttributes(
			attribute.Int("http.status_code", wrappedWriter.statusCode),
			attribute.Int64("duration_ms", durationMs),
		)

		logger := GetLogger()
		logFields := []Field{
			String("method", r.Method),
			String("path", r.URL.Path),
			Int("status_code", wrappedWriter.statusCode),
			Int64("duration_ms", durationMs),
		}

		if wrappedWriter.statusCode >= 500 {
			span.SetStatus(codes.Error, http.StatusText(wrappedWriter.statusCode))
			logger.Error(ctx, "HTTP 5xx Server Error", logFields...)
		} else if wrappedWriter.statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(wrappedWriter.statusCode))
			logger.Warn(ctx, "HTTP 4xx Client Request Failure", logFields...)
		} else {
			logger.Info(ctx, "HTTP Ingress Request Handled", logFields...)
		}
	})
}
