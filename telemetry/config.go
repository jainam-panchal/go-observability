package telemetry

import (
	"os"
	"strconv"
	"strings"
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
