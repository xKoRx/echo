package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// PostgresMetrics representa métricas de persistencia en PostgreSQL
type PostgresMetrics struct {
	*BaseMetrics // symphony.sqx.postgres.*
}

// NewPostgresMetrics crea el bundle para PostgreSQL
func NewPostgresMetrics(client MetricsClient) *PostgresMetrics {
	base := NewBaseMetrics(client, "symphony.sqx", "postgres")
	return &PostgresMetrics{BaseMetrics: base}
}

// Singleton
var (
	globalPostgresMetrics   *PostgresMetrics
	onceInitPostgresMetrics sync.Once
)

// InitGlobalPostgresBundle inicializa el bundle global
func InitGlobalPostgresBundle(client MetricsClient) {
	onceInitPostgresMetrics.Do(func() {
		globalPostgresMetrics = NewPostgresMetrics(client)
	})
}

// GetGlobalPostgresMetrics retorna el bundle global
func GetGlobalPostgresMetrics() *PostgresMetrics {
	return globalPostgresMetrics // nil si no inicializado (no-op seguro)
}

// RecordDBRegisterCount incrementa el resultado por registros/upserts
func (p *PostgresMetrics) RecordDBRegisterCount(ctx context.Context, count int64, attrs ...attribute.KeyValue) {
	base := []attribute.KeyValue{semconv.SQX.Step.String("db_register")}
	base = append(base, attrs...)
	for i := int64(0); i < count; i++ {
		p.RecordResult(ctx, base...)
	}
}

// StartDBOpTimer mide duración de una operación de BD
func (p *PostgresMetrics) StartDBOpTimer(ctx context.Context, op string, attrs ...attribute.KeyValue) func() {
	base := []attribute.KeyValue{semconv.SQX.Step.String(op)}
	base = append(base, attrs...)
	return p.StartDurationTimer(ctx, base...)
}
