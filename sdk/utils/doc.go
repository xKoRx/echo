// Package utils provee utilidades comunes para el SDK de Echo.
//
// # Utilidades Incluidas
//
// - UUID: Generación de UUIDv7 ordenables por tiempo
// - Timestamp: Helpers para timestamps Unix en ms/μs
// - JSON: Parsing, validación y manipulación de JSON
//
// # Uso de UUID
//
// Generación de identificadores únicos ordenables:
//
//	id := utils.GenerateUUIDv7()
//	// => "01HKQV8Y-9GJ3-7ABC-8DEF-123456789ABC"
//
// # Uso de Timestamp
//
// Medición de latencia y timestamps:
//
//	start := utils.NowUnixMilli()
//	// ... operación ...
//	elapsed := utils.ElapsedMs(start)
//
//	// Conversión
//	t := utils.UnixMilliToTime(1698345601234)
//
// # Uso de JSON
//
// Parsing y validación de JSON:
//
//	// Validar
//	err := utils.ValidateJSON(data)
//
//	// Parsear a map
//	m, err := utils.JSONToMap(data)
//
//	// Extraer campos
//	tradeID := utils.ExtractString(m, "payload.trade_id")
//
//	// Pretty print
//	fmt.Println(utils.PrettyPrint(data))
//
// # Integración con Echo
//
// Este paquete es usado por:
//   - sdk/domain: transformers y validaciones
//   - sdk/ipc: parsing de mensajes JSON line-delimited
//   - Agent: routing y logging
//   - Core: orquestación y métricas
package utils

