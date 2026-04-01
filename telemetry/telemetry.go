package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// ShutdownFunc flushes and stops the observability providers initialized by Init.
type ShutdownFunc func(ctx context.Context) error

// Init initializes telemetry providers and registers them as process globals.
func Init(cfg Config) (ShutdownFunc, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	res, err := newResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	shutdowns := make([]func(context.Context) error, 0, 2)

	tracerProvider, traceShutdown, err := newTracerProvider(context.Background(), cfg, res)
	if err != nil {
		return nil, err
	}
	shutdowns = append(shutdowns, traceShutdown)

	meterProvider, metricShutdown, err := newMeterProvider(context.Background(), cfg, res)
	if err != nil {
		return nil, shutdownAll(context.Background(), shutdowns, fmt.Errorf("init meter provider: %w", err))
	}
	shutdowns = append(shutdowns, metricShutdown)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		return shutdownAll(ctx, shutdowns, nil)
	}, nil
}

// MustInit initializes telemetry and panics if setup fails.
func MustInit(cfg Config) ShutdownFunc {
	shutdown, err := Init(cfg)
	if err != nil {
		panic(err)
	}

	return shutdown
}

func newResource(cfg Config) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentName(cfg.Environment),
			attribute.String("service.role", cfg.ServiceRole),
		),
	)
}

func newTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.TraceSamplingRate)),
	}

	if cfg.TracesEnabled {
		exporter, err := otlptracegrpc.New(
			ctx,
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("init trace exporter: %w", err)
		}

		options = append(options, sdktrace.WithBatcher(exporter))
	}

	provider := sdktrace.NewTracerProvider(options...)

	return provider, provider.Shutdown, nil
}

func newMeterProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdkmetric.MeterProvider, func(context.Context) error, error) {
	options := []sdkmetric.Option{
		sdkmetric.WithResource(res),
	}

	if cfg.MetricsEnabled {
		exporter, err := otlpmetricgrpc.New(
			ctx,
			otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("init metric exporter: %w", err)
		}

		options = append(options, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(cfg.MetricsExportInt),
		)))
	}

	provider := sdkmetric.NewMeterProvider(options...)

	return provider, provider.Shutdown, nil
}

func shutdownAll(ctx context.Context, shutdowns []func(context.Context) error, initial error) error {
	err := initial

	for i := len(shutdowns) - 1; i >= 0; i-- {
		err = errors.Join(err, shutdowns[i](ctx))
	}

	return err
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return errors.New("service name must not be empty")
	}
	if strings.TrimSpace(cfg.ServiceVersion) == "" {
		return errors.New("service version must not be empty")
	}
	if strings.TrimSpace(cfg.Environment) == "" {
		return errors.New("environment must not be empty")
	}
	if strings.TrimSpace(cfg.OTLPEndpoint) == "" {
		return errors.New("OTLP endpoint must not be empty")
	}
	if cfg.TraceSamplingRate < 0 || cfg.TraceSamplingRate > 1 {
		return fmt.Errorf("trace sampling rate must be between 0 and 1, got %v", cfg.TraceSamplingRate)
	}
	if cfg.MetricsExportInt <= 0 {
		return fmt.Errorf("metric export interval must be greater than 0, got %v", cfg.MetricsExportInt)
	}

	return nil
}
