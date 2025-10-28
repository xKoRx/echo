package grpc

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/utils"
)

// LoggingUnaryClientInterceptor interceptor de logging para llamadas unary del cliente.
//
// Registra cada llamada RPC con duración y resultado.
func LoggingUnaryClientInterceptor(client *telemetry.Client) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()

		// Llamar método
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Log resultado
		duration := time.Since(start)
		attrs := []attribute.KeyValue{
			attribute.String("rpc.method", method),
			attribute.String("rpc.system", "grpc"),
			attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())),
		}

		if err != nil {
			client.Error(ctx, "gRPC call failed", err, attrs...)
		} else {
			client.Debug(ctx, "gRPC call succeeded", attrs...)
		}

		return err
	}
}

// LoggingStreamClientInterceptor interceptor de logging para streams del cliente.
func LoggingStreamClientInterceptor(client *telemetry.Client) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Abrir stream
		stream, err := streamer(ctx, desc, cc, method, opts...)

		attrs := []attribute.KeyValue{
			attribute.String("rpc.method", method),
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.type", "stream"),
		}

		if err != nil {
			client.Error(ctx, "gRPC stream open failed", err, attrs...)
			return nil, err
		}

		client.Info(ctx, "gRPC stream opened", attrs...)

		return stream, nil
	}
}

// LoggingUnaryServerInterceptor interceptor de logging para llamadas unary del servidor.
func LoggingUnaryServerInterceptor(client *telemetry.Client) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Ejecutar handler
		resp, err := handler(ctx, req)

		// Log resultado
		duration := time.Since(start)
		attrs := []attribute.KeyValue{
			attribute.String("rpc.method", info.FullMethod),
			attribute.String("rpc.system", "grpc"),
			attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())),
		}

		if err != nil {
			client.Error(ctx, "gRPC handler failed", err, attrs...)
		} else {
			client.Debug(ctx, "gRPC handler succeeded", attrs...)
		}

		return resp, err
	}
}

// LoggingStreamServerInterceptor interceptor de logging para streams del servidor.
func LoggingStreamServerInterceptor(client *telemetry.Client) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		attrs := []attribute.KeyValue{
			attribute.String("rpc.method", info.FullMethod),
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.type", "stream"),
		}

		client.Info(ss.Context(), "gRPC stream handler started", attrs...)

		// Ejecutar handler
		err := handler(srv, ss)

		duration := time.Since(start)
		attrs = append(attrs, attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())))

		if err != nil {
			client.Error(ss.Context(), "gRPC stream handler failed", err, attrs...)
		} else {
			client.Info(ss.Context(), "gRPC stream handler completed", attrs...)
		}

		return err
	}
}

// TracingUnaryClientInterceptor propaga trace context en llamadas unary del cliente.
//
// Extrae trace_id del contexto y lo propaga via metadata.
func TracingUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Propagar trace_id si está en contexto
		if traceID := getTraceIDFromContext(ctx); traceID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "trace-id", traceID)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// TracingStreamClientInterceptor propaga trace context en streams del cliente.
func TracingStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Propagar trace_id si está en contexto
		if traceID := getTraceIDFromContext(ctx); traceID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "trace-id", traceID)
		}

		return streamer(ctx, desc, cc, method, opts...)
	}
}

// TracingUnaryServerInterceptor extrae trace context de metadata en servidor.
func TracingUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extraer trace_id de metadata
		if traceID := getTraceIDFromMetadata(ctx); traceID != "" {
			ctx = setTraceIDInContext(ctx, traceID)
		}

		return handler(ctx, req)
	}
}

// TracingStreamServerInterceptor extrae trace context en streams del servidor.
func TracingStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		// Extraer trace_id de metadata
		if traceID := getTraceIDFromMetadata(ctx); traceID != "" {
			ctx = setTraceIDInContext(ctx, traceID)
		}

		// Wrap ServerStream para usar contexto actualizado
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		return handler(srv, wrappedStream)
	}
}

// wrappedServerStream wrapper para ServerStream con contexto custom.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context retorna el contexto custom.
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// ErrorHandlingUnaryClientInterceptor convierte errores gRPC a formato consistente.
func ErrorHandlingUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			// Convertir a status error si no lo es
			if _, ok := status.FromError(err); !ok {
				err = status.Error(codes.Unknown, err.Error())
			}
		}
		return err
	}
}

// Helpers para trace_id

type contextKey string

const traceIDKey contextKey = "trace_id"

// getTraceIDFromContext extrae trace_id del contexto.
func getTraceIDFromContext(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// setTraceIDInContext establece trace_id en el contexto.
func setTraceIDInContext(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// getTraceIDFromMetadata extrae trace_id de metadata gRPC.
func getTraceIDFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get("trace-id")
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

// SetTraceID establece trace_id en el contexto.
//
// Útil para iniciar trace desde un componente que genera el trade_id.
//
// Example:
//
//	ctx = grpc.SetTraceID(ctx, tradeID)
func SetTraceID(ctx context.Context, traceID string) context.Context {
	return setTraceIDInContext(ctx, traceID)
}

// GetTraceID obtiene trace_id del contexto.
func GetTraceID(ctx context.Context) string {
	return getTraceIDFromContext(ctx)
}

// GetOrGenerateTraceID obtiene trace_id del contexto o genera uno nuevo.
func GetOrGenerateTraceID(ctx context.Context) (context.Context, string) {
	traceID := getTraceIDFromContext(ctx)
	if traceID == "" {
		traceID = utils.GenerateUUIDv7()
		ctx = setTraceIDInContext(ctx, traceID)
	}
	return ctx, traceID
}
