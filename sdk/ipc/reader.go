package ipc

import (
	"bufio"
	"bytes"
	"io"
	"time"

	"github.com/xKoRx/echo/sdk/utils"
)

// JSONReader lee mensajes JSON line-delimited desde un Pipe.
//
// Usa buffering para lecturas eficientes y parsea JSON automáticamente.
type JSONReader struct {
	pipe    Pipe
	scanner *bufio.Scanner
	timeout time.Duration
}

// NewJSONReader crea un nuevo JSONReader para un pipe.
//
// Example:
//
//	reader := ipc.NewJSONReader(pipe)
//	msg, err := reader.ReadMessage()
//	if err != nil {
//	    // Handle error
//	}
//	fmt.Println(msg["type"])
func NewJSONReader(pipe Pipe) *JSONReader {
	scanner := bufio.NewScanner(pipe)
	// Aumentar buffer para mensajes grandes (default: 64KB, aumentar a 1MB)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	return &JSONReader{
		pipe:    pipe,
		scanner: scanner,
		timeout: 5 * time.Second, // Default timeout
	}
}

// ParseJSONLine parsea una línea JSON a map (helper para logs/debug)
func ParseJSONLine(line []byte) (map[string]interface{}, error) {
	// Validar que no esté vacía
	b := bytes.TrimSpace(line)
	if len(b) == 0 {
		return nil, NewErrInvalidMessage("empty line", b)
	}
	if err := utils.ValidateJSON(b); err != nil {
		return nil, NewErrInvalidMessage("invalid JSON", b)
	}
	return utils.JSONToMap(b)
}

// NewJSONReaderWithTimeout crea un JSONReader con timeout custom.
func NewJSONReaderWithTimeout(pipe Pipe, timeout time.Duration) *JSONReader {
	reader := NewJSONReader(pipe)
	reader.timeout = timeout
	return reader
}

// ReadLine lee una línea completa del pipe (hasta \n).
//
// Usa buffering interno para eficiencia.
//
// Retorna la línea sin el \n final.
func (r *JSONReader) ReadLine() ([]byte, error) {
	// Establecer deadline si timeout está configurado
	if r.timeout > 0 {
		// Ignorar error de deadline si el pipe aún no tiene conexión activa
		if err := r.pipe.SetReadDeadline(time.Now().Add(r.timeout)); err != nil {
			return nil, err
		}
	}

	// Escanear siguiente línea
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, err
		}
		// EOF
		return nil, io.EOF
	}

	// Retornar bytes de la línea (sin \n)
	line := r.scanner.Bytes()

	// Copiar para evitar reutilización del buffer interno
	result := make([]byte, len(line))
	copy(result, line)

	return result, nil
}

// ReadMessage lee y parsea un mensaje JSON line-delimited.
//
// Formato esperado:
//
//	{"type":"trade_intent","payload":{...}}\n
//
// Retorna un map con el JSON parseado.
func (r *JSONReader) ReadMessage() (map[string]interface{}, error) {
	// Leer línea
	line, err := r.ReadLine()
	if err != nil {
		return nil, err
	}

	// Validar que no esté vacía
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, NewErrInvalidMessage("empty line", line)
	}

	// Validar JSON
	if err := utils.ValidateJSON(line); err != nil {
		return nil, NewErrInvalidMessage("invalid JSON", line)
	}

	// Parsear a map
	msg, err := utils.JSONToMap(line)
	if err != nil {
		return nil, NewErrInvalidMessage(err.Error(), line)
	}

	return msg, nil
}

// SetTimeout establece el timeout para operaciones de lectura.
func (r *JSONReader) SetTimeout(timeout time.Duration) {
	r.timeout = timeout
}

// ReadMessageWithTimeout lee un mensaje con timeout específico.
//
// Útil para operaciones que necesitan timeout diferente al default.
func (r *JSONReader) ReadMessageWithTimeout(timeout time.Duration) (map[string]interface{}, error) {
	oldTimeout := r.timeout
	r.timeout = timeout
	defer func() { r.timeout = oldTimeout }()

	return r.ReadMessage()
}

// PeekType lee el campo "type" de un mensaje sin consumirlo completamente.
//
// NOTA: Esta implementación sí consume el mensaje. Útil para routing rápido.
func (r *JSONReader) PeekType() (string, map[string]interface{}, error) {
	msg, err := r.ReadMessage()
	if err != nil {
		return "", nil, err
	}

	msgType := utils.ExtractString(msg, "type")
	return msgType, msg, nil
}

// BufferedReader retorna el scanner interno para acceso avanzado.
//
// Útil si se necesita control fino del buffering.
func (r *JSONReader) BufferedReader() *bufio.Scanner {
	return r.scanner
}

// LineReader implementa Reader para Named Pipes sin parsing JSON.
//
// Útil para casos donde se quiere control manual del parsing.
type LineReader struct {
	pipe    Pipe
	scanner *bufio.Scanner
	timeout time.Duration
}

// NewLineReader crea un nuevo LineReader.
func NewLineReader(pipe Pipe) *LineReader {
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	return &LineReader{
		pipe:    pipe,
		scanner: scanner,
		timeout: 5 * time.Second,
	}
}

// ReadLine lee una línea sin parsear JSON.
func (lr *LineReader) ReadLine() ([]byte, error) {
	if lr.timeout > 0 {
		if err := lr.pipe.SetReadDeadline(time.Now().Add(lr.timeout)); err != nil {
			return nil, err
		}
	}

	if !lr.scanner.Scan() {
		if err := lr.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	line := lr.scanner.Bytes()
	result := make([]byte, len(line))
	copy(result, line)

	return result, nil
}

// SetTimeout establece el timeout.
func (lr *LineReader) SetTimeout(timeout time.Duration) {
	lr.timeout = timeout
}
