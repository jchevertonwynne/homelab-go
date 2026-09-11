// Package tracing exports spans over OTLP/gRPC to Alloy's in-cluster
// receiver, which forwards them to Grafana Cloud Tempo. Always sampled —
// Alloy is the only hop and the traffic here is nowhere near worth trimming.
package tracing

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Init points the global TracerProvider at endpoint (host:port of Alloy's
// OTLP/gRPC receiver), plaintext because it never leaves the cluster network.
// An empty endpoint is a no-op, leaving the no-op tracer in place for local
// dev.
//
// Defer the returned func with a fresh context — the one passed here may
// already be cancelled by shutdown — or the last batch of spans is dropped.
func Init(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(serviceName),
	))
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	// W3C traceparent, so a trace arriving with one continues rather than
	// starting a new root.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// Middleware gives every request a server span and records the standard HTTP
// semantic-convention attributes.
func Middleware(serviceName string, h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, serviceName)
}

// Op runs fn inside a child span and records fn's error on it. The tracer is
// a parameter rather than package state: OpenTelemetry names a tracer after
// the package being instrumented, which this package cannot know.
//
// Wrapping a store method's body this way makes a slow query its own span,
// rather than time folded into the HTTP handler that called it.
func Op[T any](ctx context.Context, tracer trace.Tracer, span string, fn func(context.Context) (T, error), attrs ...attribute.KeyValue) (T, error) {
	ctx, s := tracer.Start(ctx, span, trace.WithAttributes(attrs...))
	defer s.End()
	result, err := fn(ctx)
	if err != nil {
		s.RecordError(err)
		s.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

// Do is Op for work that returns no value. It exists because Go has no void
// type parameter, so Op[struct{}] would make every call site carry a return
// value it has to discard.
func Do(ctx context.Context, tracer trace.Tracer, span string, fn func(context.Context) error, attrs ...attribute.KeyValue) error {
	_, err := Op(ctx, tracer, span, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	}, attrs...)
	return err
}
