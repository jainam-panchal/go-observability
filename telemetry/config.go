package telemetry

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServiceName     = "unknown-service"
	defaultServiceVersion  = "1.0.0"
	defaultEnvironment     = "development"
	defaultOTLPEndpoint    = "localhost:4317"
	defaultLogLevel        = "info"
	defaultTraceSampleRate = 1.0
	defaultTracesEnabled   = true
	defaultMetricsEnabled  = true
	defaultMetricExportInt = 10 * time.Second
)

// Config contains environment-driven settings used to bootstrap observability.
type Config struct {
	ServiceName       string
	ServiceVersion    string
	Environment       string
	OTLPEndpoint      string
	LogLevel          string
	TraceSamplingRate float64
	TracesEnabled     bool
	MetricsEnabled    bool
	MetricsExportInt  time.Duration
}

// DefaultConfig returns the baseline configuration for services that have not
// overridden any observability settings.
func DefaultConfig() Config {
	return Config{
		ServiceName:       defaultServiceName,
		ServiceVersion:    defaultServiceVersion,
		Environment:       defaultEnvironment,
		OTLPEndpoint:      defaultOTLPEndpoint,
		LogLevel:          defaultLogLevel,
		TraceSamplingRate: defaultTraceSampleRate,
		TracesEnabled:     defaultTracesEnabled,
		MetricsEnabled:    defaultMetricsEnabled,
		MetricsExportInt:  defaultMetricExportInt,
	}
}

// LoadConfigFromEnv reads observability settings from environment variables and
// falls back to DefaultConfig when values are missing or invalid.
func LoadConfigFromEnv() Config {
	cfg := DefaultConfig()

	cfg.ServiceName = loadString("OTEL_SERVICE_NAME", cfg.ServiceName)
	cfg.ServiceVersion = loadString("OTEL_SERVICE_VERSION", cfg.ServiceVersion)
	cfg.Environment = loadString("DEPLOYMENT_ENVIRONMENT", loadString("ENVIRONMENT", cfg.Environment))
	cfg.OTLPEndpoint = loadString("OTEL_EXPORTER_OTLP_ENDPOINT", cfg.OTLPEndpoint)
	cfg.LogLevel = loadString("LOG_LEVEL", cfg.LogLevel)
	cfg.TraceSamplingRate = loadFloat("OTEL_TRACE_SAMPLING_RATE", cfg.TraceSamplingRate)
	cfg.TracesEnabled = loadBool("OTEL_TRACES_ENABLED", cfg.TracesEnabled)
	cfg.MetricsEnabled = loadBool("OTEL_METRICS_ENABLED", cfg.MetricsEnabled)
	cfg.MetricsExportInt = loadDuration("OTEL_METRIC_EXPORT_INTERVAL", cfg.MetricsExportInt)

	return cfg
}

func loadString(key string, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}

func loadBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}

	return parsed
}

func loadFloat(key string, fallback float64) float64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func loadDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	value = strings.TrimSpace(value)

	if millis, err := strconv.Atoi(value); err == nil {
		parsed := time.Duration(millis) * time.Millisecond
		if parsed > 0 {
			return parsed
		}
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err == nil && parsed > 0 {
		return parsed
	}

	return fallback
}
