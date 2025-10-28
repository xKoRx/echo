package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateJSON verifica si los datos son JSON válido.
//
// Example:
//
//	data := []byte(`{"type":"handshake"}`)
//	err := utils.ValidateJSON(data)
//	if err != nil {
//	    // No es JSON válido
//	}
func ValidateJSON(data []byte) error {
	var js interface{}
	return json.Unmarshal(data, &js)
}

// ValidateJSONString verifica si el string es JSON válido.
func ValidateJSONString(s string) error {
	return ValidateJSON([]byte(s))
}

// PrettyPrint formatea JSON con indentación para debugging.
//
// Example:
//
//	data := []byte(`{"type":"handshake","payload":{"client_id":"master_12345"}}`)
//	pretty := utils.PrettyPrint(data)
//	fmt.Println(pretty)
//	// Salida:
//	// {
//	//   "type": "handshake",
//	//   "payload": {
//	//     "client_id": "master_12345"
//	//   }
//	// }
func PrettyPrint(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data) // Retornar original si falla
	}
	return buf.String()
}

// PrettyPrintString formatea un string JSON con indentación.
func PrettyPrintString(s string) string {
	return PrettyPrint([]byte(s))
}

// Compact compacta JSON removiendo espacios innecesarios.
//
// Example:
//
//	data := []byte(`{
//	  "type": "handshake",
//	  "payload": {
//	    "client_id": "master_12345"
//	  }
//	}`)
//	compact := utils.Compact(data)
//	// => {"type":"handshake","payload":{"client_id":"master_12345"}}
func Compact(data []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return data // Retornar original si falla
	}
	return buf.Bytes()
}

// MarshalJSON serializa cualquier valor a JSON.
//
// Example:
//
//	data := map[string]interface{}{
//	    "type": "handshake",
//	    "timestamp_ms": 1698345601000,
//	}
//	jsonBytes, err := utils.MarshalJSON(data)
func MarshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// MarshalJSONIndent serializa con indentación.
func MarshalJSONIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// UnmarshalJSON deserializa JSON a un valor.
//
// Example:
//
//	var result map[string]interface{}
//	err := utils.UnmarshalJSON(jsonBytes, &result)
func UnmarshalJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// JSONToMap convierte JSON a map[string]interface{}.
//
// Útil para parsing de mensajes JSON dinámicos.
//
// Example:
//
//	data := []byte(`{"type":"trade_intent","trade_id":"abc123"}`)
//	m, err := utils.JSONToMap(data)
//	if err == nil {
//	    fmt.Println(m["type"]) // => "trade_intent"
//	}
func JSONToMap(data []byte) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := json.Unmarshal(data, &result)
	return result, err
}

// MapToJSON convierte un map a JSON.
func MapToJSON(m map[string]interface{}) ([]byte, error) {
	return json.Marshal(m)
}

// EnsureNewline asegura que el string JSON termine con \n (line-delimited).
//
// Example:
//
//	json := `{"type":"handshake"}`
//	json = utils.EnsureNewline(json)
//	// => `{"type":"handshake"}\n`
func EnsureNewline(s string) string {
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

// EnsureNewlineBytes asegura que los bytes JSON terminen con \n.
func EnsureNewlineBytes(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return append(data, '\n')
	}
	return data
}

// ExtractField extrae un campo de un JSON parseado a map.
//
// Soporta campos anidados con notación de punto.
//
// Example:
//
//	data := map[string]interface{}{
//	    "payload": map[string]interface{}{
//	        "trade_id": "abc123",
//	    },
//	}
//	tradeID := utils.ExtractField(data, "payload.trade_id")
//	// => "abc123"
func ExtractField(m map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = m

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil
			}
		default:
			return nil
		}
	}

	return current
}

// ExtractString es como ExtractField pero retorna string.
//
// Si el campo no existe o no es string, retorna "".
func ExtractString(m map[string]interface{}, path string) string {
	v := ExtractField(m, path)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ExtractInt64 es como ExtractField pero retorna int64.
//
// Si el campo no existe o no es numérico, retorna 0.
func ExtractInt64(m map[string]interface{}, path string) int64 {
	v := ExtractField(m, path)
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	}
	return 0
}

// ExtractFloat64 es como ExtractField pero retorna float64.
func ExtractFloat64(m map[string]interface{}, path string) float64 {
	v := ExtractField(m, path)
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	}
	return 0
}

// ExtractBool es como ExtractField pero retorna bool.
func ExtractBool(m map[string]interface{}, path string) bool {
	v := ExtractField(m, path)
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// ToJSONString convierte cualquier valor a JSON string.
//
// En caso de error, retorna string vacío.
func ToJSONString(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// FromJSONString parsea un JSON string a un valor.
func FromJSONString(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}

// IsValidJSONType verifica si un valor es un tipo JSON válido.
//
// Tipos válidos: nil, bool, int/int64/float64, string, []interface{}, map[string]interface{}.
func IsValidJSONType(v interface{}) bool {
	switch v.(type) {
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64, string, []interface{}, map[string]interface{}:
		return true
	default:
		return false
	}
}

// MustMarshalJSON serializa a JSON o entra en pánico.
//
// Útil para casos donde el error es catastrófico.
func MustMarshalJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("MustMarshalJSON: %v", err))
	}
	return data
}

// MustMarshalJSONString serializa a JSON string o entra en pánico.
func MustMarshalJSONString(v interface{}) string {
	return string(MustMarshalJSON(v))
}

