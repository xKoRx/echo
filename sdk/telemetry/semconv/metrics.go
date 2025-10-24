package semconv

import (
	"go.opentelemetry.io/otel/attribute"
)

// Metrics define las convenciones semánticas para atributos OpenTelemetry
// usados en la recolección y categorización de métricas del sistema.
//
// Proporciona un conjunto estandarizado de atributos para dimensionar y
// clasificar las métricas generadas, siguiendo mejores prácticas de
// observabilidad y permitiendo análisis detallados en herramientas como Prometheus y Grafana.
var Metrics struct {
	// Status indica el estado de la operación que se está midiendo.
	// Valores comunes: "ok", "error", "retry", "timeout", etc.
	Status attribute.Key

	// Result representa el resultado final de la operación.
	// Valores comunes: "success", "failure", "partial", etc.
	Result attribute.Key

	// Action identifica la acción que se realizó.
	// Valores comunes: "create", "update", "delete", "process", etc.
	Action attribute.Key

	// Service identifica el servicio que genera la métrica.
	// Ejemplos: "api-persist", "middleend", "analyzer", etc.
	Service attribute.Key

	// Component identifica el componente específico dentro del servicio.
	// Ejemplos: "handler", "processor", "stream-consumer", etc.
	Component attribute.Key

	// Env identifica el entorno de ejecución.
	// Valores comunes: "development", "staging", "production", etc.
	Env attribute.Key

	// Region identifica la zona geográfica donde se ejecuta el servicio.
	// Ejemplos: "us-east", "eu-west", etc.
	Region attribute.Key

	// Instance identifica la instancia específica del servicio.
	// Ejemplos: nombre del pod, ID del host, etc.
	Instance attribute.Key

	// Exchange identifica el exchange de trading involucrado.
	// Ejemplos: "binance", "bybit", etc.
	Exchange attribute.Key

	// Symbol identifica el par de trading.
	// Ejemplos: "BTCUSDT", "ETHUSDT", etc.
	Symbol attribute.Key

	// Strategy identifica la estrategia de trading aplicada.
	// Ejemplos: "breakout", "momentum", etc.
	Strategy attribute.Key

	// Market identifica el tipo de mercado.
	// Valores comunes: "crypto", "forex", "stocks", etc.
	Market attribute.Key

	// Interval representa el intervalo temporal de la métrica.
	// Valores comunes: "1m", "5m", "1h", "1d", etc.
	Interval attribute.Key

	// TimeFrame representa el marco temporal de análisis.
	// Valores comunes: "intraday", "daily", "weekly", etc.
	TimeFrame attribute.Key

	// Size representa dimensiones de tamaño en la métrica.
	// Puede ser tamaño en bytes, cantidad de elementos, etc.
	Size attribute.Key

	// Duration representa una medida de tiempo, generalmente en segundos.
	Duration attribute.Key

	// Count representa un conteo simple de elementos o eventos.
	Count attribute.Key
}

func init() {
	// Inicialización de atributos de estado y resultado
	Metrics.Status = attribute.Key("status")
	Metrics.Result = attribute.Key("result")
	Metrics.Action = attribute.Key("action")

	// Inicialización de atributos de servicio
	Metrics.Service = attribute.Key("service")
	Metrics.Component = attribute.Key("component")

	// Inicialización de atributos de entorno
	Metrics.Env = attribute.Key("env")
	Metrics.Region = attribute.Key("region")
	Metrics.Instance = attribute.Key("instance")

	// Inicialización de atributos de negocio
	Metrics.Exchange = attribute.Key("exchange")
	Metrics.Symbol = attribute.Key("symbol")
	Metrics.Strategy = attribute.Key("strategy")
	Metrics.Market = attribute.Key("market")

	// Inicialización de atributos de temporalidad
	Metrics.Interval = attribute.Key("interval")
	Metrics.TimeFrame = attribute.Key("time_frame")

	// Inicialización de atributos de métricas específicas
	Metrics.Size = attribute.Key("size")
	Metrics.Duration = attribute.Key("duration")
	Metrics.Count = attribute.Key("count")
}
