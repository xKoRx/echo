package internal

import (
	"context"
	"fmt"

	"github.com/xKoRx/echo/sdk/telemetry"
)

// initTelemetry inicializa el cliente de telemetría.
//
// Usa sdk/telemetry con bundle Echo incluido automáticamente.
func initTelemetry(ctx context.Context, config *Config) (*telemetry.Client, error) {
	// Opciones de configuración
	opts := []telemetry.Option{}

	// TODO i0: sin OTEL Collector, logs solo a stdout
	// En i0, no hay OTLP endpoint configurado, se usa defaults (stdout)
    if config.OTLPEndpoint != "" {
        // Usar mismo endpoint para trazas y métricas si solo se entrega uno
        opts = append(opts, telemetry.WithOTLPEndpoint(config.OTLPEndpoint))
    }
    // Endpoint específico para métricas (el collector que muestras no implementa MetricsService en 4317)
    // Por convención interna, usamos 14317 para métricas si está disponible.
    opts = append(opts, telemetry.WithMetricsEndpoint("192.168.31.60:14317"))

	// Agregar versión del servicio
	opts = append(opts, telemetry.WithVersion(config.ServiceVersion))

	// Inicializar cliente
	// EchoMetrics bundle se crea automáticamente en initMetrics
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
