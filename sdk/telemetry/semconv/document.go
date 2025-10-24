package semconv

import (
	"go.opentelemetry.io/otel/attribute"
)

// Document define las convenciones semánticas para atributos OpenTelemetry
// usados en operaciones con documentos y archivos.
//
// Estos atributos permiten estandarizar la telemetría relacionada con operaciones
// de lectura, escritura y gestión de documentos, facilitando el diagnóstico
// y monitoreo de sistemas que manejan archivos.
var Document struct {
	// Path representa la ruta donde se encuentra o se guardará el documento.
	Path attribute.Key

	// Filename contiene el nombre del archivo sin la ruta.
	Filename attribute.Key

	// Size registra el tamaño del documento en bytes.
	Size attribute.Key

	// Type identifica el tipo de documento o la operación realizada.
	// Ejemplos: "pdf", "image", "upload", "download", etc.
	Type attribute.Key

	// Success indica si la operación con el documento fue exitosa.
	// Valor booleano: true/false.
	Success attribute.Key

	// ErrorType clasifica el tipo de error ocurrido, si lo hubiera.
	// Ejemplos: "not_found", "permission_denied", "storage_full", etc.
	ErrorType attribute.Key

	// EventFileUploaded marca un evento de carga exitosa de un archivo.
	EventFileUploaded attribute.Key

	// EventFilePathProvided indica que se proporcionó una ruta válida.
	EventFilePathProvided attribute.Key

	// EventPathNotFound señala que la ruta especificada no existe.
	EventPathNotFound attribute.Key

	// EventFileSaved marca un evento de guardado exitoso de un archivo.
	EventFileSaved attribute.Key

	// EventInvalidContent indica que el contenido del archivo no es válido
	// para la operación solicitada.
	EventInvalidContent attribute.Key

	// EventNFSError señala un error en el sistema de archivos de red.
	EventNFSError attribute.Key

	// EventMissingFileOrPath indica que falta el archivo o la ruta necesaria
	// para completar la operación.
	EventMissingFileOrPath attribute.Key
}

func init() {
	// Inicialización de las convenciones semánticas para documentos
	Document.Path = attribute.Key("document.path")
	Document.Filename = attribute.Key("document.filename")
	Document.Size = attribute.Key("document.size")

	Document.Type = attribute.Key("document.type")

	Document.Success = attribute.Key("document.success")

	Document.ErrorType = attribute.Key("document.error_type")

	// Eventos específicos del middleware
	Document.EventFileUploaded = attribute.Key("file_uploaded")
	Document.EventFilePathProvided = attribute.Key("path_valid")
	Document.EventPathNotFound = attribute.Key("path_not_found")
	Document.EventFileSaved = attribute.Key("file_saved")
	Document.EventInvalidContent = attribute.Key("invalid_content_type")
	Document.EventNFSError = attribute.Key("nfs_error")
	Document.EventMissingFileOrPath = attribute.Key("missing_file_or_path")
}
