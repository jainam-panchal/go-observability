package telemetry

import "testing"

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	if cfg.ServiceName != defaultServiceName {
		t.Fatalf("ServiceName = %q, want %q", cfg.ServiceName, defaultServiceName)
	}
	if cfg.ServiceVersion != defaultServiceVersion {
		t.Fatalf("ServiceVersion = %q, want %q", cfg.ServiceVersion, defaultServiceVersion)
	}
	if cfg.Environment != defaultEnvironment {
		t.Fatalf("Environment = %q, want %q", cfg.Environment, defaultEnvironment)
	}
	if cfg.OTLPEndpoint != defaultOTLPEndpoint {
		t.Fatalf("OTLPEndpoint = %q, want %q", cfg.OTLPEndpoint, defaultOTLPEndpoint)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, defaultLogLevel)
	}
	if cfg.TraceSamplingRate != defaultTraceSampleRate {
		t.Fatalf("TraceSamplingRate = %v, want %v", cfg.TraceSamplingRate, defaultTraceSampleRate)
	}
	if cfg.TracesEnabled != defaultTracesEnabled {
		t.Fatalf("TracesEnabled = %v, want %v", cfg.TracesEnabled, defaultTracesEnabled)
	}
	if cfg.MetricsEnabled != defaultMetricsEnabled {
		t.Fatalf("MetricsEnabled = %v, want %v", cfg.MetricsEnabled, defaultMetricsEnabled)
	}
	if cfg.MetricsExportInt != defaultMetricExportInt {
		t.Fatalf("MetricsExportInt = %v, want %v", cfg.MetricsExportInt, defaultMetricExportInt)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "checkout-api")
	t.Setenv("OTEL_SERVICE_VERSION", "2.3.4")
	t.Setenv("DEPLOYMENT_ENVIRONMENT", "production")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("OTEL_TRACE_SAMPLING_RATE", "0.25")
	t.Setenv("OTEL_TRACES_ENABLED", "false")
	t.Setenv("OTEL_METRICS_ENABLED", "true")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "3000")

	cfg := LoadConfigFromEnv()

	if cfg.ServiceName != "checkout-api" {
		t.Fatalf("ServiceName = %q, want %q", cfg.ServiceName, "checkout-api")
	}
	if cfg.ServiceVersion != "2.3.4" {
		t.Fatalf("ServiceVersion = %q, want %q", cfg.ServiceVersion, "2.3.4")
	}
	if cfg.Environment != "production" {
		t.Fatalf("Environment = %q, want %q", cfg.Environment, "production")
	}
	if cfg.OTLPEndpoint != "otel-collector:4317" {
		t.Fatalf("OTLPEndpoint = %q, want %q", cfg.OTLPEndpoint, "otel-collector:4317")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.TraceSamplingRate != 0.25 {
		t.Fatalf("TraceSamplingRate = %v, want %v", cfg.TraceSamplingRate, 0.25)
	}
	if cfg.TracesEnabled {
		t.Fatal("TracesEnabled = true, want false")
	}
	if !cfg.MetricsEnabled {
		t.Fatal("MetricsEnabled = false, want true")
	}
	if cfg.MetricsExportInt.String() != "3s" {
		t.Fatalf("MetricsExportInt = %v, want %v", cfg.MetricsExportInt, "3s")
	}
}

func TestLoadConfigFromEnvFallsBackForInvalidValues(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "   ")
	t.Setenv("OTEL_TRACE_SAMPLING_RATE", "not-a-number")
	t.Setenv("OTEL_TRACES_ENABLED", "not-a-bool")
	t.Setenv("OTEL_METRICS_ENABLED", "not-a-bool")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "invalid")
	t.Setenv("ENVIRONMENT", "stage")

	cfg := LoadConfigFromEnv()

	if cfg.ServiceName != defaultServiceName {
		t.Fatalf("ServiceName = %q, want %q", cfg.ServiceName, defaultServiceName)
	}
	if cfg.TraceSamplingRate != defaultTraceSampleRate {
		t.Fatalf("TraceSamplingRate = %v, want %v", cfg.TraceSamplingRate, defaultTraceSampleRate)
	}
	if cfg.TracesEnabled != defaultTracesEnabled {
		t.Fatalf("TracesEnabled = %v, want %v", cfg.TracesEnabled, defaultTracesEnabled)
	}
	if cfg.MetricsEnabled != defaultMetricsEnabled {
		t.Fatalf("MetricsEnabled = %v, want %v", cfg.MetricsEnabled, defaultMetricsEnabled)
	}
	if cfg.Environment != "stage" {
		t.Fatalf("Environment = %q, want %q", cfg.Environment, "stage")
	}
	if cfg.MetricsExportInt != defaultMetricExportInt {
		t.Fatalf("MetricsExportInt = %v, want %v", cfg.MetricsExportInt, defaultMetricExportInt)
	}
}

func TestLoadConfigFromEnvPrefersDeploymentEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "stage")
	t.Setenv("DEPLOYMENT_ENVIRONMENT", "production")

	cfg := LoadConfigFromEnv()

	if cfg.Environment != "production" {
		t.Fatalf("Environment = %q, want %q", cfg.Environment, "production")
	}
}
