package internal

import (
	"context"
	"fmt"

	"github.com/xKoRx/echo/sdk/telemetry"
)

// initTelemetry inicializa el cliente de telemetría para el Agent (i1).
//
// Usa sdk/telemetry con bundle Echo incluido automáticamente.
func initTelemetry(ctx context.Context, config *Config) (*telemetry.Client, error) {
	opts := []telemetry.Option{
		telemetry.WithVersion(config.ServiceVersion),
		telemetry.WithLogLevel(config.LogLevel),
	}

	// i1: Endpoints desde ETCD
	if config.OTLPEndpoint != "" {
		opts = append(opts, telemetry.WithOTLPEndpoint(config.OTLPEndpoint))
	}

	// i1: Endpoint específico para métricas desde config
	if config.MetricsEndpoint != "" {
		opts = append(opts, telemetry.WithMetricsEndpoint(config.MetricsEndpoint))
	}

	client, err := telemetry.New(
		ctx,
		config.ServiceName,
		config.Environment,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to init telemetry: %w", err)
	}

	return client, nil
}
