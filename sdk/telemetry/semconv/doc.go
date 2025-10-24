// Package semconv define convenciones semánticas para atributos OpenTelemetry
// utilizados en el sistema de telemetría.
//
// Este paquete contiene estructuras que representan convenciones semánticas
// para diferentes dominios como logs, HTTP, documentos y métricas. Cada dominio
// tiene su propio conjunto de atributos predefinidos que siguen las mejores
// prácticas de OpenTelemetry y facilitan la correlación entre logs, métricas y trazas.
//
// Uso básico:
//
//	// Para logs
//	attrs := []attribute.KeyValue{
//	    semconv.Logs.Feature.String("Authorization"),
//	    semconv.Logs.Event.String("login_attempt"),
//	}
//
//	// Para HTTP
//	httpAttrs := []attribute.KeyValue{
//	    semconv.HTTP.Method.String("GET"),
//	    semconv.HTTP.Path.String("/api/v1/products"),
//	    semconv.HTTP.StatusCode.Int(200),
//	}
//
// Las convenciones definidas en este paquete permiten una instrumentación
// consistente en toda la aplicación y facilitan la observabilidad del sistema.
package semconv
