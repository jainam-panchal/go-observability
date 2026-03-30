package worker

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestWorkerStartJobCreatesRootSpanWithoutIncomingContext(t *testing.T) {
	testTelemetry := installTestProviders(t)
	defer testTelemetry.restore()

	ctx, finish := StartJob(context.Background(), "email.send")
	if !trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("job context missing active span")
	}
	finish(nil)

	spans := testTelemetry.spanRecorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}

	if spans[0].Parent().IsValid() {
		t.Fatalf("parent span = %s, want invalid", spans[0].Parent().SpanID())
	}
	if spans[0].Name() != "job email.send" {
		t.Fatalf("span name = %q, want %q", spans[0].Name(), "job email.send")
	}
	if !hasStringAttribute(spans[0].Attributes(), "job.name", "email.send") {
		t.Fatalf("job.name missing from span attributes: %#v", spans[0].Attributes())
	}
}

func TestWorkerStartJobUsesIncomingContextAsParent(t *testing.T) {
	testTelemetry := installTestProviders(t)
	defer testTelemetry.restore()

	tracer := otel.GetTracerProvider().Tracer("worker-test")
	parentCtx, parentSpan := tracer.Start(context.Background(), "parent")

	ctx, finish := StartJob(parentCtx, "report.generate")
	if !trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("job context missing active span")
	}
	finish(nil)
	parentSpan.End()

	assertHasChildSpan(t, testTelemetry.spanRecorder.Ended(), parentSpan.SpanContext(), "job report.generate")
}

func TestWorkerStartJobEmitsWorkerMetrics(t *testing.T) {
	testTelemetry := installTestProviders(t)
	defer testTelemetry.restore()

	_, finishSuccess := StartJob(context.Background(), "thumbnail.render")
	time.Sleep(5 * time.Millisecond)
	finishSuccess(nil)

	_, finishFailure := StartJob(context.Background(), "thumbnail.render")
	time.Sleep(5 * time.Millisecond)
	finishFailure(assertErr{})

	metrics := testTelemetry.collectMetrics(t)
	assertHasMetricSumPoint(t, metrics, "worker.job.started", map[string]string{
		"job.name": "thumbnail.render",
	}, 2)
	assertHasMetricSumPoint(t, metrics, "worker.job.completed", map[string]string{
		"job.name":   "thumbnail.render",
		"job.status": "success",
	}, 1)
	assertHasMetricSumPoint(t, metrics, "worker.job.completed", map[string]string{
		"job.name":   "thumbnail.render",
		"job.status": "error",
	}, 1)
	assertHasMetricHistogramPoint(t, metrics, "worker.job.duration", map[string]string{
		"job.name": "thumbnail.render",
	}, 2)
}

type assertErr struct{}

func (assertErr) Error() string {
	return "boom"
}

type testProviders struct {
	meterReader   *sdkmetric.ManualReader
	spanRecorder  *tracetest.SpanRecorder
	restoreGlobal func()
}

func installTestProviders(t *testing.T) *testProviders {
	t.Helper()

	previousTracerProvider := otel.GetTracerProvider()
	previousMeterProvider := otel.GetMeterProvider()

	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(tracerProvider)

	meterReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(meterReader))
	otel.SetMeterProvider(meterProvider)

	return &testProviders{
		meterReader:  meterReader,
		spanRecorder: spanRecorder,
		restoreGlobal: func() {
			otel.SetTracerProvider(previousTracerProvider)
			otel.SetMeterProvider(previousMeterProvider)
		},
	}
}

func (p *testProviders) restore() {
	p.restoreGlobal()
}

func (p *testProviders) collectMetrics(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()

	var metrics metricdata.ResourceMetrics
	if err := p.meterReader.Collect(context.Background(), &metrics); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	return metrics
}

func assertHasChildSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, parent trace.SpanContext, name string) {
	t.Helper()

	for _, span := range spans {
		if span.Name() == name && span.Parent().SpanID() == parent.SpanID() && span.Parent().TraceID() == parent.TraceID() {
			return
		}
	}

	t.Fatalf("child span %q with parent trace=%s span=%s not found", name, parent.TraceID(), parent.SpanID())
}

func assertHasMetricSumPoint(t *testing.T, metrics metricdata.ResourceMetrics, name string, labels map[string]string, want int64) {
	t.Helper()

	for _, scopeMetrics := range metrics.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			if metric.Name != name {
				continue
			}

			sum, ok := metric.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric %q is not an int64 sum", name)
			}

			for _, point := range sum.DataPoints {
				if matchesAttributes(point.Attributes, labels) && point.Value == want {
					return
				}
			}
		}
	}

	t.Fatalf("sum metric %q with labels %#v and value %d not found", name, labels, want)
}

func assertHasMetricHistogramPoint(t *testing.T, metrics metricdata.ResourceMetrics, name string, labels map[string]string, minCount uint64) {
	t.Helper()

	for _, scopeMetrics := range metrics.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			if metric.Name != name {
				continue
			}

			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("metric %q is not a float64 histogram", name)
			}

			for _, point := range histogram.DataPoints {
				if matchesAttributes(point.Attributes, labels) && point.Count >= minCount {
					return
				}
			}
		}
	}

	t.Fatalf("histogram metric %q with labels %#v and count >= %d not found", name, labels, minCount)
}

func matchesAttributes(set attribute.Set, labels map[string]string) bool {
	for key, want := range labels {
		value, ok := set.Value(attribute.Key(key))
		if !ok || value.AsString() != want {
			return false
		}
	}

	return true
}

func hasStringAttribute(attrs []attribute.KeyValue, key, want string) bool {
	for _, attr := range attrs {
		if string(attr.Key) == key && attr.Value.AsString() == want {
			return true
		}
	}

	return false
}
