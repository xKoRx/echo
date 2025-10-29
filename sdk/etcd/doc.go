// Package etcd proporciona un cliente para interactuar con ETCD que puede ser
// inyectado en el contexto y utilizado para recuperar variables de configuración.
//
// Estructura de claves:
// El cliente sigue el patrón de ruta `/APP/ENV/VAR_KEY` donde:
//   - `APP`: Nombre de la aplicación
//   - `ENV`: Entorno (development, testing, production)
//   - `VAR_KEY`: Clave de la variable
//
// Características principales:
//   - Cliente configurable mediante opciones funcionales (functional options pattern)
//   - Soporte para configuración mediante variables de entorno
//   - Inyección en contexto para facilitar su uso en middlewares
//   - Cliente por defecto y singleton para usos simples
//   - Caché con actualizaciones automáticas (hot-reload)
//   - Funciones de conveniencia para diferentes tipos de datos
//
// Componentes principales:
//
//   - Cliente principal: Funcionalidad básica para interactuar con etcd
//   - Caché: Mantiene una copia local de los datos con actualización automática
//   - Middleware HTTP: Inyecta el cliente en el contexto de cada request
//
// Ejemplo básico de uso:
//
//	client, err := etcd.New(
//		etcd.WithApp("myapp"),
//		etcd.WithEnv("development"),
//		etcd.WithTimeout(5 * time.Second),
//	)
//	if err != nil {
//		log.Fatalf("Error creating etcd client: %v", err)
//	}
//	defer client.Close()
//
//	// Obtener variables usando diferentes métodos
//	strValue, _ := client.GetVar(ctx, "api-key")
//	intValue, _ := client.GetVarInt(ctx, "max-connections")
//	boolValue, _ := client.GetVarBool(ctx, "feature-enabled")
//	durValue, _ := client.GetVarDuration(ctx, "timeout-ms")
//
//	// Con valores por defecto
//	timeout, _ := client.GetVarDurationWithDefault(ctx, "timeout-ms", 5*time.Second)
//
// Para más información consultar el README.md del paquete.
package etcd
