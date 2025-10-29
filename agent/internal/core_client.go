package internal

import (
	"context"
	"fmt"

	grpcSDK "github.com/xKoRx/echo/sdk/grpc"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"google.golang.org/grpc/metadata"
)

// CoreClient wrapper sobre cliente gRPC al Core.
//
// Usa sdk/grpc para gestión de conexión y reconexión.
type CoreClient struct {
	client    *grpcSDK.Client
	svcClient pb.AgentServiceClient
}

// NewCoreClient crea un nuevo cliente al Core (i1).
//
// Usa configuración de KeepAlive desde Config (RFC-003 sección 7).
//
// Example:
//
//	client, err := NewCoreClient(ctx, config)
func NewCoreClient(ctx context.Context, agentConfig *Config) (*CoreClient, error) {
	// Configuración con defaults de SDK
	config := grpcSDK.DefaultClientConfig(agentConfig.CoreAddress)

	// i1: Configurar KeepAlive cliente desde config (RFC-003 sección 7)
	if agentConfig.KeepAliveTime > 0 {
		config.KeepAlive = &grpcSDK.KeepAliveConfig{
			Time:                agentConfig.KeepAliveTime,
			Timeout:             agentConfig.KeepAliveTimeout,
			PermitWithoutStream: agentConfig.PermitWithoutStream,
		}
	}

	// Crear cliente usando SDK
	grpcClient, err := grpcSDK.NewClient(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc client: %w", err)
	}

	// Crear cliente del servicio generado
	svcClient := pb.NewAgentServiceClient(grpcClient.Conn())

	return &CoreClient{
		client:    grpcClient,
		svcClient: svcClient,
	}, nil
}

// StreamBidi crea un stream bidireccional con el Core.
//
// Retorna el stream raw para que el Agent lo gestione.
// Issue #C7: Envía agent-id en metadata gRPC para identificación única.
func (c *CoreClient) StreamBidi(ctx context.Context, agentID string) (pb.AgentService_StreamBidiClient, error) {
	// Agregar agent-id al contexto como metadata (Issue #C7)
	md := metadata.New(map[string]string{
		"agent-id": agentID,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	stream, err := c.svcClient.StreamBidi(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	return stream, nil
}

// Ping ejecuta un health check al Core (i1).
func (c *CoreClient) Ping(ctx context.Context, agentID string) (*pb.PingResponse, error) {
	req := &pb.PingRequest{
		AgentId: agentID, // i1: ID real del agent desde config
	}

	resp, err := c.svcClient.Ping(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	return resp, nil
}

// Close cierra la conexión.
func (c *CoreClient) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

// IsReady indica si la conexión está lista.
func (c *CoreClient) IsReady() bool {
	if c.client == nil {
		return false
	}
	return c.client.IsReady()
}
