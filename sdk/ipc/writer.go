package ipc

import (
	"fmt"
	"sync"
	"time"

	"github.com/xKoRx/echo/sdk/utils"
)

// JSONWriter escribe mensajes JSON line-delimited a un Pipe.
//
// Serializa writes para thread-safety y asegura que cada mensaje termine con \n.
type JSONWriter struct {
	pipe    Pipe
	mu      sync.Mutex // Serializar writes
	timeout time.Duration
}

// NewJSONWriter crea un nuevo JSONWriter para un pipe.
//
// Example:
//
//	writer := ipc.NewJSONWriter(pipe)
//	msg := map[string]interface{}{
//	    "type": "execute_order",
//	    "payload": map[string]interface{}{
//	        "command_id": "abc123",
//	    },
//	}
//	if err := writer.WriteMessage(msg); err != nil {
//	    // Handle error
//	}
func NewJSONWriter(pipe Pipe) *JSONWriter {
	return &JSONWriter{
		pipe:    pipe,
		timeout: 5 * time.Second,
	}
}

// NewJSONWriterWithTimeout crea un JSONWriter con timeout custom.
func NewJSONWriterWithTimeout(pipe Pipe, timeout time.Duration) *JSONWriter {
	writer := NewJSONWriter(pipe)
	writer.timeout = timeout
	return writer
}

// WriteLine escribe una línea de bytes al pipe.
//
// Agrega \n automáticamente si no está presente.
// Es thread-safe (serializa writes con mutex).
func (w *JSONWriter) WriteLine(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Asegurar que termina con \n
	data = utils.EnsureNewlineBytes(data)

	// Establecer deadline si timeout está configurado
	if w.timeout > 0 {
		if err := w.pipe.SetWriteDeadline(time.Now().Add(w.timeout)); err != nil {
			return err
		}
	}

	// Escribir al pipe
	n, err := w.pipe.Write(data)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	// Verificar que se escribió todo
	if n != len(data) {
		return fmt.Errorf("incomplete write: wrote %d of %d bytes", n, len(data))
	}

	return nil
}

// WriteMessage serializa y escribe un mensaje JSON line-delimited.
//
// Formato de salida:
//
//	{"type":"execute_order","payload":{...}}\n
//
// Es thread-safe.
func (w *JSONWriter) WriteMessage(msg map[string]interface{}) error {
	// Serializar a JSON
	jsonBytes, err := utils.MapToJSON(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Escribir con newline
	return w.WriteLine(jsonBytes)
}

// WriteString escribe un string como línea.
//
// Agrega \n automáticamente.
func (w *JSONWriter) WriteString(s string) error {
	return w.WriteLine([]byte(s))
}

// WriteJSON escribe cualquier valor serializable a JSON.
//
// Example:
//
//	type MyMessage struct {
//	    Type    string `json:"type"`
//	    Payload interface{} `json:"payload"`
//	}
//	msg := MyMessage{Type: "test", Payload: map[string]string{"key": "value"}}
//	writer.WriteJSON(msg)
func (w *JSONWriter) WriteJSON(v interface{}) error {
	jsonBytes, err := utils.MarshalJSON(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return w.WriteLine(jsonBytes)
}

// SetTimeout establece el timeout para operaciones de escritura.
func (w *JSONWriter) SetTimeout(timeout time.Duration) {
	w.timeout = timeout
}

// WriteMessageWithTimeout escribe un mensaje con timeout específico.
func (w *JSONWriter) WriteMessageWithTimeout(msg map[string]interface{}, timeout time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	oldTimeout := w.timeout
	w.timeout = timeout
	defer func() { w.timeout = oldTimeout }()

	// Unlock para permitir que WriteMessage adquiera el lock
	w.mu.Unlock()
	err := w.WriteMessage(msg)
	w.mu.Lock()

	return err
}

// Flush no hace nada en Named Pipes (auto-flush), pero mantiene compatibilidad.
func (w *JSONWriter) Flush() error {
	// Named Pipes no necesitan flush explícito
	// Pero podemos implementarlo para compatibilidad con otras interfaces
	return nil
}

// LineWriter implementa Writer para Named Pipes sin JSON.
//
// Para casos donde se quiere control manual del formato.
type LineWriter struct {
	pipe    Pipe
	mu      sync.Mutex
	timeout time.Duration
}

// NewLineWriter crea un nuevo LineWriter.
func NewLineWriter(pipe Pipe) *LineWriter {
	return &LineWriter{
		pipe:    pipe,
		timeout: 5 * time.Second,
	}
}

// WriteLine escribe una línea de bytes.
func (lw *LineWriter) WriteLine(data []byte) error {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	data = utils.EnsureNewlineBytes(data)

	if lw.timeout > 0 {
		if err := lw.pipe.SetWriteDeadline(time.Now().Add(lw.timeout)); err != nil {
			return err
		}
	}

	n, err := lw.pipe.Write(data)
	if err != nil {
		return err
	}

	if n != len(data) {
		return fmt.Errorf("incomplete write: %d of %d bytes", n, len(data))
	}

	return nil
}

// WriteString escribe un string.
func (lw *LineWriter) WriteString(s string) error {
	return lw.WriteLine([]byte(s))
}

// SetTimeout establece el timeout.
func (lw *LineWriter) SetTimeout(timeout time.Duration) {
	lw.timeout = timeout
}

// BufferedWriter combina Reader y Writer para comunicación bidireccional.
type BufferedWriter struct {
	*JSONWriter
	*JSONReader
}

// NewBufferedWriter crea un writer/reader bidireccional.
//
// Example:
//
//	rw := ipc.NewBufferedWriter(pipe)
//	// Escribir
//	rw.WriteMessage(msg)
//	// Leer
//	response, _ := rw.ReadMessage()
func NewBufferedWriter(pipe Pipe) *BufferedWriter {
	return &BufferedWriter{
		JSONWriter: NewJSONWriter(pipe),
		JSONReader: NewJSONReader(pipe),
	}
}

