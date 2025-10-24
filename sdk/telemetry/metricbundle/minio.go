package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// MinIOMetrics representa métricas de operaciones contra MinIO
type MinIOMetrics struct {
	*BaseMetrics // symphony.sqx.minio.*
}

// NewMinIOMetrics crea el bundle para MinIO
func NewMinIOMetrics(client MetricsClient) *MinIOMetrics {
	base := NewBaseMetrics(client, "symphony.sqx", "minio")
	return &MinIOMetrics{BaseMetrics: base}
}

// Singleton
var (
	globalMinIOMetrics   *MinIOMetrics
	onceInitMinIOMetrics sync.Once
)

// InitGlobalMinIOBundle inicializa el bundle global
func InitGlobalMinIOBundle(client MetricsClient) {
	onceInitMinIOMetrics.Do(func() {
		globalMinIOMetrics = NewMinIOMetrics(client)
	})
}

// GetGlobalMinIOMetrics retorna el bundle global
func GetGlobalMinIOMetrics() *MinIOMetrics {
	return globalMinIOMetrics // nil si no inicializado (no-op seguro)
}

// StartOpTimer mide duración de una operación (upload/download/list/etc)
func (m *MinIOMetrics) StartOpTimer(ctx context.Context, op string, bucket string, attrs ...attribute.KeyValue) func() {
	base := []attribute.KeyValue{attribute.String("bucket", bucket), semconv.SQX.Step.String(op)}
	base = append(base, attrs...)
	return m.StartDurationTimer(ctx, base...)
}

// RecordUploadCount incrementa el resultado para uploads
func (m *MinIOMetrics) RecordUploadCount(ctx context.Context, bucket string, count int64, attrs ...attribute.KeyValue) {
	base := []attribute.KeyValue{attribute.String("bucket", bucket), semconv.SQX.Step.String("upload_results")}
	base = append(base, attrs...)
	for i := int64(0); i < count; i++ {
		m.RecordResult(ctx, base...)
	}
}

// RecordDownloadCount incrementa el resultado para downloads
func (m *MinIOMetrics) RecordDownloadCount(ctx context.Context, bucket string, count int64, attrs ...attribute.KeyValue) {
	base := []attribute.KeyValue{attribute.String("bucket", bucket), semconv.SQX.Step.String("download_strategies")}
	base = append(base, attrs...)
	for i := int64(0); i < count; i++ {
		m.RecordResult(ctx, base...)
	}
}
