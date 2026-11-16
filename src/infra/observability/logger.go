package observability

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

type Field struct {
	Key   string
	Value interface{}
}

func String(key string, val string) Field {
	return Field{Key: key, Value: val}
}

func Int(key string, val int) Field {
	return Field{Key: key, Value: val}
}

func Int64(key string, val int64) Field {
	return Field{Key: key, Value: val}
}

func Bool(key string, val bool) Field {
	return Field{Key: key, Value: val}
}

func Any(key string, val interface{}) Field {
	return Field{Key: key, Value: val}
}

type CanonicalLogEntry struct {
	Timestamp  string                 `json:"timestamp"`
	Level      string                 `json:"level"`
	Service    string                 `json:"service"`
	Message    string                 `json:"message"`
	TenantID   string                 `json:"tenant_id,omitempty"`
	TraceID    string                 `json:"trace_id,omitempty"`
	SpanID     string                 `json:"span_id,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

type Logger struct {
	mu      sync.Mutex
	writer  io.Writer
	service string
}

var (
	globalLogger *Logger
	loggerOnce   sync.Once
)

type tenantContextKey struct{}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

func ExtractTenantID(ctx context.Context) string {
	if val, ok := ctx.Value(tenantContextKey{}).(string); ok {
		return val
	}
	return ""
}

func GetLogger() *Logger {
	loggerOnce.Do(func() {
		globalLogger = NewLogger(os.Stdout, "internal-developer-platform")
	})
	return globalLogger
}

func NewLogger(writer io.Writer, service string) *Logger {
	return &Logger{
		writer:  writer,
		service: service,
	}
}

func (l *Logger) emit(ctx context.Context, level LogLevel, msg string, fields ...Field) {
	entry := CanonicalLogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     string(level),
		Service:   l.service,
		Message:   msg,
		TenantID:  ExtractTenantID(ctx),
	}

	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		entry.TraceID = span.SpanContext().TraceID().String()
		entry.SpanID = span.SpanContext().SpanID().String()
	}

	if len(fields) > 0 {
		attrs := make(map[string]interface{}, len(fields))
		for _, f := range fields {
			attrs[f.Key] = f.Value
		}
		entry.Attributes = attrs
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}

	payload = append(payload, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.writer.Write(payload)
}

func (l *Logger) Debug(ctx context.Context, msg string, fields ...Field) {
	l.emit(ctx, LogLevelDebug, msg, fields...)
}

func (l *Logger) Info(ctx context.Context, msg string, fields ...Field) {
	l.emit(ctx, LogLevelInfo, msg, fields...)
}

func (l *Logger) Warn(ctx context.Context, msg string, fields ...Field) {
	l.emit(ctx, LogLevelWarn, msg, fields...)
}

func (l *Logger) Error(ctx context.Context, msg string, fields ...Field) {
	l.emit(ctx, LogLevelError, msg, fields...)
}
