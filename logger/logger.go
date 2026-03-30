package logger

import (
	"context"
	"fmt"
	"os"

	"github.com/jainam-panchal/go-observability/telemetry"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a JSON logger configured for container-friendly stdout logging.
func New(cfg telemetry.Config, options ...zap.Option) (*zap.Logger, error) {
	level, err := zap.ParseAtomicLevel(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", cfg.LogLevel, err)
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.Lock(os.Stdout),
		level,
	)

	return newWithCore(cfg, core, options...)
}

func newWithCore(cfg telemetry.Config, core zapcore.Core, options ...zap.Option) (*zap.Logger, error) {
	if core == nil {
		return nil, fmt.Errorf("nil zap core")
	}

	loggerOptions := []zap.Option{
		zap.AddCaller(),
		zap.AddStacktrace(zap.ErrorLevel),
	}
	loggerOptions = append(loggerOptions, options...)

	base := zap.New(core, loggerOptions...)

	return base.With(
		zap.String("service", cfg.ServiceName),
		zap.String("service_version", cfg.ServiceVersion),
		zap.String("deployment_environment", cfg.Environment),
	), nil
}

// MustNew creates a logger and panics if construction fails.
func MustNew(cfg telemetry.Config, options ...zap.Option) *zap.Logger {
	logger, err := New(cfg, options...)
	if err != nil {
		panic(err)
	}

	return logger
}

// WithContext enriches the provided logger with trace fields when the context
// carries an active span.
func WithContext(ctx context.Context, base *zap.Logger) *zap.Logger {
	if base == nil {
		base = zap.L()
	}

	fields := traceFields(ctx)
	if len(fields) == 0 {
		return base
	}

	return base.With(fields...)
}

// L returns the global logger enriched with trace context when available.
func L(ctx context.Context) *zap.Logger {
	return WithContext(ctx, zap.L())
}

func traceFields(ctx context.Context) []zap.Field {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return nil
	}

	return []zap.Field{
		zap.String("trace_id", spanContext.TraceID().String()),
		zap.String("span_id", spanContext.SpanID().String()),
	}
}
