package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// CandleMetrics representa métricas relacionadas a velas financieras (OHLCV)
type CandleMetrics struct {
	*BaseMetrics
	// Métricas específicas para velas, si fueran necesarias
}

// NewCandleMetrics inicializa un nuevo bundle de métricas para velas
func NewCandleMetrics(client MetricsClient) *CandleMetrics {
	// Creamos la base con namespace "trading" y entity "candle"
	base := NewBaseMetrics(client, "trading", "candle")

	return &CandleMetrics{
		BaseMetrics: base,
		// Aquí se podrían agregar métricas específicas si fueran necesarias
	}
}

// ----------------------------------------------------------------------------------
// Bundle global singleton con inicialización segura para concurrencia
// ----------------------------------------------------------------------------------

var (
	globalCandleMetrics   *CandleMetrics
	onceInitCandleMetrics sync.Once
)

// InitGlobalCandleBundle inicializa el bundle global para uso compartido
func InitGlobalCandleBundle(client MetricsClient) {
	onceInitCandleMetrics.Do(func() {
		globalCandleMetrics = NewCandleMetrics(client)
	})
}

// GetGlobalCandleMetrics retorna el bundle global ya inicializado
func GetGlobalCandleMetrics() *CandleMetrics {
	return globalCandleMetrics // nil si no inicializado (no-op seguro)
}

// ----------------------------------------------------------------------------------
// Métodos específicos para velas
// ----------------------------------------------------------------------------------

// AddDefaultCandleAttributes añade atributos comunes para métricas de velas
func AddDefaultCandleAttributes(exchange, symbol, interval string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Exchange.String(exchange),
		semconv.Metrics.Symbol.String(symbol),
		semconv.Metrics.Interval.String(interval),
	}
}

// ----------------------------------------------------------------------------------
// Helpers para casos de uso comunes
// ----------------------------------------------------------------------------------

// RecordCandleProcessed registra métricas para una vela procesada
func (cm *CandleMetrics) RecordCandleProcessed(
	ctx context.Context,
	exchange, symbol, interval string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultCandleAttributes(exchange, symbol, interval)
	attrs = append(attrs, additionalAttrs...)

	// Añadir el resultado (éxito o error)
	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	// Registrar en el contador de resultados
	cm.RecordResult(ctx, attrs...)
}
