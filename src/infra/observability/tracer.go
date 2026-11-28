package observability

import (
	"context"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	appErrors "internal-developer-platform/src/shared/errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

type Tracer struct {
	tracer trace.Tracer
}

var defaultTracer *Tracer

func InitTracer(serviceName string, otlpEndpoint ...string) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	ctx := context.Background()

	endpoint := ""
	if len(otlpEndpoint) > 0 {
		endpoint = otlpEndpoint[0]
	}
	if endpoint == "" {
		endpoint = "127.0.0.1:4318"
	}
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithTimeout(2 * time.Second),
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create otlp trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.DeploymentEnvironmentKey.String("dev"),
		),
	)
	if err != nil {
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(500*time.Millisecond)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	defaultTracer = &Tracer{
		tracer: tp.Tracer(serviceName),
	}

	shutdown := func(shutdownCtx context.Context) error {
		_ = tp.ForceFlush(shutdownCtx)
		return tp.Shutdown(shutdownCtx)
	}

	return tp, shutdown, nil
}

func GetTracer() *Tracer {
	return &Tracer{tracer: otel.Tracer("internal-developer-platform")}
}

func (t *Tracer) StartSpan(ctx context.Context, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, spanName, trace.WithAttributes(attrs...), trace.WithSpanKind(trace.SpanKindInternal))
}

func (t *Tracer) StartServerSpan(ctx context.Context, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, spanName, trace.WithAttributes(attrs...), trace.WithSpanKind(trace.SpanKindServer))
}

func (t *Tracer) StartClientSpan(ctx context.Context, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, spanName, trace.WithAttributes(attrs...), trace.WithSpanKind(trace.SpanKindClient))
}

func RecordError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	var appErr *appErrors.ApplicationError
	if stdErrors.As(err, &appErr) && appErr.StatusCode == 404 {
		span.SetStatus(codes.Unset, "")
		span.SetAttributes(
			attribute.Int("app.status_code", appErr.StatusCode),
			attribute.String("app.error_code", appErr.CanonicalCode),
			attribute.String("app.error_message", appErr.Message),
		)
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(
		attribute.String("error.type", fmt.Sprintf("%T", err)),
		attribute.String("error.message", err.Error()),
	)
	if stdErrors.As(err, &appErr) {
		span.SetAttributes(describeApplicationError(appErr)...)
	}
}

func describeApplicationError(appErr *appErrors.ApplicationError) []attribute.KeyValue {
	class := "server"
	if appErr.StatusCode >= 400 && appErr.StatusCode < 500 {
		class = "client"
	}
	return []attribute.KeyValue{
		attribute.Int("app.status_code", appErr.StatusCode),
		attribute.String("app.error_code", appErr.CanonicalCode),
		attribute.String("app.error_class", class),
		attribute.Bool("app.error_retryable", appErr.Retryable),
	}
}

func AnnotateSpan(ctx context.Context, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

func AddSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	trace.SpanFromContext(ctx).AddEvent(name, trace.WithAttributes(attrs...))
}

func CurrentSpan(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

func WithSpan(ctx context.Context, spanName string, fn func(ctx context.Context) error, attrs ...attribute.KeyValue) error {
	ctx, span := GetTracer().StartSpan(ctx, spanName, attrs...)
	defer span.End()
	if err := fn(ctx); err != nil {
		RecordError(span, err)
		return err
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

func RunTraced[T any](ctx context.Context, spanName string, fn func(ctx context.Context) (T, error), attrs ...attribute.KeyValue) (T, error) {
	ctx, span := GetTracer().StartSpan(ctx, spanName, attrs...)
	defer span.End()
	val, err := fn(ctx)
	if err != nil {
		RecordError(span, err)
		return val, err
	}
	span.SetStatus(codes.Ok, "")
	return val, nil
}

func WithClientSpan(ctx context.Context, spanName string, fn func(ctx context.Context) error, attrs ...attribute.KeyValue) error {
	ctx, span := GetTracer().StartClientSpan(ctx, spanName, attrs...)
	defer span.End()
	if err := fn(ctx); err != nil {
		RecordError(span, err)
		return err
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

func RunTracedClient[T any](ctx context.Context, spanName string, fn func(ctx context.Context) (T, error), attrs ...attribute.KeyValue) (T, error) {
	ctx, span := GetTracer().StartClientSpan(ctx, spanName, attrs...)
	defer span.End()
	val, err := fn(ctx)
	if err != nil {
		RecordError(span, err)
		return val, err
	}
	span.SetStatus(codes.Ok, "")
	return val, nil
}
