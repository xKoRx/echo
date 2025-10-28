package internal

import (
	"fmt"

	"github.com/xKoRx/echo/sdk/utils"
	"go.opentelemetry.io/otel/attribute"
)

// nowUnixMilli retorna el timestamp actual en milisegundos.
//
// Wrapper sobre utils.NowUnixMilli() de la SDK.
func nowUnixMilli() int64 {
	return utils.NowUnixMilli()
}

// mapToAttrs convierte un map[string]interface{} a atributos OTEL.
//
// Helper para pasar fields a métodos de telemetría.
func mapToAttrs(fields map[string]interface{}) []attribute.KeyValue {
	if fields == nil {
		return nil
	}

	attrs := make([]attribute.KeyValue, 0, len(fields))
	for key, value := range fields {
		switch v := value.(type) {
		case string:
			attrs = append(attrs, attribute.String(key, v))
		case int:
			attrs = append(attrs, attribute.Int(key, v))
		case int64:
			attrs = append(attrs, attribute.Int64(key, v))
		case float64:
			attrs = append(attrs, attribute.Float64(key, v))
		case bool:
			attrs = append(attrs, attribute.Bool(key, v))
		default:
			// Para otros tipos, convertir a string
			attrs = append(attrs, attribute.String(key, toString(v)))
		}
	}

	return attrs
}

// toString convierte un valor a string de forma segura.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
