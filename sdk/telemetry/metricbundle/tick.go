package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// TickMetrics representa métricas relacionadas a ticks de mercado
type TickMetrics struct {
	*BaseMetrics
	// Si se necesitan métricas específicas adicionales, se añadirían aquí
}

// NewTickMetrics inicializa un nuevo bundle de métricas para ticks
func NewTickMetrics(client MetricsClient) *TickMetrics {
	// Creamos la base con namespace "trading" y entity "tick"
	base := NewBaseMetrics(client, "trading", "tick")

	return &TickMetrics{
		BaseMetrics: base,
	}
}

// ----------------------------------------------------------------------------------
// Bundle global singleton con inicialización segura para concurrencia
// ----------------------------------------------------------------------------------

var (
	globalTickMetrics   *TickMetrics
	onceInitTickMetrics sync.Once
)

// InitGlobalTickBundle inicializa el bundle global para uso compartido
func InitGlobalTickBundle(client MetricsClient) {
	onceInitTickMetrics.Do(func() {
		globalTickMetrics = NewTickMetrics(client)
	})
}

// GetGlobalTickMetrics retorna el bundle global ya inicializado
func GetGlobalTickMetrics() *TickMetrics {
	return globalTickMetrics // nil si no inicializado (no-op seguro)
}

// ----------------------------------------------------------------------------------
// Métodos específicos para ticks
// ----------------------------------------------------------------------------------

// AddDefaultTickAttributes añade atributos comunes para métricas de ticks
func AddDefaultTickAttributes(exchange, symbol string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Exchange.String(exchange),
		semconv.Metrics.Symbol.String(symbol),
		semconv.Metrics.Service.String("market-data"),
	}
}

// ----------------------------------------------------------------------------------
// Helpers para casos de uso comunes
// ----------------------------------------------------------------------------------

// RecordTickProcessed registra métricas para un tick procesado
func (tm *TickMetrics) RecordTickProcessed(
	ctx context.Context,
	exchange string,
	symbol string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultTickAttributes(exchange, symbol)
	attrs = append(attrs, additionalAttrs...)

	// Añadir el resultado (éxito o error)
	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	// Añadir la acción realizada
	attrs = append(attrs, semconv.Metrics.Action.String("process"))

	// Registrar en el contador de resultados
	tm.RecordResult(ctx, attrs...)
}

// RecordTickVolume registra un volumen de ticks procesados
func (tm *TickMetrics) RecordTickVolume(
	ctx context.Context,
	exchange string,
	symbol string,
	count int64,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultTickAttributes(exchange, symbol)
	attrs = append(attrs, additionalAttrs...)

	// Añadir el conteo
	attrs = append(attrs, semconv.Metrics.Count.Int64(count))

	// Añadir la acción realizada
	attrs = append(attrs, semconv.Metrics.Action.String("count"))

	// Registrar en el contador de resultados, multiplicando por el conteo
	// para que represente la cantidad total de ticks
	for i := int64(0); i < count; i++ {
		tm.RecordResult(ctx, attrs...)
	}
}
