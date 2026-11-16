package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Tracer struct {
	tracer trace.Tracer
}

var defaultTracer *Tracer

func InitTracer(serviceName string) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	defaultTracer = &Tracer{
		tracer: tp.Tracer(serviceName),
	}

	shutdown := func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}

	return tp, shutdown, nil
}

func GetTracer() *Tracer {
	if defaultTracer == nil {
		defaultTracer = &Tracer{tracer: otel.Tracer("internal-developer-platform")}
	}
	return defaultTracer
}

func (t *Tracer) StartSpan(ctx context.Context, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, spanName, trace.WithAttributes(attrs...), trace.WithSpanKind(trace.SpanKindInternal))
}

func (t *Tracer) StartServerSpan(ctx context.Context, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, spanName, trace.WithAttributes(attrs...), trace.WithSpanKind(trace.SpanKindServer))
}

func RecordError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(
		attribute.String("error.type", fmt.Sprintf("%T", err)),
		attribute.String("error.message", err.Error()),
	)
}

func CurrentSpan(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}
