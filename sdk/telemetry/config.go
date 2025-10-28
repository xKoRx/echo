package telemetry

import "go.opentelemetry.io/otel/attribute"

// Config contiene la configuración para el cliente de telemetría
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string

	// OTLP Collector endpoints
	// Traces y métricas pueden vivir en endpoints/puertos distintos
	OTLPEndpoint        string // Compat: si se setea, aplica a ambos si los específicos están vacíos
	OTLPTracesEndpoint  string
	OTLPMetricsEndpoint string

	// Atributos comunes a todos los logs, métricas y trazas
	CommonAttributes []attribute.KeyValue

	// Habilitar/deshabilitar componentes
	EnableLogs    bool
	EnableMetrics bool
	EnableTraces  bool
}

// DefaultConfig retorna una configuración con valores por defecto
func DefaultConfig(serviceName, environment string) Config {
	return Config{
		ServiceName:         serviceName,
		ServiceVersion:      "0.0.1",
		Environment:         environment,
		OTLPEndpoint:        "192.168.31.60:4317",
		OTLPTracesEndpoint:  "",
		OTLPMetricsEndpoint: "",
		EnableLogs:          true,
		EnableMetrics:       true,
		EnableTraces:        true,
		CommonAttributes:    []attribute.KeyValue{},
	}
}

// Option es una función que modifica la configuración
type Option func(*Config)

// WithVersion establece la versión del servicio
func WithVersion(version string) Option {
	return func(c *Config) {
		c.ServiceVersion = version
	}
}

// WithOTLPEndpoint establece el endpoint del collector
func WithOTLPEndpoint(endpoint string) Option {
	return func(c *Config) {
		c.OTLPEndpoint = endpoint
	}
}

// WithTracesEndpoint establece endpoint específico para trazas
func WithTracesEndpoint(endpoint string) Option {
	return func(c *Config) { c.OTLPTracesEndpoint = endpoint }
}

// WithMetricsEndpoint establece endpoint específico para métricas
func WithMetricsEndpoint(endpoint string) Option {
	return func(c *Config) { c.OTLPMetricsEndpoint = endpoint }
}

// WithCommonAttributes añade atributos comunes
func WithCommonAttributes(attrs ...attribute.KeyValue) Option {
	return func(c *Config) {
		c.CommonAttributes = append(c.CommonAttributes, attrs...)
	}
}

// WithLogsDisabled deshabilita logs
func WithLogsDisabled() Option {
	return func(c *Config) {
		c.EnableLogs = false
	}
}

// WithMetricsDisabled deshabilita métricas
func WithMetricsDisabled() Option {
	return func(c *Config) {
		c.EnableMetrics = false
	}
}

// WithTracesDisabled deshabilita trazas
func WithTracesDisabled() Option {
	return func(c *Config) {
		c.EnableTraces = false
	}
}
