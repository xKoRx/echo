package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// ClientConfig configuración para cliente gRPC.
type ClientConfig struct {
	// Target dirección del servidor (ej: "192.168.31.60:50051")
	Target string

	// Timeout para conexión inicial
	DialTimeout time.Duration

	// KeepAlive configuración de keepalive
	KeepAlive *KeepAliveConfig

	// Insecure usar conexión sin TLS (true en i0)
	Insecure bool

	// MaxRetries número máximo de reintentos de conexión
	MaxRetries int

	// RetryBackoff backoff entre reintentos
	RetryBackoff time.Duration

	// UnaryInterceptors interceptors para llamadas unary
	UnaryInterceptors []grpc.UnaryClientInterceptor

	// StreamInterceptors interceptors para streams
	StreamInterceptors []grpc.StreamClientInterceptor
}

// KeepAliveConfig configuración de keepalive.
type KeepAliveConfig struct {
	// Time intervalo de keepalive pings
	Time time.Duration

	// Timeout timeout para respuesta de ping
	Timeout time.Duration

	// PermitWithoutStream permitir pings sin streams activos
	PermitWithoutStream bool
}

// DefaultClientConfig retorna configuración por defecto.
func DefaultClientConfig(target string) *ClientConfig {
	return &ClientConfig{
		Target:       target,
		DialTimeout:  10 * time.Second,
		Insecure:     true, // i0: sin TLS
		MaxRetries:   3,
		RetryBackoff: 2 * time.Second,
		KeepAlive: &KeepAliveConfig{
			Time:                5 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		},
	}
}

// Client wrapper sobre grpc.ClientConn con funcionalidad adicional.
type Client struct {
	conn   *grpc.ClientConn
	config *ClientConfig
	target string
}

// NewClient crea un nuevo cliente gRPC.
//
// Example:
//
//	config := grpc.DefaultClientConfig("192.168.31.60:50051")
//	client, err := grpc.NewClient(ctx, config)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
func NewClient(ctx context.Context, config *ClientConfig) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Construir dial options
	opts := []grpc.DialOption{
		grpc.WithBlock(), // Bloquear hasta conectar
	}

	// Credentials
	if config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// KeepAlive
	if config.KeepAlive != nil {
		kaParams := keepalive.ClientParameters{
			Time:                config.KeepAlive.Time,
			Timeout:             config.KeepAlive.Timeout,
			PermitWithoutStream: config.KeepAlive.PermitWithoutStream,
		}
		opts = append(opts, grpc.WithKeepaliveParams(kaParams))
	}

	// Interceptors
	if len(config.UnaryInterceptors) > 0 {
		opts = append(opts, grpc.WithChainUnaryInterceptor(config.UnaryInterceptors...))
	}
	if len(config.StreamInterceptors) > 0 {
		opts = append(opts, grpc.WithChainStreamInterceptor(config.StreamInterceptors...))
	}

	// Context con timeout para dial
	dialCtx := ctx
	if config.DialTimeout > 0 {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(ctx, config.DialTimeout)
		defer cancel()
	}

	// Conectar
	conn, err := grpc.DialContext(dialCtx, config.Target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", config.Target, err)
	}

	client := &Client{
		conn:   conn,
		config: config,
		target: config.Target,
	}

	return client, nil
}

// Conn retorna la conexión gRPC subyacente.
//
// Útil para pasar a servicios generados.
func (c *Client) Conn() *grpc.ClientConn {
	return c.conn
}

// Close cierra la conexión.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Target retorna el target del cliente.
func (c *Client) Target() string {
	return c.target
}

// State retorna el estado de la conexión.
func (c *Client) State() connectivity.State {
	if c.conn == nil {
		return connectivity.Shutdown
	}
	return c.conn.GetState()
}

// WaitForReady espera a que la conexión esté lista.
//
// Example:
//
//	if err := client.WaitForReady(ctx, 30*time.Second); err != nil {
//	    return err
//	}
func (c *Client) WaitForReady(ctx context.Context, timeout time.Duration) error {
	if c.conn == nil {
		return fmt.Errorf("connection is nil")
	}

	ctxWithTimeout := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		ctxWithTimeout, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Esperar a que el estado sea READY
	for {
		state := c.conn.GetState()
		if state == connectivity.Ready {
			return nil
		}

		if !c.conn.WaitForStateChange(ctxWithTimeout, state) {
			// Context cancelado o timeout
			return ctxWithTimeout.Err()
		}
	}
}

// IsReady indica si la conexión está lista.
func (c *Client) IsReady() bool {
	return c.State() == connectivity.Ready
}

// IsConnected indica si hay conexión (READY o IDLE).
func (c *Client) IsConnected() bool {
	state := c.State()
	return state == connectivity.Ready || state == connectivity.Idle
}

// Reconnect intenta reconectar si la conexión se perdió.
//
// Usa backoff exponencial según config.
func (c *Client) Reconnect(ctx context.Context) error {
	if c.IsConnected() {
		return nil // Ya conectado
	}

	// Cerrar conexión anterior
	if c.conn != nil {
		_ = c.conn.Close()
	}

	// Intentar reconectar con backoff
	var lastErr error
	backoff := c.config.RetryBackoff

	for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
		// Esperar backoff (excepto primera vez)
		if attempt > 0 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}

			// Exponencial backoff
			backoff = backoff * 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}

		// Intentar conectar
		newClient, err := NewClient(ctx, c.config)
		if err != nil {
			lastErr = err
			continue
		}

		// Éxito
		c.conn = newClient.conn
		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts: %w", c.config.MaxRetries, lastErr)
}

// WithRetry ejecuta una función con retry automático si la conexión se pierde.
//
// Example:
//
//	err := client.WithRetry(ctx, func() error {
//	    // Llamar método gRPC
//	    _, err := service.SomeMethod(ctx, req)
//	    return err
//	})
func (c *Client) WithRetry(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		// Verificar conexión
		if !c.IsConnected() {
			if err := c.Reconnect(ctx); err != nil {
				return err
			}
		}

		// Ejecutar función
		err := fn()
		if err == nil {
			return nil // Éxito
		}

		lastErr = err

		// Si el contexto fue cancelado, no reintentar
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Esperar backoff
		if attempt < c.config.MaxRetries {
			select {
			case <-time.After(c.config.RetryBackoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return lastErr
}
