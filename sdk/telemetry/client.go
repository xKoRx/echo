package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
)

// Client es el cliente unificado de telemetría para echo
type Client struct {
	config Config
	logger *slog.Logger
	tracer trace.Tracer
	meter  metric.Meter
	
	// Providers (para shutdown)
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	
	// Instrumentos de métricas comunes
	counters   map[string]metric.Int64Counter
	histograms map[string]metric.Float64Histogram
	
	// Metric bundles
	echoMetrics *metricbundle.EchoMetrics
}

// New crea una nueva instancia del cliente de telemetría
func New(ctx context.Context, serviceName, environment string, opts ...Option) (*Client, error) {
	cfg := DefaultConfig(serviceName, environment)
	for _, opt := range opts {
		opt(&cfg)
	}
	
	client := &Client{
		config:     cfg,
		counters:   make(map[string]metric.Int64Counter),
		histograms: make(map[string]metric.Float64Histogram),
	}
	
	// Crear resource común
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
		resource.WithAttributes(cfg.CommonAttributes...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}
	
	// Inicializar logs
	if cfg.EnableLogs {
		client.initLogs()
	}
	
	// Inicializar trazas
	if cfg.EnableTraces {
		if err := client.initTraces(ctx, res); err != nil {
			return nil, fmt.Errorf("failed to init traces: %w", err)
		}
	}
	
	// Inicializar métricas
	if cfg.EnableMetrics {
		if err := client.initMetrics(ctx, res); err != nil {
			return nil, fmt.Errorf("failed to init metrics: %w", err)
		}
	}
	
	return client, nil
}

func (c *Client) initLogs() {
	// Parsear nivel de log desde config
	var level slog.Level
	switch c.config.LogLevel {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // Default
	}
	
	// Usar slog estándar con JSON handler
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	c.logger = slog.New(handler)
}

func (c *Client) initTraces(ctx context.Context, res *resource.Resource) error {
    endpoint := c.config.OTLPTracesEndpoint
    if endpoint == "" {
        endpoint = c.config.OTLPEndpoint
    }
    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint(endpoint),
        otlptracegrpc.WithInsecure(),
    )
	if err != nil {
		return err
	}
	
	c.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	
	otel.SetTracerProvider(c.tracerProvider)
	c.tracer = c.tracerProvider.Tracer(c.config.ServiceName)
	
	return nil
}

func (c *Client) initMetrics(ctx context.Context, res *resource.Resource) error {
    endpoint := c.config.OTLPMetricsEndpoint
    if endpoint == "" {
        endpoint = c.config.OTLPEndpoint
    }
    exporter, err := otlpmetricgrpc.New(ctx,
        otlpmetricgrpc.WithEndpoint(endpoint),
        otlpmetricgrpc.WithInsecure(),
    )
	if err != nil {
		return err
	}
	
	c.meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)
	
	otel.SetMeterProvider(c.meterProvider)
	c.meter = c.meterProvider.Meter(c.config.ServiceName)
	
	// Inicializar bundles de métricas
	echoMetrics, err := metricbundle.NewEchoMetrics(c.meter)
	if err != nil {
		return fmt.Errorf("failed to init echo metrics: %w", err)
	}
	c.echoMetrics = echoMetrics
	
	return nil
}

// Shutdown cierra todos los exporters y libera recursos
func (c *Client) Shutdown(ctx context.Context) error {
	var errs []error
	
	if c.tracerProvider != nil {
		if err := c.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	
	if c.meterProvider != nil {
		if err := c.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	
	return nil
}

// GetOrCreateCounter obtiene o crea un contador
func (c *Client) GetOrCreateCounter(name, description string) (metric.Int64Counter, error) {
	if counter, exists := c.counters[name]; exists {
		return counter, nil
	}
	
	counter, err := c.meter.Int64Counter(name,
		metric.WithDescription(description),
	)
	if err != nil {
		return nil, err
	}
	
	c.counters[name] = counter
	return counter, nil
}

// GetOrCreateHistogram obtiene o crea un histograma
func (c *Client) GetOrCreateHistogram(name, description string) (metric.Float64Histogram, error) {
	if histogram, exists := c.histograms[name]; exists {
		return histogram, nil
	}
	
	histogram, err := c.meter.Float64Histogram(name,
		metric.WithDescription(description),
	)
	if err != nil {
		return nil, err
	}
	
	c.histograms[name] = histogram
	return histogram, nil
}

// ExtractAttributes extrae atributos del contexto (helpers)
func ExtractAttributes(ctx context.Context) []attribute.KeyValue {
	// TODO: Implementar extracción de atributos desde context
	// Por ahora retorna vacío
	return []attribute.KeyValue{}
}

// EchoMetrics retorna el bundle de métricas de Echo.
//
// Example:
//
//	metrics := client.EchoMetrics()
//	metrics.RecordIntentReceived(ctx,
//	    semconv.Echo.TradeID.String("01HKQV8Y..."),
//	    semconv.Echo.Symbol.String("XAUUSD"),
//	)
func (c *Client) EchoMetrics() *metricbundle.EchoMetrics {
	return c.echoMetrics
}

