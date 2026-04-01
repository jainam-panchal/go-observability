package telemetry

import (
	"context"
	"runtime"
	"syscall"

	"go.opentelemetry.io/otel/metric"
)

const runtimeInstrumentationName = "github.com/jainam-panchal/go-observability/telemetry/runtime"

func startRuntimeMetrics(_ context.Context, provider metric.MeterProvider, cfg Config) (func(context.Context) error, error) {
	if !cfg.MetricsEnabled || !cfg.RuntimeMetrics {
		return func(context.Context) error { return nil }, nil
	}

	meter := provider.Meter(runtimeInstrumentationName)

	cpuSeconds, err := meter.Float64ObservableCounter(
		"process.cpu.seconds",
		metric.WithDescription("Total user+system CPU time consumed by the current process."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	residentMemory, err := meter.Float64ObservableGauge(
		"process.resident.memory.bytes",
		metric.WithDescription("Approximate memory footprint of the current process in bytes."),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	goroutines, err := meter.Int64ObservableGauge(
		"go.goroutines",
		metric.WithDescription("Current number of goroutines."),
	)
	if err != nil {
		return nil, err
	}

	gcCycles, err := meter.Int64ObservableCounter(
		"go.gc.cycles",
		metric.WithDescription("Total completed Go GC cycles."),
	)
	if err != nil {
		return nil, err
	}

	gcLastPause, err := meter.Float64ObservableGauge(
		"go.gc.last_pause.seconds",
		metric.WithDescription("Last observed Go GC pause duration in seconds."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	registration, err := meter.RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			var usage syscall.Rusage
			if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err == nil {
				utime := float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1e6
				stime := float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1e6
				observer.ObserveFloat64(cpuSeconds, utime+stime)
			}

			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)

			observer.ObserveFloat64(residentMemory, float64(mem.Sys))
			observer.ObserveInt64(goroutines, int64(runtime.NumGoroutine()))
			observer.ObserveInt64(gcCycles, int64(mem.NumGC))

			var pauseSeconds float64
			if mem.NumGC > 0 {
				idx := (mem.NumGC - 1) % uint32(len(mem.PauseNs))
				pauseSeconds = float64(mem.PauseNs[idx]) / 1e9
			}
			observer.ObserveFloat64(gcLastPause, pauseSeconds)

			return nil
		},
		cpuSeconds,
		residentMemory,
		goroutines,
		gcCycles,
		gcLastPause,
	)
	if err != nil {
		return nil, err
	}

	return func(context.Context) error {
		return registration.Unregister()
	}, nil
}
