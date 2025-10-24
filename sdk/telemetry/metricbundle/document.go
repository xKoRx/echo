package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// DocumentMetrics representa métricas relacionadas a documentos
type DocumentMetrics struct {
	*BaseMetrics
	// Si se necesitan métricas específicas adicionales, se añadirían aquí
}

// NewDocumentMetrics inicializa un nuevo bundle de métricas para documentos
func NewDocumentMetrics(client MetricsClient) *DocumentMetrics {
	// Creamos la base con namespace "trading" y entity "document"
	base := NewBaseMetrics(client, "trading", "document")

	return &DocumentMetrics{
		BaseMetrics: base,
	}
}

// ----------------------------------------------------------------------------------
// Bundle global singleton con inicialización segura para concurrencia
// ----------------------------------------------------------------------------------

var (
	globalDocumentMetrics   *DocumentMetrics
	onceInitDocumentMetrics sync.Once
)

// InitGlobalDocumentBundle inicializa el bundle global para uso compartido
func InitGlobalDocumentBundle(client MetricsClient) {
	onceInitDocumentMetrics.Do(func() {
		globalDocumentMetrics = NewDocumentMetrics(client)
	})
}

// GetGlobalDocumentMetrics retorna el bundle global ya inicializado
func GetGlobalDocumentMetrics() *DocumentMetrics {
	return globalDocumentMetrics // nil si no inicializado (no-op seguro)
}

// ----------------------------------------------------------------------------------
// Métodos específicos para documentos
// ----------------------------------------------------------------------------------

// AddDefaultDocumentAttributes añade atributos comunes para métricas de documentos
func AddDefaultDocumentAttributes(path string, fileSize int64, documentType string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Document.Path.String(path),
		semconv.Document.Size.Int64(fileSize),
		semconv.Document.Type.String(documentType),
		semconv.Metrics.Service.String("document-service"),
	}
}

// ----------------------------------------------------------------------------------
// Helpers para casos de uso comunes
// ----------------------------------------------------------------------------------

// RecordDocumentProcessed registra métricas para un documento procesado
func (dm *DocumentMetrics) RecordDocumentProcessed(
	ctx context.Context,
	path string,
	fileSize int64,
	documentType string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultDocumentAttributes(path, fileSize, documentType)
	attrs = append(attrs, additionalAttrs...)

	// Añadir el resultado (éxito o error)
	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	// Registrar en el contador de resultados
	dm.RecordResult(ctx, attrs...)
}
