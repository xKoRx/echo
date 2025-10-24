package semconv

import (
	"go.opentelemetry.io/otel/attribute"
)

// Logs define las convenciones semánticas para atributos OpenTelemetry usados en logs.
// Proporciona atributos estandarizados que deben incluirse en todos los mensajes de log
// para facilitar la correlación, filtrado y análisis de los registros generados.
//
// Estos atributos se alinean con las convenciones de OpenTelemetry y permiten
// una integración fluida con sistemas de observabilidad como Loki y Grafana.
var Logs struct {
	// Feature identifica el componente o característica funcional que genera el log.
	// Ejemplos: "Authentication", "Database", "API", "Cache", etc.
	Feature attribute.Key

	// Event identifica la acción específica que ocurrió dentro del componente.
	// Ejemplos: "user_login", "query_executed", "cache_miss", etc.
	Event attribute.Key

	// ServiceName identifica el servicio que genera el log.
	// Se mapea directamente a la convención OTel "service.name".
	ServiceName attribute.Key

	// Environment identifica el entorno de ejecución.
	// Ejemplos: "development", "staging", "production".
	Environment attribute.Key
}

func init() {
	// Inicialización de las convenciones semánticas
	Logs.Feature = attribute.Key("feature")
	Logs.Event = attribute.Key("event")

	// Atributos de servicio (siguiendo convenciones OTel)
	Logs.ServiceName = attribute.Key("service.name")
	Logs.Environment = attribute.Key("service.environment")
}
