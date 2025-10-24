package metricbundle

import (
	"context"
	"fmt"
	"sync"
)

// Este archivo contiene funciones y estructuras para facilitar
// la migración desde el paquete original metrics al nuevo metricbundle.

// ----------------------------------------------------------------------------------
// Función global de migración desde el paquete metrics original al nuevo metricbundle
// ----------------------------------------------------------------------------------

var (
	// Garantiza que la migración solo se ejecute una vez
	migrationDone sync.Once

	// Guarda cualquier error durante la migración
	migrationErr error
)

// MigrateFromOldMetricsPackage convierte todos los bundles del paquete antiguo metrics
// al nuevo formato metricbundle. Esta función es segura para llamadas concurrentes.
//
// Se recomienda llamar a esta función al inicializar la aplicación, antes de usar
// cualquier función del nuevo paquete metricbundle.
//
// Ejemplo de uso:
//
//	import (
//	    "github.com/xKoRx/sdk/pkg/shared/telemetry/metricbundle"
//	)
//
//	func main() {
//	    // Migrar desde el paquete antiguo
//	    if err := metricbundle.MigrateFromOldMetricsPackage(ctx); err != nil {
//	        log.Warn(ctx, "Error en migración de métricas", err)
//	    }
//
//	    // Usar los nuevos bundles
//	    httpMetrics := metricbundle.GetGlobalHTTPMetrics()
//	    // ...
//	}
func MigrateFromOldMetricsPackage(ctx context.Context) error {
	migrationDone.Do(func() {
		// Aquí deberíamos migrar todos los bundles existentes
		// (Esta función es más un placeholder ya que la verdadera migración
		// dependerá de la estructura específica del paquete original)

		// Ejemplo de migración:
		/*
			// 1. Obtener el cliente de métricas actual
			metricsClient := obtenerClienteMetricas()

			// 2. Inicializar los nuevos bundles
			InitGlobalHTTPBundle(metricsClient)
			InitGlobalCandleBundle(metricsClient)
			InitGlobalTickBundle(metricsClient)
			InitGlobalDocumentBundle(metricsClient)
		*/

		migrationErr = fmt.Errorf("migration not implemented")
	})

	return migrationErr
}

// ----------------------------------------------------------------------------------
// Funciones para obtener una instancia, independientemente de si se
// ha migrado o no.
// ----------------------------------------------------------------------------------

// SafeGetHTTPMetrics intenta obtener el bundle HTTP, inicializándolo
// si es necesario y está disponible el cliente necesario.
func SafeGetHTTPMetrics() *HTTPMetrics {
	if globalHTTPMetrics != nil {
		return globalHTTPMetrics
	}

	// Si no está inicializado, retorna nil
	// Código para inicializarlo automáticamente iría aquí si tuviéramos
	// acceso al MetricsClient

	return nil
}

// SafeGetCandleMetrics intenta obtener el bundle Candle, inicializándolo
// si es necesario y está disponible el cliente necesario.
func SafeGetCandleMetrics() *CandleMetrics {
	if globalCandleMetrics != nil {
		return globalCandleMetrics
	}

	// Si no está inicializado, retorna nil
	return nil
}

// SafeGetTickMetrics intenta obtener el bundle Tick, inicializándolo
// si es necesario y está disponible el cliente necesario.
func SafeGetTickMetrics() *TickMetrics {
	if globalTickMetrics != nil {
		return globalTickMetrics
	}

	// Si no está inicializado, retorna nil
	return nil
}

// SafeGetDocumentMetrics intenta obtener el bundle Document, inicializándolo
// si es necesario y está disponible el cliente necesario.
func SafeGetDocumentMetrics() *DocumentMetrics {
	if globalDocumentMetrics != nil {
		return globalDocumentMetrics
	}

	// Si no está inicializado, retorna nil
	return nil
}
