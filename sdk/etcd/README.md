# Cliente de ETCD

Este paquete proporciona un cliente para interactuar con ETCD que puede ser inyectado en el contexto y utilizado para recuperar variables de configuración.

## Estructura de claves

El cliente sigue el patrón de ruta `/APP/ENV/VAR_KEY` donde:

- `APP`: Nombre de la aplicación
- `ENV`: Entorno (development, testing, production)
- `VAR_KEY`: Clave de la variable

## Características principales

- Cliente configurable mediante opciones funcionales (functional options pattern)
- Soporte para configuración mediante variables de entorno
- Inyección en contexto para facilitar su uso en middlewares
- Cliente por defecto (singleton) para usos simples
- Caché con actualizaciones automáticas (hot-reload)
- Estructura modular y testeable

## Configuración de conexión

El cliente soporta múltiples formas de configurar la conexión a ETCD:

1. **Variables de entorno**:
   - `ETCD_HOST`: Host del servidor ETCD (por defecto: "192.168.31.254")
   - `ETCD_PORT`: Puerto del servidor ETCD (por defecto: "2379")
   - `ETCD_TIMEOUT`: Timeout en segundos (por defecto: 5)
   - `ENV`: Entorno de ejecución (por defecto: "development")

2. **Opciones funcionales**:
   - `WithEndpoints(eps ...string)`: Establece los endpoints del servidor
   - `WithTimeout(t time.Duration)`: Establece el timeout para operaciones
   - `WithApp(name string)`: Establece el nombre de la aplicación
   - `WithEnv(env string)`: Establece el entorno
   - `WithPrefix(p string)`: Establece un prefijo personalizado

## Uso básico

```go
// Crear un cliente personalizado
config := etcd.ClientConfig{
    Timeout:   3 * time.Second,
    AppName:   "app-test",
    Env:       "development",
    Host:      "192.168.31.254", // También puede usar la variable de entorno ETCD_HOST
    Port:      "2379",      // También puede usar la variable de entorno ETCD_PORT
}

cliente, err := etcd.NewClient(config)
if err != nil {
    log.Fatalf("Error al crear cliente: %v", err)
}
defer client.Close()

// Usar el cliente para obtener una variable
ctx := context.Background()
valor, err := client.GetVar(ctx, "api-key")
if err != nil {
    // Manejar error
}
```

## Cliente con caché

La caché permite mantener una copia local de los datos y recibir actualizaciones automáticas cuando cambian los valores en ETCD:

```go
// Crear una caché
cache, err := etcd.NewCache(client, "/config/")
if err != nil {
    log.Fatalf("Error al crear caché: %v", err)
}
defer cache.Close()

// Obtener un valor de la caché (sin necesidad de context)
valor, existe := cache.Get("/config/api-key")
if !existe {
    // Valor no encontrado
}

// Forzar recarga de la caché
if err := cache.Reload(); err != nil {
    log.Printf("Error al recargar caché: %v", err)
}
```

## Middleware HTTP

El paquete incluye un middleware para inyectar el cliente en el contexto de cada request HTTP:

```go
// Crear middleware
middleware := etcd.EtcdMiddleware(client)

// Aplicar middleware a un router/mux
router := http.NewServeMux()
wrappedRouter := middleware(router)

// Uso en handlers
http.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
    etcdClient := etcd.GetEtcdClient(r.Context())
    if etcdClient == nil {
        http.Error(w, "Cliente etcd no disponible", http.StatusInternalServerError)
        return
    }
    
    // Usar el cliente
    valor, err := etcdClient.GetVar(r.Context(), "api-key")
    // ...
})
```

## Métodos principales

### Cliente básico

- `GetVar(ctx, key)`: Obtiene una variable usando el patrón `/APP/ENV/key`
- `SetVar(ctx, key, value)`: Establece una variable usando el patrón `/APP/ENV/key`
- `DeleteVar(ctx, key)`: Elimina una variable
- `Close()`: Cierra la conexión con ETCD

### Cliente con caché

- `Get(key)`: Obtiene un valor de la caché (sin necesidad de context)
- `Reload()`: Fuerza una recarga de la caché desde ETCD
- `Close()`: Libera los recursos de la caché

### Funciones de contexto

- `SetEtcdClient(ctx, client)`: Añade el cliente al contexto
- `GetEtcdClient(ctx)`: Recupera el cliente del contexto

### Cliente por defecto (singleton)

- `InitDefault(opts ...Option)`: Inicializa el cliente global (usar en main)
- `Default()`: Obtiene el cliente global (inicializado previamente)

## Estructura interna

El paquete está organizado en varios componentes:

- `client.go`: Implementación del cliente base
- `cache.go`: Implementación de la caché con hot-reload
- `etcd_middleware.go`: Middleware HTTP para inyección en contexto
- Tests unitarios y de integración para cada componente

## Testing

Para realizar pruebas unitarias con mocks:

```go
// Crear mocks
mockKV := new(MockKV)
mockKV.On("Get", mock.Anything, "test-key", mock.Anything).Return(expectedResponse, nil)

// Crear cliente de prueba
client := &etcd.Client{
    kv: mockKV,
    // ...
}

// Realizar pruebas
// ...

// Verificar llamadas
mockKV.AssertExpectations(t)
``` 