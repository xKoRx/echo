package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// ServerConfig configuración para servidor gRPC.
type ServerConfig struct {
	// Port puerto del servidor
	Port int

	// Address dirección de bind (ej: "0.0.0.0", "192.168.31.60")
	Address string

	// KeepAlive configuración de keepalive
	KeepAlive *ServerKeepAliveConfig

	// MaxConnections número máximo de conexiones concurrentes (0 = sin límite)
	MaxConnections int

	// ConnectionTimeout timeout para establecer conexión
	ConnectionTimeout time.Duration

	// ShutdownGracePeriod periodo de gracia para shutdown
	ShutdownGracePeriod time.Duration

	// UnaryInterceptors interceptors para llamadas unary
	UnaryInterceptors []grpc.UnaryServerInterceptor

	// StreamInterceptors interceptors para streams
	StreamInterceptors []grpc.StreamServerInterceptor
}

// ServerKeepAliveConfig configuración de keepalive del servidor.
type ServerKeepAliveConfig struct {
	// MaxConnectionIdle tiempo máximo de conexión idle antes de cerrar
	MaxConnectionIdle time.Duration

	// MaxConnectionAge edad máxima de conexión antes de forzar cierre
	MaxConnectionAge time.Duration

	// MaxConnectionAgeGrace periodo de gracia tras MaxConnectionAge
	MaxConnectionAgeGrace time.Duration

	// Time intervalo de keepalive pings
	Time time.Duration

	// Timeout timeout para respuesta de ping
	Timeout time.Duration
}

// DefaultServerConfig retorna configuración por defecto.
func DefaultServerConfig(port int) *ServerConfig {
	return &ServerConfig{
		Port:                port,
		Address:             "0.0.0.0",
		MaxConnections:      0, // Sin límite
		ConnectionTimeout:   10 * time.Second,
		ShutdownGracePeriod: 30 * time.Second,
		KeepAlive: &ServerKeepAliveConfig{
			MaxConnectionIdle:     5 * time.Minute,
			MaxConnectionAge:      0, // Sin límite
			MaxConnectionAgeGrace: 1 * time.Minute,
			Time:                  2 * time.Hour,
			Timeout:               20 * time.Second,
		},
	}
}

// Server wrapper sobre grpc.Server con funcionalidad adicional.
type Server struct {
	grpcServer *grpc.Server
	config     *ServerConfig
	listener   net.Listener
	address    string
}

// NewServer crea un nuevo servidor gRPC.
//
// Example:
//
//	config := grpc.DefaultServerConfig(50051)
//	server, err := grpc.NewServer(config)
//	if err != nil {
//	    return err
//	}
//
//	// Registrar servicios
//	pb.RegisterAgentServiceServer(server.GRPCServer(), &myService{})
//
//	// Servir
//	if err := server.Serve(ctx); err != nil {
//	    return err
//	}
func NewServer(config *ServerConfig) (*Server, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Construir server options
	opts := []grpc.ServerOption{}

	// KeepAlive
	if config.KeepAlive != nil {
		kaParams := keepalive.ServerParameters{
			MaxConnectionIdle:     config.KeepAlive.MaxConnectionIdle,
			MaxConnectionAge:      config.KeepAlive.MaxConnectionAge,
			MaxConnectionAgeGrace: config.KeepAlive.MaxConnectionAgeGrace,
			Time:                  config.KeepAlive.Time,
			Timeout:               config.KeepAlive.Timeout,
		}
		opts = append(opts, grpc.KeepaliveParams(kaParams))

		// Enforcement policy: permitir pings más frecuentes para clientes detrás de redes ruidosas
		kaEnforcement := keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second, // permitir pings cada 10s
			PermitWithoutStream: true,
		}
		opts = append(opts, grpc.KeepaliveEnforcementPolicy(kaEnforcement))
	}

	// Interceptors
	if len(config.UnaryInterceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(config.UnaryInterceptors...))
	}
	if len(config.StreamInterceptors) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(config.StreamInterceptors...))
	}

	// Crear servidor gRPC
	grpcServer := grpc.NewServer(opts...)

	// Crear listener
	address := fmt.Sprintf("%s:%d", config.Address, config.Port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	server := &Server{
		grpcServer: grpcServer,
		config:     config,
		listener:   listener,
		address:    address,
	}

	return server, nil
}

// GRPCServer retorna el servidor gRPC subyacente.
//
// Útil para registrar servicios:
//
//	pb.RegisterAgentServiceServer(server.GRPCServer(), &myService{})
func (s *Server) GRPCServer() *grpc.Server {
	return s.grpcServer
}

// Address retorna la dirección en la que el servidor está escuchando.
func (s *Server) Address() string {
	return s.address
}

// Serve inicia el servidor y bloquea hasta que se llame Stop o el contexto se cancele.
//
// Example:
//
//	go func() {
//	    if err := server.Serve(ctx); err != nil {
//	        log.Errorf("Server error: %v", err)
//	    }
//	}()
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		return fmt.Errorf("listener is nil")
	}

	// Canal para errores de Serve
	errCh := make(chan error, 1)

	// Servir en goroutine
	go func() {
		errCh <- s.grpcServer.Serve(s.listener)
	}()

	// Esperar cancelación de contexto o error
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// Graceful shutdown
		return s.Shutdown(ctx)
	}
}

// Shutdown hace un graceful shutdown del servidor.
//
// Espera hasta que todas las conexiones activas terminen o se alcance el timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.grpcServer == nil {
		return nil
	}

	// Canal para señalizar fin de shutdown
	done := make(chan struct{})

	// Graceful stop en goroutine
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()

	// Esperar graceful stop o timeout
	timeout := s.config.ShutdownGracePeriod
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		// Forzar stop si timeout
		s.grpcServer.Stop()
		return fmt.Errorf("forced shutdown after %v", timeout)
	case <-ctx.Done():
		// Context cancelado, forzar stop
		s.grpcServer.Stop()
		return ctx.Err()
	}
}

// Stop detiene el servidor inmediatamente (no graceful).
func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.Stop()
	}
}

// IsServing indica si el servidor está activo.
func (s *Server) IsServing() bool {
	// grpc.Server no expone estado directamente
	// Verificar si listener está activo
	return s.listener != nil
}
