package metricbundle

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MetricsClient define la interfaz para recopilar métricas a través de OpenTelemetry.
// Esta interfaz abstrae las operaciones fundamentales para registrar métricas de diferentes tipos:
// contadores, histogramas y gauges.
type MetricsClient interface {
	// Counter crea o retorna un contador existente.
	// Los contadores son monótonos y solo permiten incrementos.
	Counter(name, description string) metric.Int64Counter

	// Gauge crea o retorna un gauge existente.
	// Los gauges representan valores que pueden subir y bajar en el tiempo.
	Gauge(name, description string) metric.Float64ObservableGauge

	// Histogram crea o retorna un histograma existente.
	// Los histogramas capturan distribuciones de valores (como latencias).
	Histogram(name, description string) metric.Float64Histogram

	// RecordCounter incrementa un contador con un valor específico.
	RecordCounter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue)

	// RecordHistogram registra un valor en un histograma.
	RecordHistogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue)

	// RegisterGauge registra un gauge con una función callback que reportará su valor actual.
	RegisterGauge(ctx context.Context, name string, callback func(ctx context.Context) float64, attrs ...attribute.KeyValue) error

	// Shutdown cierra el cliente y libera los recursos asociados.
	Shutdown(ctx context.Context) error

	// IsInitialized retorna si el cliente está correctamente inicializado.
	IsInitialized() bool

	// Error retorna el error de inicialización si ocurrió alguno.
	Error() error
}

// BaseMetrics contiene contadores y histogramas comunes a todos los bundles de métricas.
// Proporciona funcionalidad base para registrar resultados y duraciones de operaciones,
// y sirve como componente fundamental para todos los bundles específicos de dominio.
type BaseMetrics struct {
	// client es la implementación de MetricsClient para registrar métricas.
	client MetricsClient

	// entity representa el tipo de entidad que este bundle monitorea (candle, http, document, etc.).
	entity string

	// namespace es el prefijo principal de todas las métricas (e.g., "trading", "app").
	namespace string

	// ResultCounter contabiliza los resultados de operaciones (éxitos, fallos, etc.).
	ResultCounter metric.Int64Counter

	// DurationHistogram mide la distribución de tiempos de ejecución en segundos.
	DurationHistogram metric.Float64Histogram
}

// NewBaseMetrics crea una nueva instancia de BaseMetrics con los contadores e histogramas básicos.
// Cada bundle específico utilizará esta base y añadirá sus propias métricas especializadas.
//
// Parámetros:
//   - client: implementación de MetricsClient para registrar métricas
//   - namespace: espacio de nombres para agrupar métricas (ej. "trading")
//   - entity: tipo de entidad que este bundle monitorea (ej. "http", "candle")
func NewBaseMetrics(client MetricsClient, namespace, entity string) *BaseMetrics {
	metricName := func(metricType string) string {
		return strings.Join([]string{namespace, entity, metricType}, ".")
	}

	return &BaseMetrics{
		client:    client,
		entity:    entity,
		namespace: namespace,
		ResultCounter: client.Counter(
			metricName("result"),
			"Results of operations for "+entity+" labeled by status, service, etc.",
		),
		DurationHistogram: client.Histogram(
			metricName("duration"),
			"Duration of operations for "+entity+" in seconds.",
		),
	}
}

// RecordResult incrementa el contador de resultados para un evento específico.
// Debe utilizarse para registrar el resultado de cualquier operación importante.
//
// Atributos comunes a incluir:
//   - semconv.Metrics.Status.String("success"/"error")
//   - semconv.Metrics.Result.String("success"/"failure"/"partial")
//   - semconv.Metrics.Service.String("nombre-servicio")
func (bm *BaseMetrics) RecordResult(ctx context.Context, attrs ...attribute.KeyValue) {
	// Usar el cliente para registrar el contador permite que se adjunten
	// automáticamente los atributos Common + Metric desde el contexto
	// sin depender del paquete telemetry (evita dependencia cíclica).
	name := MetricName(bm.namespace, bm.entity, "result")
	bm.client.RecordCounter(ctx, name, 1, attrs...)
}

// StartDurationTimer mide la duración de una operación y retorna una función
// que debe llamarse al finalizar la operación para registrar el tiempo transcurrido.
//
// Ejemplo de uso:
//
//	done := metrics.StartDurationTimer(ctx,
//	    semconv.Metrics.Service.String("api"),
//	    semconv.Metrics.Action.String("process_order")
//	)
//	// Realizar operación...
//	done() // Registra automáticamente la duración
func (bm *BaseMetrics) StartDurationTimer(ctx context.Context, attrs ...attribute.KeyValue) func() {
	start := time.Now()
	return func() {
		duration := time.Since(start).Seconds()
		name := MetricName(bm.namespace, bm.entity, "duration")
		bm.client.RecordHistogram(ctx, name, duration, attrs...)
	}
}

// FormattedMetricName genera un nombre de métrica con formato estándar <namespace>.<entity>.<metric_type>.
// Esta función debe usarse para mantener la consistencia en los nombres de todas las métricas.
//
// Parámetros:
//   - namespace: espacio de nombres general (ej. "trading")
//   - entity: tipo de entidad (ej. "http", "candle", "document")
//   - metricType: tipo específico de métrica (ej. "requests", "errors", "latency")
func MetricName(namespace, entity string, metricType string) string {
	return strings.Join([]string{namespace, entity, metricType}, ".")
}

// FormattedMetricName es un alias deprecated de MetricName para compatibilidad
// Deprecated: Use MetricName instead
func FormattedMetricName(namespace, entity string, metricType string) string {
	return MetricName(namespace, entity, metricType)
}
