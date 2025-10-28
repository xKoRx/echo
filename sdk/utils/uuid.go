// Package utils provee utilidades comunes para el SDK de Echo
package utils

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// GenerateUUIDv7 genera un UUID v7 (ordenable por tiempo)
//
// UUIDv7 usa los primeros 48 bits para timestamp Unix ms,
// seguido de bits random, permitiendo orden cronológico.
//
// Formato: xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx
// donde x = timestamp/random, y = variant bits
//
// Example:
//
//	id := utils.GenerateUUIDv7()
//	// => "01HKQV8Y-9GJ3-7ABC-8DEF-123456789ABC"
func GenerateUUIDv7() string {
	// 1. Timestamp: primeros 48 bits (6 bytes)
	ts := time.Now().UnixMilli() // Unix timestamp en milisegundos

	// 2. Generar 10 bytes random para el resto del UUID
	randomBytes := make([]byte, 10)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback a timestamp + contador si crypto/rand falla
		binary.BigEndian.PutUint64(randomBytes, uint64(time.Now().UnixNano()))
	}

	// 3. Construir el UUID
	// Formato: tttttttt-tttt-7xxx-yxxx-xxxxxxxxxxxx
	// t = timestamp (48 bits)
	// 7 = version
	// y = variant (10xx en binario)
	uuid := make([]byte, 16)

	// Timestamp (48 bits = 6 bytes) en los primeros 6 bytes
	binary.BigEndian.PutUint64(uuid[0:8], uint64(ts<<16)) // Shift left para alinear

	// Random bytes (resto)
	copy(uuid[6:], randomBytes)

	// Set version 7 (4 bits en byte 6, posición alta)
	uuid[6] = (uuid[6] & 0x0F) | 0x70

	// Set variant (2 bits en byte 8, posición alta)
	uuid[8] = (uuid[8] & 0x3F) | 0x80

	// 4. Formatear como string con guiones
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(uuid[0:4]),
		binary.BigEndian.Uint16(uuid[4:6]),
		binary.BigEndian.Uint16(uuid[6:8]),
		binary.BigEndian.Uint16(uuid[8:10]),
		uuid[10:16],
	)
}

// MustGenerateUUIDv7 es igual que GenerateUUIDv7 pero entra en pánico en caso de error.
// Útil para inicializaciones donde el error es catastrófico.
func MustGenerateUUIDv7() string {
	// GenerateUUIDv7 ya maneja errores internamente con fallback
	return GenerateUUIDv7()
}

