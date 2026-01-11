package telemetry

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds OpenTelemetry configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string // e.g., "otlp-gateway-prod-ap-southeast-2.grafana.net"
	OTLPHeaders    map[string]string
	Enabled        bool
}

// Provider holds the initialized OTEL providers
type Provider struct {
	TracerProvider *trace.TracerProvider
	MeterProvider  *metric.MeterProvider
}

// Initialize sets up OpenTelemetry with Grafana Cloud
func Initialize(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		log.Println("📊 OpenTelemetry disabled")
		return nil, nil
	}

	log.Printf("📊 Initializing OpenTelemetry for %s...", cfg.ServiceName)

	// Create resource with service info
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
			attribute.String("service.namespace", "metamorph"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create trace exporter
	// Note: Grafana Cloud OTLP uses /otlp as the base path
	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
		otlptracehttp.WithURLPath("/otlp/v1/traces"),
		otlptracehttp.WithHeaders(cfg.OTLPHeaders),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create tracer provider
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter,
			trace.WithBatchTimeout(5*time.Second),
		),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()), // Sample 100% for now
	)

	// Set global tracer provider
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create metric exporter
	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetrichttp.WithURLPath("/otlp/v1/metrics"),
		otlpmetrichttp.WithHeaders(cfg.OTLPHeaders),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	// Create meter provider
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			metric.WithInterval(30*time.Second),
		)),
		metric.WithResource(res),
	)

	// Set global meter provider
	otel.SetMeterProvider(meterProvider)

	log.Printf("✓ OpenTelemetry initialized (endpoint: %s)", cfg.OTLPEndpoint)

	return &Provider{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
	}, nil
}

// Shutdown gracefully shuts down the OTEL providers
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}

	log.Println("📊 Shutting down OpenTelemetry...")

	if p.TracerProvider != nil {
		if err := p.TracerProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}

	if p.MeterProvider != nil {
		if err := p.MeterProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
	}

	return nil
}
