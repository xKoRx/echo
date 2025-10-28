// +build windows

package ipc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// WindowsPipe implementa Pipe usando Named Pipes de Windows.
//
// Usa github.com/Microsoft/go-winio para abstracción de Named Pipes.
type WindowsPipe struct {
	conn         net.Conn
	name         string
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// WindowsPipeServer implementa PipeServer para Named Pipes de Windows.
type WindowsPipeServer struct {
	listener     net.Listener
	name         string
	config       *PipeConfig
	currentConn  net.Conn
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// NewWindowsPipeServer crea un servidor de Named Pipe.
//
// El nombre del pipe se auto-completa con el prefijo \\.\pipe\.
//
// Example:
//
//	server, err := ipc.NewWindowsPipeServer("echo_master_12345")
//	// Pipe real: \\.\pipe\echo_master_12345
func NewWindowsPipeServer(name string) (PipeServer, error) {
	config := DefaultPipeConfig(name)
	return NewWindowsPipeServerWithConfig(config)
}

// NewWindowsPipeServerWithConfig crea un servidor con configuración custom.
func NewWindowsPipeServerWithConfig(config *PipeConfig) (PipeServer, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Formato: \\.\pipe\<name>
	pipePath := fmt.Sprintf(`\\.\pipe\%s`, config.Name)

	// Crear listener
	// go-winio maneja la creación del pipe con CreateNamedPipe internamente
	pipeConfig := &winio.PipeConfig{
		// Security descriptor: permite acceso local
		SecurityDescriptor: "", // Default: acceso local

		// Message mode: false = byte mode (line-delimited)
		MessageMode: false,

		// Input/Output buffer sizes
		InputBufferSize:  int32(config.BufferSize),
		OutputBufferSize: int32(config.BufferSize),
	}

	listener, err := winio.ListenPipe(pipePath, pipeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe listener: %w", err)
	}

	server := &WindowsPipeServer{
		listener:     listener,
		name:         config.Name,
		config:       config,
		readTimeout:  config.Timeout,
		writeTimeout: config.Timeout,
	}

	return server, nil
}

// WaitForConnection espera a que un cliente se conecte.
//
// Bloquea hasta que un cliente se conecta o el contexto se cancela.
func (s *WindowsPipeServer) WaitForConnection(ctx context.Context) error {
	// Canal para manejar Accept con timeout
	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := s.listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	select {
	case conn := <-connCh:
		s.currentConn = conn
		return nil
	case err := <-errCh:
		return fmt.Errorf("accept failed: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Read implementa io.Reader.
func (s *WindowsPipeServer) Read(p []byte) (n int, err error) {
	if s.currentConn == nil {
		return 0, ErrPipeClosed
	}

	return s.currentConn.Read(p)
}

// Write implementa io.Writer.
func (s *WindowsPipeServer) Write(p []byte) (n int, err error) {
	if s.currentConn == nil {
		return 0, ErrPipeClosed
	}

	return s.currentConn.Write(p)
}

// Close cierra el servidor y la conexión activa.
func (s *WindowsPipeServer) Close() error {
	var err error
	if s.currentConn != nil {
		err = s.currentConn.Close()
		s.currentConn = nil
	}

	if s.listener != nil {
		if lerr := s.listener.Close(); lerr != nil && err == nil {
			err = lerr
		}
		s.listener = nil
	}

	return err
}

// SetReadDeadline establece el deadline para lecturas.
func (s *WindowsPipeServer) SetReadDeadline(t time.Time) error {
	if s.currentConn == nil {
		return ErrPipeClosed
	}
	return s.currentConn.SetReadDeadline(t)
}

// SetWriteDeadline establece el deadline para escrituras.
func (s *WindowsPipeServer) SetWriteDeadline(t time.Time) error {
	if s.currentConn == nil {
		return ErrPipeClosed
	}
	return s.currentConn.SetWriteDeadline(t)
}

// Name retorna el nombre del pipe (sin prefijo).
func (s *WindowsPipeServer) Name() string {
	return s.name
}

// NewWindowsPipeClient crea un cliente que se conecta a un Named Pipe existente.
//
// Example:
//
//	client, err := ipc.NewWindowsPipeClient("echo_master_12345")
func NewWindowsPipeClient(name string) (Pipe, error) {
	config := DefaultPipeConfig(name)
	return NewWindowsPipeClientWithConfig(config)
}

// NewWindowsPipeClientWithConfig crea un cliente con configuración custom.
func NewWindowsPipeClientWithConfig(config *PipeConfig) (Pipe, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	pipePath := fmt.Sprintf(`\\.\pipe\%s`, config.Name)

	// Conectar al pipe
	// go-winio usa DialPipe internamente que llama a CreateFile
	conn, err := winio.DialPipe(pipePath, &config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to pipe: %w", err)
	}

	pipe := &WindowsPipe{
		conn:         conn,
		name:         config.Name,
		readTimeout:  config.Timeout,
		writeTimeout: config.Timeout,
	}

	return pipe, nil
}

// Read implementa io.Reader.
func (p *WindowsPipe) Read(b []byte) (n int, err error) {
	if p.conn == nil {
		return 0, ErrPipeClosed
	}

	return p.conn.Read(b)
}

// Write implementa io.Writer.
func (p *WindowsPipe) Write(b []byte) (n int, err error) {
	if p.conn == nil {
		return 0, ErrPipeClosed
	}

	return p.conn.Write(b)
}

// Close cierra la conexión.
func (p *WindowsPipe) Close() error {
	if p.conn == nil {
		return nil
	}

	err := p.conn.Close()
	p.conn = nil
	return err
}

// SetReadDeadline establece el deadline para lecturas.
func (p *WindowsPipe) SetReadDeadline(t time.Time) error {
	if p.conn == nil {
		return ErrPipeClosed
	}
	return p.conn.SetReadDeadline(t)
}

// SetWriteDeadline establece el deadline para escrituras.
func (p *WindowsPipe) SetWriteDeadline(t time.Time) error {
	if p.conn == nil {
		return ErrPipeClosed
	}
	return p.conn.SetWriteDeadline(t)
}

