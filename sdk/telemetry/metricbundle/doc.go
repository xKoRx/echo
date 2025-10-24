// Package metricbundle proporciona una colección de bundles de métricas para diferentes
// dominios de aplicación, como HTTP, documentos, velas financieras y ticks de mercado.
//
// Cada bundle está diseñado para encapsular métricas específicas de su dominio y
// proporcionar una interfaz unificada para registrar métricas con atributos adecuados,
// siguiendo convenciones semánticas estandarizadas definidas en el paquete semconv.
//
// Estructura del paquete:
//
// - base.go: Define la estructura BaseMetrics y funcionalidad común para todos los bundles
// - http.go: Métricas relacionadas con solicitudes HTTP y APIs
// - candle.go: Métricas para velas financieras (OHLCV)
// - tick.go: Métricas para ticks de mercado
// - document.go: Métricas para procesamiento de documentos
// - migration.go: Facilita la migración desde sistemas de métricas anteriores
//
// Convención de nombres de métricas:
//
// Todas las métricas siguen el formato <namespace>.<entity>.<metric_type>, por ejemplo:
//   - trading.http.requests
//   - trading.candle.result
//   - trading.tick.duration
//
// Inicialización:
//
// Para utilizar los bundles de métricas, primero debe inicializarse el sistema de telemetría:
//
//	client, err := telemetry.Init(ctx, etcdClient, "servicio", "prod", telemetry.HTTP, telemetry.Candle)
//	if err != nil {
//	    log.Fatal("Error inicializando telemetría:", err)
//	}
//
// Uso básico:
//
//	// Obtener un bundle de métricas a través del cliente de telemetría
//	httpMetrics := client.HTTPMetrics()
//
//	// Registrar una solicitud HTTP
//	httpMetrics.RecordHTTPRequest(ctx, "GET", "/api/products", 200,
//	    semconv.Metrics.Service.String("product-api"),
//	)
//
//	// Registrar duración de operación
//	done := httpMetrics.StartDurationTimer(ctx,
//	    semconv.HTTP.Method.String("POST"),
//	    semconv.HTTP.Path.String("/api/orders"),
//	)
//	// ... realizar operación ...
//	done() // Registra la duración al llamar a done()
//
// Cada bundle incluye métodos específicos para su dominio que facilitan
// el registro de métricas con los atributos adecuados, manteniendo
// la coherencia y facilitando el análisis en sistemas de observabilidad.
package metricbundle
