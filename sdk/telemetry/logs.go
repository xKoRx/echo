package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
)

// Info registra un mensaje informativo
func (c *Client) Info(ctx context.Context, msg string, attrs ...attribute.KeyValue) {
	if c.logger == nil {
		return
	}
	
	args := c.convertAttrsToSlogArgs(attrs)
	c.logger.InfoContext(ctx, msg, args...)
}

// Error registra un mensaje de error
func (c *Client) Error(ctx context.Context, msg string, err error, attrs ...attribute.KeyValue) {
	if c.logger == nil {
		return
	}
	
	args := c.convertAttrsToSlogArgs(attrs)
	if err != nil {
		args = append(args, slog.String("error", err.Error()))
	}
	c.logger.ErrorContext(ctx, msg, args...)
}

// Warn registra un mensaje de advertencia
func (c *Client) Warn(ctx context.Context, msg string, attrs ...attribute.KeyValue) {
	if c.logger == nil {
		return
	}
	
	args := c.convertAttrsToSlogArgs(attrs)
	c.logger.WarnContext(ctx, msg, args...)
}

// Debug registra un mensaje de debug
func (c *Client) Debug(ctx context.Context, msg string, attrs ...attribute.KeyValue) {
	if c.logger == nil {
		return
	}
	
	args := c.convertAttrsToSlogArgs(attrs)
	c.logger.DebugContext(ctx, msg, args...)
}

// convertAttrsToSlogArgs convierte atributos OTEL a argumentos slog
func (c *Client) convertAttrsToSlogArgs(attrs []attribute.KeyValue) []any {
	args := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		args = append(args, string(attr.Key), attr.Value.AsInterface())
	}
	return args
}

