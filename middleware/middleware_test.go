package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestRegisterGinMiddlewaresCreatesServerSpanAndMetrics(t *testing.T) {
	testTelemetry := installTestProviders(t)
	defer testTelemetry.restore()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterGinMiddlewares(router)

	router.GET("/users/:id", func(c *gin.Context) {
		if !trace.SpanContextFromContext(c.Request.Context()).IsValid() {
			t.Fatal("request context does not contain an active span")
		}

		c.Status(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}

	spans := testTelemetry.spanRecorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}

	span := spans[0]
	if span.Name() != "GET /users/:id" {
		t.Fatalf("span name = %q, want %q", span.Name(), "GET /users/:id")
	}
	if got := span.Attributes(); !hasAttribute(got, "http.request.method", "GET") {
		t.Fatalf("span attributes missing method: %#v", got)
	}
	if got := span.Attributes(); !hasAttribute(got, "http.route", "/users/:id") {
		t.Fatalf("span attributes missing route: %#v", got)
	}
	if got := span.Attributes(); !hasIntAttribute(got, "http.response.status_code", http.StatusCreated) {
		t.Fatalf("span attributes missing status code: %#v", got)
	}

	metrics := testTelemetry.collectMetrics(t)
	assertHasMetricSumPoint(t, metrics, "http.server.request.count", map[string]string{
		"http.request.method": "GET",
		"http.route":          "/users/:id",
	}, map[string]int64{
		"http.response.status_code": 201,
	}, 1)
	assertHasMetricHistogramPoint(t, metrics, "http.server.request.duration", map[string]string{
		"http.request.method": "GET",
		"http.route":          "/users/:id",
	}, map[string]int64{
		"http.response.status_code": 201,
	})
	assertHasMetricSumPoint(t, metrics, "http.server.active_requests", map[string]string{
		"http.request.method": "GET",
		"http.route":          "/users/:id",
	}, nil, 0)
}

func TestRegisterGinMiddlewaresRecordsActiveRequestsDuringHandler(t *testing.T) {
	testTelemetry := installTestProviders(t)
	defer testTelemetry.restore()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterGinMiddlewares(router)

	release := make(chan struct{})
	handlerEntered := make(chan struct{})

	router.POST("/jobs/:id", func(c *gin.Context) {
		close(handlerEntered)
		<-release
		c.Status(http.StatusAccepted)
	})

	req := httptest.NewRequest(http.MethodPost, "/jobs/7", nil)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		router.ServeHTTP(recorder, req)
	}()

	<-handlerEntered

	metrics := testTelemetry.collectMetrics(t)
	assertHasMetricSumPoint(t, metrics, "http.server.active_requests", map[string]string{
		"http.request.method": "POST",
		"http.route":          "/jobs/:id",
	}, nil, 1)

	close(release)
	<-done

	metrics = testTelemetry.collectMetrics(t)
	assertHasMetricSumPoint(t, metrics, "http.server.active_requests", map[string]string{
		"http.request.method": "POST",
		"http.route":          "/jobs/:id",
	}, nil, 0)
}

func TestRegisterGinMiddlewaresExtractsRemoteParent(t *testing.T) {
	testTelemetry := installTestProviders(t)
	defer testTelemetry.restore()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterGinMiddlewares(router)
	router.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	spans := testTelemetry.spanRecorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}

	parent := spans[0].Parent()
	if parent.TraceID().String() != "11111111111111111111111111111111" {
		t.Fatalf("parent trace id = %s, want %s", parent.TraceID().String(), "11111111111111111111111111111111")
	}
	if parent.SpanID().String() != "2222222222222222" {
		t.Fatalf("parent span id = %s, want %s", parent.SpanID().String(), "2222222222222222")
	}
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
	previousPropagator := otel.GetTextMapPropagator()

	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(tracerProvider)

	meterReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(meterReader))
	otel.SetMeterProvider(meterProvider)

	otel.SetTextMapPropagator(propagation.TraceContext{})

	return &testProviders{
		meterReader:  meterReader,
		spanRecorder: spanRecorder,
		restoreGlobal: func() {
			otel.SetTracerProvider(previousTracerProvider)
			otel.SetMeterProvider(previousMeterProvider)
			otel.SetTextMapPropagator(previousPropagator)
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

func assertHasMetricSumPoint(t *testing.T, metrics metricdata.ResourceMetrics, name string, stringLabels map[string]string, intLabels map[string]int64, want int64) {
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
				if matchesAttributes(point.Attributes, stringLabels, intLabels) && point.Value == want {
					return
				}
			}
		}
	}

	t.Fatalf("sum metric %q with labels %#v %#v and value %d not found", name, stringLabels, intLabels, want)
}

func assertHasMetricHistogramPoint(t *testing.T, metrics metricdata.ResourceMetrics, name string, stringLabels map[string]string, intLabels map[string]int64) {
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
				if matchesAttributes(point.Attributes, stringLabels, intLabels) && point.Count > 0 {
					return
				}
			}
		}
	}

	t.Fatalf("histogram metric %q with labels %#v %#v not found", name, stringLabels, intLabels)
}

func matchesAttributes(set attribute.Set, stringLabels map[string]string, intLabels map[string]int64) bool {
	for key, want := range stringLabels {
		value, ok := set.Value(attribute.Key(key))
		if !ok || value.AsString() != want {
			return false
		}
	}

	for key, want := range intLabels {
		value, ok := set.Value(attribute.Key(key))
		if !ok || value.AsInt64() != want {
			return false
		}
	}

	return true
}

func hasAttribute(attrs []attribute.KeyValue, key, want string) bool {
	for _, attr := range attrs {
		if string(attr.Key) == key && attr.Value.AsString() == want {
			return true
		}
	}

	return false
}

func hasIntAttribute(attrs []attribute.KeyValue, key string, want int) bool {
	for _, attr := range attrs {
		if string(attr.Key) == key && int(attr.Value.AsInt64()) == want {
			return true
		}
	}

	return false
}
