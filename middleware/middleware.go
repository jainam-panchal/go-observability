package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jainam-panchal/go-observability/internal/requestmeta"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/jainam-panchal/go-observability/middleware"

// RegisterGinMiddlewares registers the standard inbound tracing and request
// metrics middleware on a Gin router.
func RegisterGinMiddlewares(router gin.IRoutes) {
	router.Use(newGinMiddleware().Handler())
}

type ginMiddleware struct {
	tracer          trace.Tracer
	requestCount    metric.Int64Counter
	requestDuration metric.Float64Histogram
	activeRequests  metric.Int64UpDownCounter
}

func newGinMiddleware() *ginMiddleware {
	meter := otel.GetMeterProvider().Meter(instrumentationName)

	requestCount, _ := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("Total number of inbound HTTP requests."),
	)
	requestDuration, _ := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("Duration of inbound HTTP requests in seconds."),
		metric.WithUnit("s"),
	)
	activeRequests, _ := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("Current number of active inbound HTTP requests."),
	)

	return &ginMiddleware{
		tracer:          otel.GetTracerProvider().Tracer(instrumentationName),
		requestCount:    requestCount,
		requestDuration: requestDuration,
		activeRequests:  activeRequests,
	}
}

// Handler returns the Gin middleware function that instruments inbound HTTP
// requests with traces and metrics.
func (m *ginMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		route := routeTemplate(c)
		method := c.Request.Method
		start := time.Now()

		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagationHeaderCarrier(c.Request.Header))
		ctx, span := m.tracer.Start(
			ctx,
			fmt.Sprintf("%s %s", method, route),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(method),
				semconv.HTTPRoute(route),
			),
		)
		defer span.End()

		ctx = requestmeta.WithHTTPMetadata(ctx, method, route)
		c.Request = c.Request.WithContext(ctx)

		activeAttrs := []attribute.KeyValue{
			semconv.HTTPRequestMethodKey.String(method),
			semconv.HTTPRoute(route),
		}
		m.activeRequests.Add(ctx, 1, metric.WithAttributes(activeAttrs...))
		defer m.activeRequests.Add(ctx, -1, metric.WithAttributes(activeAttrs...))

		c.Next()

		statusCode := c.Writer.Status()
		durationSeconds := time.Since(start).Seconds()
		attrs := append(activeAttrs, semconv.HTTPResponseStatusCode(statusCode))

		span.SetName(fmt.Sprintf("%s %s", method, route))
		span.SetAttributes(semconv.HTTPResponseStatusCode(statusCode))
		if statusCode >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(statusCode))
		}

		m.requestCount.Add(ctx, 1, metric.WithAttributes(attrs...))
		m.requestDuration.Record(ctx, durationSeconds, metric.WithAttributes(attrs...))
	}
}

func routeTemplate(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}

	if c.Request.URL.Path != "" {
		return c.Request.URL.Path
	}

	return "/"
}

type propagationHeaderCarrier http.Header

func (h propagationHeaderCarrier) Get(key string) string {
	return http.Header(h).Get(key)
}

func (h propagationHeaderCarrier) Set(key, value string) {
	http.Header(h).Set(key, value)
}

func (h propagationHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for key := range h {
		keys = append(keys, key)
	}

	return keys
}
