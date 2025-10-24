package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// HTTPMetrics representa métricas relacionadas a peticiones HTTP y proporciona
// métodos especializados para registrar eventos y mediciones de APIs.
//
// Este bundle está diseñado para instrumentar servicios web, APIs REST,
// y cualquier componente que procese peticiones HTTP, facilitando el monitoreo
// y análisis de tráfico, errores y rendimiento.
type HTTPMetrics struct {
	*BaseMetrics
	// RequestsCounter contabiliza el número de peticiones HTTP procesadas,
	// categorizado por método, ruta, código de estado, etc.
	RequestsCounter metric.Int64Counter
}

// NewHTTPMetrics inicializa un nuevo bundle de métricas para HTTP.
//
// Este bundle extiende BaseMetrics con contadores específicos para monitoreo HTTP.
// Utiliza el namespace "trading" y la entidad "http" por defecto.
//
// Parámetros:
//   - client: implementación de MetricsClient para registrar métricas
func NewHTTPMetrics(client MetricsClient) *HTTPMetrics {
	// Creamos la base con namespace "trading" y entity "http"
	base := NewBaseMetrics(client, "trading", "http")

	// Métricas específicas para HTTP
	requestsCounter := client.Counter(
		MetricName("trading", "http", "requests"),
		"Number of HTTP requests handled, labeled by status, service, path, etc.",
	)

	return &HTTPMetrics{
		BaseMetrics:     base,
		RequestsCounter: requestsCounter,
	}
}

// ----------------------------------------------------------------------------------
// Bundle global singleton con inicialización segura para concurrencia
// ----------------------------------------------------------------------------------

var (
	globalHTTPMetrics   *HTTPMetrics
	onceInitHTTPMetrics sync.Once
)

// InitGlobalHTTPBundle inicializa el bundle global de HTTP para uso compartido.
//
// Este método debe ser llamado una sola vez al inicio de la aplicación,
// normalmente desde el cliente principal de telemetría.
//
// Parámetros:
//   - client: implementación de MetricsClient para registrar métricas
func InitGlobalHTTPBundle(client MetricsClient) {
	onceInitHTTPMetrics.Do(func() {
		globalHTTPMetrics = NewHTTPMetrics(client)
	})
}

// GetGlobalHTTPMetrics retorna el bundle global ya inicializado.
//
// Panics: Si el bundle no ha sido inicializado previamente con InitGlobalHTTPBundle.
func GetGlobalHTTPMetrics() *HTTPMetrics {
	return globalHTTPMetrics // nil si no inicializado (no-op seguro)
}

// ----------------------------------------------------------------------------------
// Métodos específicos de HTTP
// ----------------------------------------------------------------------------------

// RecordRequests incrementa el contador de requests HTTP.
//
// Parámetros:
//   - ctx: Contexto de la operación, puede contener un span activo
//   - amount: Cantidad a incrementar (normalmente 1)
//   - attrs: Atributos adicionales para categorizar la petición
func (hm *HTTPMetrics) RecordRequests(ctx context.Context, amount int64, attrs ...attribute.KeyValue) {
	if hm == nil || hm.RequestsCounter == nil {
		return
	}
	hm.RequestsCounter.Add(ctx, amount, metric.WithAttributes(attrs...))
}

// AddDefaultHTTPAttributes añade atributos comunes de HTTP a un conjunto existente.
//
// Esta función de utilidad genera un conjunto de atributos estándar para peticiones HTTP
// que incluyen método, ruta y código de estado.
//
// Parámetros:
//   - method: Método HTTP (GET, POST, PUT, etc.)
//   - path: Ruta de la petición sin parámetros de consulta
//   - statusCode: Código de estado HTTP (200, 404, 500, etc.)
func AddDefaultHTTPAttributes(method, path string, statusCode int) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Service.String("api"),
		semconv.HTTP.Method.String(method),
		semconv.HTTP.Path.String(path),
		semconv.HTTP.StatusCode.Int(statusCode),
	}
}

// ----------------------------------------------------------------------------------
// Helpers para casos de uso comunes
// ----------------------------------------------------------------------------------

// RecordHTTPRequest es un helper que registra una petición HTTP con todos sus atributos.
//
// Este método combina las operaciones comunes de registro de métricas para una petición HTTP:
// incrementa el contador de peticiones y registra el resultado según el código de estado.
//
// Parámetros:
//   - ctx: Contexto de la operación, puede contener un span activo
//   - method: Método HTTP (GET, POST, PUT, etc.)
//   - path: Ruta de la petición sin parámetros de consulta
//   - statusCode: Código de estado HTTP (200, 404, 500, etc.)
//   - additionalAttrs: Atributos adicionales opcionales para enriquecer las métricas
func (hm *HTTPMetrics) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, additionalAttrs ...attribute.KeyValue) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultHTTPAttributes(method, path, statusCode)
	attrs = append(attrs, additionalAttrs...)

	// Registrar en el contador general
	hm.RecordRequests(ctx, 1, attrs...)

	// También registrar en el contador de resultados
	// con atributo de status basado en el código HTTP
	status := "success"
	if statusCode >= 400 {
		status = "error"
	}

	resultAttrs := append(attrs, semconv.Metrics.Status.String(status))
	hm.RecordResult(ctx, resultAttrs...)
}
