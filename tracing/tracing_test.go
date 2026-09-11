package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func recorder(t *testing.T) (trace.Tracer, *tracetest.SpanRecorder) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp.Tracer("test"), sr
}

func TestOpReturnsTheValueAndNamesTheSpan(t *testing.T) {
	tr, sr := recorder(t)
	got, err := Op(context.Background(), tr, "db.Items", func(context.Context) (int, error) {
		return 42, nil
	}, attribute.String("db.system", "sqlite"))
	if err != nil || got != 42 {
		t.Fatalf("Op = (%d, %v), want (42, nil)", got, err)
	}
	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	s := spans[0]
	if s.Name() != "db.Items" {
		t.Errorf("span name = %q", s.Name())
	}
	if s.Status().Code == codes.Error {
		t.Error("a successful call marked the span as an error")
	}
	var found bool
	for _, a := range s.Attributes() {
		if string(a.Key) == "db.system" && a.Value.AsString() == "sqlite" {
			found = true
		}
	}
	if !found {
		t.Errorf("attributes not applied: %v", s.Attributes())
	}
}

// The reason this wrapper exists rather than each method starting its own
// span: the error has to land on the span, or a failed query looks identical
// to a slow one in a trace.
func TestOpRecordsTheErrorOnTheSpan(t *testing.T) {
	tr, sr := recorder(t)
	want := errors.New("no such table")
	_, err := Op(context.Background(), tr, "db.Broken", func(context.Context) (int, error) {
		return 0, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("Op swallowed the error: %v", err)
	}
	s := sr.Ended()[0]
	if s.Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", s.Status().Code)
	}
	if s.Status().Description != want.Error() {
		t.Errorf("span description = %q, want %q", s.Status().Description, want.Error())
	}
	if len(s.Events()) == 0 {
		t.Error("RecordError left no exception event, so the stack never reaches Tempo")
	}
}

func TestOpPassesAChildContext(t *testing.T) {
	tr, sr := recorder(t)
	// fn must receive the span's context, or anything it traces is a sibling
	// rather than a child.
	_, _ = Op(context.Background(), tr, "outer", func(ctx context.Context) (struct{}, error) {
		_, inner := tr.Start(ctx, "inner")
		inner.End()
		return struct{}{}, nil
	})
	spans := sr.Ended()
	if len(spans) != 2 {
		t.Fatalf("recorded %d spans, want 2", len(spans))
	}
	var outer, inner sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() == "outer" {
			outer = s
		} else {
			inner = s
		}
	}
	if inner.Parent().SpanID() != outer.SpanContext().SpanID() {
		t.Error("inner span is not a child of the wrapper's span")
	}
}

func TestDoRecordsErrorsToo(t *testing.T) {
	tr, sr := recorder(t)
	want := errors.New("flush failed")
	if err := Do(context.Background(), tr, "counter.Flush", func(context.Context) error {
		return want
	}); !errors.Is(err, want) {
		t.Fatalf("Do = %v, want the original error", err)
	}
	if got := sr.Ended()[0].Status().Code; got != codes.Error {
		t.Errorf("span status = %v, want Error", got)
	}
}
