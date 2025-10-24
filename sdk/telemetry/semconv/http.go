package semconv

import (
	"go.opentelemetry.io/otel/attribute"
)

// HTTP define las convenciones semánticas para atributos OpenTelemetry relacionados con HTTP.
// Estos atributos permiten instrumentar de manera consistente las peticiones HTTP,
// siguiendo las convenciones estándar de OpenTelemetry para servicios web.
//
// Los atributos se organizan en categorías lógicas: información de la petición (método, ruta),
// información del cliente, detalles de la respuesta, y eventos específicos del ciclo de vida HTTP.
var HTTP struct {
	// Method identifica el método HTTP de la petición (GET, POST, PUT, etc.).
	Method attribute.Key

	// Path representa la ruta de la URL sin parámetros de consulta.
	Path attribute.Key

	// URL representa la URL completa incluyendo protocolo, host, puerto y parámetros.
	URL attribute.Key

	// Endpoint identifica el punto de entrada o handler que procesó la petición.
	Endpoint attribute.Key

	// UserAgent contiene la información del agente de usuario del cliente.
	UserAgent attribute.Key

	// ClientIP registra la dirección IP del cliente que realizó la petición.
	ClientIP attribute.Key

	// StatusCode almacena el código de estado HTTP de la respuesta (200, 404, 500, etc.).
	StatusCode attribute.Key

	// DurationMs registra la duración de la petición en milisegundos.
	DurationMs attribute.Key

	// Error contiene información detallada si ocurrió un error durante el procesamiento.
	Error attribute.Key

	// EventStart marca el inicio de una petición HTTP.
	EventStart attribute.Key

	// EventComplete marca la finalización exitosa de una petición HTTP.
	EventComplete attribute.Key

	// EventError marca un error durante el procesamiento de una petición HTTP.
	EventError attribute.Key

	// Component identifica el componente HTTP general (servidor, cliente, etc.).
	Component attribute.Key

	// Middleware identifica el middleware específico que procesó la petición.
	Middleware attribute.Key

	// Handler identifica el manejador final que procesó la petición.
	Handler attribute.Key
}

func init() {
	// Inicialización de atributos HTTP
	HTTP.Method = attribute.Key("http.method")
	HTTP.Path = attribute.Key("http.path")
	HTTP.URL = attribute.Key("http.url")
	HTTP.Endpoint = attribute.Key("http.endpoint")

	HTTP.UserAgent = attribute.Key("http.user_agent")
	HTTP.ClientIP = attribute.Key("http.client_ip")

	HTTP.StatusCode = attribute.Key("http.status_code")
	HTTP.DurationMs = attribute.Key("http.duration_ms")
	HTTP.Error = attribute.Key("http.error")

	// Eventos
	HTTP.EventStart = attribute.Key("request_start")
	HTTP.EventComplete = attribute.Key("request_complete")
	HTTP.EventError = attribute.Key("request_error")

	// Componentes
	HTTP.Component = attribute.Key("http.component")
	HTTP.Middleware = attribute.Key("http.middleware")
	HTTP.Handler = attribute.Key("http.handler")
}
