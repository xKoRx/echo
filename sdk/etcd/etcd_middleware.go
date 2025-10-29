package etcd

import (
	"context"
	"net/http"

	"github.com/xKoRx/sdk/pkg/shared/telemetry"
	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// contextKey se usa para almacenar el cliente etcd en el Context
type etcdContextKey string

const (
	etcdClientKey etcdContextKey = "etcd_client"
)

// SetEtcdClient establece el cliente etcd en el contexto
func SetEtcdClient(ctx context.Context, client *Client) context.Context {
	return context.WithValue(ctx, etcdClientKey, client)
}

// GetEtcdClient obtiene el cliente etcd desde el contexto
func GetEtcdClient(ctx context.Context) *Client {
	value, ok := ctx.Value(etcdClientKey).(*Client)
	if !ok {
		return nil
	}
	return value
}

// EtcdMiddleware crea un middleware que añade el cliente etcd al contexto de cada request
func EtcdMiddleware(client *Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			telemetryClient := telemetry.GetClient(r.Context())
			if telemetryClient == nil {
				telemetryClient = telemetry.GetGlobalClient()
			}

			ctx, span := telemetryClient.StartSpan(r.Context(), "middleware.etcd")
			defer span.End()

			httpMetrics := telemetryClient.HTTPMetrics()
			baseAttrs := []attribute.KeyValue{
				semconv.Logs.Feature.String("EtcdMiddleware"),
				semconv.HTTP.Path.String(r.URL.Path),
				semconv.HTTP.Method.String(r.Method),
				semconv.HTTP.Middleware.String("etcd"),
			}

			stopTimer := httpMetrics.StartDurationTimer(ctx, baseAttrs...)
			defer stopTimer()

			telemetryClient.Info(ctx, "Iniciando middleware de etcd", baseAttrs...)

			if client == nil {
				errAttrs := append(baseAttrs,
					semconv.Logs.Event.String("etcd_client_nil"))
				telemetryClient.Error(ctx, "etcd client is nil in middleware", nil, errAttrs...)

				span.SetStatus(codes.Error, "Etcd client is nil")
				httpMetrics.RecordRequests(ctx, 1, append(baseAttrs,
					semconv.HTTP.StatusCode.Int(http.StatusInternalServerError))...)

				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			telemetryClient.Info(ctx, "Cliente etcd configurado correctamente", baseAttrs...)

			// Añadir el cliente etcd al contexto y reemplazar en la request
			newCtx := SetEtcdClient(ctx, client)
			r = r.WithContext(newCtx)

			telemetryClient.Info(ctx, "Continuando con la cadena de middleware", baseAttrs...)

			// Registrar petición exitosa
			httpMetrics.RecordRequests(ctx, 1, append(baseAttrs,
				semconv.HTTP.StatusCode.Int(http.StatusOK))...)

			// Llamamos al siguiente middleware / handler
			next.ServeHTTP(w, r)

			telemetryClient.Info(ctx, "Middleware de etcd finalizado", append(baseAttrs,
				semconv.Logs.Event.String("middleware_completed"))...)
		})
	}
}
