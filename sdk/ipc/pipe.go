// Package ipc provee abstracciones para comunicación inter-proceso (IPC).
//
// Este paquete abstrae Named Pipes de Windows para comunicación entre
// el Agent (Go) y los EAs (MQL4/MQL5).
//
// # Arquitectura
//
// - Agent crea Named Pipes (server)
// - EAs se conectan a los pipes (clients via DLL)
// - Protocolo: JSON line-delimited (cada mensaje termina con \n)
//
// # Uso Básico (Server - Agent)
//
//	// Crear pipe server
//	pipe, err := ipc.NewWindowsPipeServer("echo_master_12345")
//	if err != nil {
//	    return err
//	}
//	defer pipe.Close()
//
//	// Esperar conexión
//	if err := pipe.WaitForConnection(ctx); err != nil {
//	    return err
//	}
//
//	// Leer mensajes
//	reader := ipc.NewJSONReader(pipe)
//	for {
//	    msg, err := reader.ReadMessage()
//	    if err != nil {
//	        break
//	    }
//	    // Procesar mensaje...
//	}
//
// # Uso Básico (Writer)
//
//	writer := ipc.NewJSONWriter(pipe)
//	msg := map[string]interface{}{
//	    "type": "execute_order",
//	    "payload": map[string]interface{}{
//	        "command_id": "abc123",
//	    },
//	}
//	if err := writer.WriteMessage(msg); err != nil {
//	    return err
//	}
//
// # Características
//
// - Buffering para lecturas eficientes
// - Timeout configurable
// - Line-delimited para parsing simple en MQL4
// - Reconexión (responsabilidad del caller)
package ipc

import (
	"context"
	"io"
	"time"
)

// Pipe define la interfaz para un Named Pipe bidireccional.
//
// Compatible con Windows Named Pipes. Puede extenderse a Unix Domain Sockets.
type Pipe interface {
	// Read lee datos del pipe.
	//
	// Retorna el número de bytes leídos y error si aplica.
	// Si no hay datos disponibles, puede bloquearse según timeout.
	Read(p []byte) (n int, err error)

	// Write escribe datos al pipe.
	//
	// Retorna el número de bytes escritos y error si aplica.
	Write(p []byte) (n int, err error)

	// Close cierra el pipe y libera recursos.
	Close() error

	// SetReadDeadline establece el deadline para operaciones de lectura.
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline establece el deadline para operaciones de escritura.
	SetWriteDeadline(t time.Time) error
}

// PipeServer define la interfaz para un servidor de Named Pipes.
//
// El servidor crea el pipe y espera conexiones de clientes.
type PipeServer interface {
	Pipe

	// WaitForConnection espera a que un cliente se conecte.
	//
	// Bloquea hasta que un cliente se conecta o el contexto se cancela.
	WaitForConnection(ctx context.Context) error

	// DisconnectClient cierra solo la conexión actual sin cerrar el listener.
	//
	// Permite que el servidor continúe aceptando nuevas conexiones después
	// de que un cliente se desconecte. Útil para reconexión automática.
	DisconnectClient() error

	// Name retorna el nombre del pipe.
	Name() string
}

// PipeConfig configuración para crear Named Pipes.
type PipeConfig struct {
	// Name nombre del pipe (sin el prefijo \\.\pipe\)
	Name string

	// BufferSize tamaño del buffer del pipe (bytes)
	BufferSize int

	// Timeout timeout por defecto para operaciones (0 = sin timeout)
	Timeout time.Duration

	// MaxConnections número máximo de conexiones simultáneas (solo server)
	MaxConnections int
}

// DefaultPipeConfig retorna una configuración por defecto.
func DefaultPipeConfig(name string) *PipeConfig {
	return &PipeConfig{
		Name:           name,
		BufferSize:     8192, // 8KB
		Timeout:        5 * time.Second,
		MaxConnections: 1, // 1 conexión por pipe en i0
	}
}

// Reader abstracción de lectura line-delimited.
type Reader interface {
	// ReadLine lee una línea completa (hasta \n).
	ReadLine() ([]byte, error)

	// ReadMessage lee y parsea un mensaje JSON.
	ReadMessage() (map[string]interface{}, error)
}

// Writer abstracción de escritura line-delimited.
type Writer interface {
	// WriteLine escribe una línea (agrega \n automáticamente).
	WriteLine(data []byte) error

	// WriteMessage serializa y escribe un mensaje JSON.
	WriteMessage(msg map[string]interface{}) error
}

// PipeReader implementa buffered reading de Named Pipes.
type PipeReader struct {
	pipe   Pipe
	buffer []byte
	pos    int
	end    int
}

// PipeWriter implementa writing serializado de Named Pipes.
type PipeWriter struct {
	pipe Pipe
}

// ErrPipeClosed indica que el pipe fue cerrado.
var ErrPipeClosed = io.ErrClosedPipe

// ErrTimeout indica que una operación excedió el timeout.
var ErrTimeout = context.DeadlineExceeded

// ErrInvalidMessage indica que el mensaje recibido no es JSON válido.
type ErrInvalidMessage struct {
	Reason string
	Data   []byte
}

func (e *ErrInvalidMessage) Error() string {
	return "invalid message: " + e.Reason
}

// NewErrInvalidMessage crea un error de mensaje inválido.
func NewErrInvalidMessage(reason string, data []byte) error {
	return &ErrInvalidMessage{
		Reason: reason,
		Data:   data,
	}
}
