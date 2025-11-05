---
title: "RFC-002 — Plan de Implementación Iteración 0 (POC 48h)"
version: "1.0"
date: "2025-10-24"
status: "Draft"
authors: ["Aranea Labs - Trading Copier Team"]
depends_on: ["RFC-001"]
---

# RFC-002: Plan de Implementación Iteración 0 (POC 48h)

## 1. Resumen Ejecutivo

Este RFC detalla el **plan de implementación bottom-up** para la **Iteración 0** del sistema Echo, un POC funcional de 48 horas que demuestra la viabilidad técnica del copiador de operaciones.

### Alcance i0 (según RFC-001)
- **1 símbolo único**: XAUUSD hardcoded
- **2 masters MT4 → 2 slaves MT4** en el mismo host Windows
- **Solo órdenes a mercado**: BUY/SELL, CLOSE (sin SL/TP)
- **Lot size fijo hardcoded**: 0.10
- **Sin persistencia**: todo in-memory
- **Sin config externa**: valores hardcoded con `//TODO` para i1
- **Telemetría básica**: SDK echo/telemetry con EchoMetrics bundle
- **Procesamiento secuencial** en Core (sin concurrencia, con `//TODO` para i1)

### Criterios de Éxito
- Latencia p95 intra-host **< 120 ms**
- **0 duplicados** en 10 ejecuciones consecutivas
- **0 cruces de datos** entre cuentas (validar con 2 masters × 2 slaves)
- Métricas E2E visibles (funnel completo)

---

## ⚠️ Nota Importante: Proto v1 vs v0

Este RFC originalmente especificaba proto **v0** en su diseño inicial, pero durante la implementación se decidió usar **v1** para evitar confusión con "versión 0" que podría implicar un prototipo desechable o inestable.

**Decisión técnica**: 
- Usar `echo.v1` como **primera versión estable** del sistema
- Package Go: `github.com/xKoRx/echo/sdk/pb/v1`
- Package proto: `echo.v1`

**Impacto**: 
- ✅ **Ningún impacto funcional** - solo convención de nombrado
- ✅ Mejor percepción de madurez del sistema
- ⚠️ Secciones del RFC que mencionen `v0` deben interpretarse como `v1`

**Ubicación real del código generado**: `sdk/pb/v1/` (no `sdk/proto/v0/` como se menciona en algunas secciones)

**Fecha de decisión**: 2025-10-26  
**Documentado en**: Auditoría AUDIT-CRITICA-ARQUITECTURA-i0.md (Issue #NEW-I1)

---

## 2. Objetivos y No-Objetivos

### Objetivos
1. **🔒 Desarrollar `echo_pipe.dll`**: Prerequisito crítico - DLL C++ para Named Pipes (detalle en 4.1)
2. Validar **arquitectura E2E**: Master EA → Agent → Core → Agent → Slave EA
3. Probar **Named Pipes** (IPC EA↔Agent) y **gRPC bidi** (Agent↔Core)
4. Implementar **dedupe básico** en Core (map in-memory)
5. Establecer **contratos Proto v0** extensibles
6. Validar **routing** sin cruces de datos (4 clientes simultáneos)
7. Medir **latencia** por hop con timestamps + OTEL traces
8. Instrumentación **OTEL desde día 1** con bundle EchoMetrics

### No-Objetivos
- ❌ SL/TP, Money Management, filtros, tolerancias
- ❌ Persistencia en DB (Postgres, Mongo)
- ❌ Config en etcd/YAML
- ❌ Reintentos, error recovery avanzado
- ❌ Multi-símbolo, mapeo de símbolos
- ❌ Concurrencia (procesamiento secuencial)

---

## 3. Arquitectura de la SDK de Echo

**Principio fundamental**: Todo código reutilizable debe desarrollarse en `github.com/xKoRx/echo/sdk` desde el inicio. Agent y Core **solo consumen** la SDK, no reimplementan lógica.

### 3.1 Estructura de Paquetes SDK

```
sdk/
├── proto/v0/                    # Contratos generados
│   ├── common.pb.go
│   ├── trade.pb.go
│   ├── agent.pb.go
│   └── agent_grpc.pb.go
├── domain/                      # Tipos de dominio y validaciones
│   ├── trade.go                # TradeIntent, ExecuteOrder (tipos enriquecidos)
│   ├── validation.go           # Validaciones de negocio
│   ├── transformers.go         # Proto ↔ Domain ↔ JSON
│   └── errors.go               # Error codes y tipos
├── grpc/                        # Cliente/Server gRPC genérico
│   ├── client.go               # Cliente con reconnection, backoff
│   ├── server.go               # Server con middleware básico
│   ├── stream.go               # Helpers para bidi-streaming
│   └── interceptors.go         # Logging, telemetry interceptors
├── ipc/                         # Named Pipes abstraction
│   ├── pipe.go                 # Interface genérica
│   ├── windows_pipe.go         # Implementación Windows
│   ├── reader.go               # JSON line-delimited reader
│   └── writer.go               # JSON line-delimited writer
├── telemetry/                   # Observabilidad
│   ├── client.go               # Cliente OTEL (ya existe)
│   ├── config.go
│   ├── bundle_echo.go          # EchoMetrics bundle
│   ├── traces.go
│   └── semconv.go              # Convenciones semánticas
├── utils/                       # Utilidades comunes
│   ├── uuid.go                 # UUIDv7 generator
│   ├── timestamp.go            # Timestamp helpers
│   └── json.go                 # JSON helpers (parsing, validation)
└── testing/                     # Helpers para testing
    ├── mocks.go                # Mocks de interfaces
    └── fixtures.go             # Fixtures de datos

```

### 3.2 Responsabilidades por Paquete

#### `sdk/proto/v0`
- Código generado por `protoc`
- **NO tocar manualmente** (regenerar con `make proto`)

#### `sdk/domain`
- **Tipos de dominio** enriquecidos (adicionales a proto)
- **Validaciones** de negocio (símbolo válido, lot size, etc.)
- **Transformadores**: Proto ↔ Domain ↔ JSON
- **Error types** custom con contexto

Ejemplo:
```go
type TradeIntent struct {
    *pb.TradeIntent                    // Embedding proto
    ValidationErrors []error           // Errores de validación
}

func (t *TradeIntent) Validate() error { ... }
func ProtoToTradeIntent(p *pb.TradeIntent) *TradeIntent { ... }
func TradeIntentToJSON(t *TradeIntent) ([]byte, error) { ... }
func JSONToTradeIntent(data []byte) (*TradeIntent, error) { ... }
```

#### `sdk/grpc`
- **Cliente gRPC genérico** con:
  - Reconnection con backoff exponencial
  - Health checks
  - Context propagation
  - Interceptors de telemetry
- **Server gRPC genérico** con:
  - Middleware de logging
  - Middleware de telemetry (traces, metrics)
  - Graceful shutdown
- **Stream helpers**:
  - Abstracción de Send/Recv con channels
  - Error handling
  - Serialización de writes

Ejemplo:
```go
type StreamClient struct {
    stream pb.AgentService_StreamBidiClient
    sendCh chan proto.Message
    recvCh chan proto.Message
    errCh  chan error
}

func NewStreamClient(stream pb.AgentService_StreamBidiClient) *StreamClient { ... }
func (s *StreamClient) Send(msg proto.Message) error { ... }
func (s *StreamClient) Receive() <-chan proto.Message { ... }
```

#### `sdk/ipc`
- **Interface genérica** de Named Pipes (cross-platform ready para futuro)
- **Implementación Windows** usando syscalls o DLL
- **JSON line-delimited reader/writer**:
  - Buffering
  - Timeout handling
  - Reconnection
  
Ejemplo:
```go
type Pipe interface {
    Read(timeout time.Duration) ([]byte, error)
    Write(data []byte) error
    Close() error
}

type WindowsPipe struct { ... }

type JSONReader struct {
    pipe   Pipe
    buffer *bufio.Scanner
}

func (r *JSONReader) ReadMessage() (map[string]interface{}, error) { ... }
```

#### `sdk/telemetry`
- Ya existe, ajustar para agregar `bundle_echo.go`
- **EchoMetrics** con métricas del funnel
- **Convenciones semánticas** específicas de Echo

#### `sdk/utils`
- **UUIDv7 generator**: 
```go
func GenerateUUIDv7() string { ... }
```
- **Timestamp helpers**:
```go
func NowUnixMilli() int64 { ... }
func AddTimestamp(ts *pb.TimestampMetadata, field string, value int64) { ... }
```
- **JSON helpers**:
```go
func ValidateJSON(data []byte) error { ... }
func PrettyPrint(data []byte) string { ... }
```

---

## 4. Orden de Desarrollo (Bottom-Up)

### Fase 0: DLL de Named Pipes (2-4h) ⚠️ **CRÍTICO**
**Objetivo**: Crear la DLL que permite a MQL4 comunicarse con Named Pipes de Windows

**⚠️ Nota**: Esta es la **única dependencia externa** del proyecto. Sin esta DLL, las EAs no pueden comunicarse con el Agent.

**Entregables**:
- [ ] **`echo_pipe.dll`** compilada para Windows x64
- [ ] Código fuente C++ documentado (`echo_pipe.cpp`)
- [ ] Tests básicos (programa C++ de prueba)
- [ ] Instrucciones de compilación
- [ ] Script de build automatizado (opcional)

**Criterios**:
- DLL carga correctamente en MT4 (no crashea)
- Funciones exportadas visibles con `dumpbin /exports echo_pipe.dll`
- Test program puede crear pipe, conectar, leer, escribir, cerrar
- DLL funciona en MT4 build 600+ (32-bit y 64-bit según broker)

**Ver sección 4.1 para especificación completa de la DLL**

---

### Fase 1: SDK Foundation (6-8h)
**Objetivo**: Construir toda la infraestructura reutilizable

**Entregables**:
- [ ] **Proto**: `trade.proto`, `common.proto`, `agent.proto` + generación
- [ ] **sdk/domain**: 
  - [ ] `trade.go`: tipos enriquecidos (TradeIntent, ExecuteOrder, etc.)
  - [ ] `validation.go`: ValidateSymbol, ValidateLotSize, etc.
  - [ ] `transformers.go`: Proto ↔ Domain ↔ JSON
  - [ ] `errors.go`: ErrorCode mapping, custom errors
- [ ] **sdk/grpc**:
  - [ ] `client.go`: cliente genérico con dial + options
  - [ ] `server.go`: server genérico con register + serve
  - [ ] `stream.go`: StreamClient/StreamServer abstractions
  - [ ] `interceptors.go`: logging + telemetry interceptors
- [ ] **sdk/ipc**:
  - [ ] `pipe.go`: interface Pipe
  - [ ] `windows_pipe.go`: implementación Windows (syscall o DLL)
  - [ ] `reader.go`: JSONReader con buffering
  - [ ] `writer.go`: JSONWriter con serialización
- [ ] **sdk/telemetry**:
  - [ ] `bundle_echo.go`: EchoMetrics con 6 counters + 6 histogramas
  - [ ] `semconv.go`: atributos Echo (trade_id, command_id, etc.)
- [ ] **sdk/utils**:
  - [ ] `uuid.go`: GenerateUUIDv7()
  - [ ] `timestamp.go`: helpers de timestamp
  - [ ] `json.go`: ValidateJSON, helpers

**Criterios**:
- `make proto` genera código sin errores
- Tests unitarios para cada paquete SDK (>= 80% coverage)
- Todos los paquetes exponen interfaces, no implementaciones concretas donde sea posible
- Documentación godoc completa (cada función pública documentada)

---

### Fase 2: Slave EA Mínimo (6-8h)
**Entregables**:
- [ ] EA MQL4 que conecta a Named Pipe del Agent
- [ ] Envía mensaje de **handshake** con metadata (account_id, role=slave)
- [ ] Recibe comando `ExecuteOrder`, llama `OrderSend`, reporta `ExecutionResult`
- [ ] Envía telemetría básica (timestamps por evento)

**Criterios**:
- Conecta a pipe `\\.\pipe\echo_slave_<account_id>` sin errores
- Ejecuta market order en cuenta demo y reporta ticket
- Logs en Expert tab visibles

**Ver sección 9 para prompt detallado**

---

### Fase 3: Agent Mínimo (8-12h)
**Objetivo**: Construir el bridge entre EAs (Named Pipes) y Core (gRPC) **usando exclusivamente SDK**

**Entregables**:
- [ ] Binario Go que arranca como proceso (servicio Windows en i1)
- [ ] Servidor Named Pipes usando **`sdk/ipc`** (1 pipe por EA: `echo_master_<id>`, `echo_slave_<id>`)
- [ ] Cliente gRPC al Core usando **`sdk/grpc.StreamClient`** (stream bidi persistente)
- [ ] Routing usando **`sdk/domain`** transformers:
  - pipe → stream: JSON → Domain → Proto (TradeIntent)
  - stream → pipe: Proto → Domain → JSON (ExecuteOrder)
- [ ] Telemetría usando **`sdk/telemetry.EchoMetrics`**: logs estructurados + métricas de Agent

**Criterios**:
- Agent **NO reimplementa** lógica de pipes, gRPC o transformaciones (todo vía SDK)
- Crea pipes y acepta conexiones de EAs
- Lee JSON line-delimited sin corrupción (usando `sdk/ipc.JSONReader`)
- Stream gRPC se mantiene abierto > 1 min sin errores (usando `sdk/grpc.StreamClient`)
- Logs muestran mensajes entrantes/salientes
- Métricas `echo.intent.received`, `echo.intent.forwarded` registradas

---

### Fase 4: Master EA Mínimo (4-6h)
**Entregables**:
- [ ] EA MQL4 que conecta a Named Pipe del Agent
- [ ] Handshake con metadata (account_id, role=master)
- [ ] Botón manual "BUY" → genera `TradeIntent` con UUIDv7
- [ ] Reporta cierre de orden cuando detecta posición cerrada

**Criterios**:
- Click en botón genera TradeIntent bien formado
- JSON válido en pipe (validar con logs del Agent)

**Ver sección 9 para prompt detallado**

---

### Fase 5: Core Mínimo (8-12h)
**Objetivo**: Construir la orquestación central **usando exclusivamente SDK**

**Entregables**:
- [ ] Servidor gRPC bidi usando **`sdk/grpc.StreamServer`** que acepta streams de Agents
- [ ] Router que recibe `TradeIntent`, valida con **`sdk/domain.Validate()`**, deduplica
- [ ] Map de dedupe: `map[trade_id]*DedupeEntry` con TTL 1h
- [ ] Procesamiento **secuencial** (canal o lock global, con `//TODO` para i1)
- [ ] Transforma usando **`sdk/domain.TradeIntentToExecuteOrder()`** (lot size = 0.10 hardcoded)
- [ ] Envía `ExecuteOrder` al Agent correspondiente
- [ ] Telemetría usando **`sdk/telemetry.EchoMetrics`**: logs estructurados + métricas

**Criterios**:
- Core **NO reimplementa** lógica de gRPC, validaciones o transformaciones (todo vía SDK)
- Acepta múltiples streams de Agents simultáneos
- Rechaza duplicados (mismo trade_id)
- Procesa intents en orden FIFO (secuencial en i0)
- Logs muestran flujo completo de procesamiento
- Métricas `echo.order.created`, `echo.order.sent`, `echo.execution.completed` registradas

---

### Fase 6: Integración E2E (6-8h)
**Entregables**:
- [ ] Configuración con 2 masters y 2 slaves en mismo host
- [ ] Scripts de arranque (Agent, Core, 4 terminales MT4)
- [ ] Tests manuales: BUY desde Master1 → ejecuta en Slave1 y Slave2
- [ ] Validación de latencia E2E con timestamps
- [ ] Dashboard Grafana básico (opcional para i0)

**Criterios**:
- 10 ejecuciones consecutivas sin duplicados
- 0 cruces de datos entre cuentas
- p95 latency < 120ms
- Métricas visibles en logs o Prometheus

---

## 4.1 Especificación Completa: `echo_pipe.dll`

### 4.1.1 ¿Por qué necesitamos esta DLL?

MQL4 **NO tiene soporte nativo** para Named Pipes. Named Pipes son el mecanismo de IPC más eficiente en Windows para comunicación entre procesos en el mismo host, pero MQL4 solo puede accederlos mediante una DLL externa escrita en C/C++.

**Alternativas descartadas**:
- ❌ **TCP Sockets**: MQL4 build 600+ los soporta, pero tienen mayor latencia y overhead
- ❌ **Files compartidos**: Polling lento, race conditions, no eficiente
- ❌ **COM/DDE**: Muy complejo, legacy, poco mantenible

**Named Pipes con DLL** es la solución óptima para i0.

---

### 4.1.2 Código Fuente Completo: `echo_pipe.cpp`

```cpp
/*
 * echo_pipe.dll - Named Pipes IPC para MetaTrader 4/5
 * 
 * Permite a EAs MQL4 comunicarse con el Agent de Echo via Named Pipes.
 * 
 * Compilar con:
 *   x64: cl /LD /O2 echo_pipe.cpp /Fe:echo_pipe_x64.dll
 *   x86: cl /LD /O2 echo_pipe.cpp /Fe:echo_pipe_x86.dll
 * 
 * Versión: 1.0.0
 * Fecha: 2025-10-24
 */

#include <windows.h>
#include <stdio.h>
#include <string.h>

// ============================================================================
// FUNCIÓN 1: ConnectPipe
// ============================================================================
// Conecta a un Named Pipe existente creado por el Agent (cliente)
// 
// Parámetros:
//   - pipeName: Nombre del pipe (ej: "\\.\pipe\echo_master_12345")
// 
// Retorna:
//   - Handle del pipe (int > 0) si éxito
//   - -1 si error
// 
extern "C" __declspec(dllexport) int __stdcall ConnectPipe(const wchar_t* pipeName)
{
    if (pipeName == NULL) {
        return -1;
    }

    HANDLE hPipe = CreateFileW(
        pipeName,                   // Nombre del pipe
        GENERIC_READ | GENERIC_WRITE, // Acceso lectura/escritura
        0,                          // No compartir
        NULL,                       // Seguridad por defecto
        OPEN_EXISTING,              // El pipe ya debe existir
        FILE_ATTRIBUTE_NORMAL,      // Atributos normales
        NULL                        // No template
    );

    if (hPipe == INVALID_HANDLE_VALUE) {
        // Opcional: loggear GetLastError() para debugging
        return -1;
    }

    // Configurar modo de lectura mensaje por mensaje (line-delimited)
    DWORD mode = PIPE_READMODE_BYTE; // Leer en modo byte (no mensaje completo)
    SetNamedPipeHandleState(hPipe, &mode, NULL, NULL);

    return (int)(INT_PTR)hPipe; // Castear handle a int para MQL4
}

// ============================================================================
// FUNCIÓN 2: WritePipe
// ============================================================================
// Escribe datos en el pipe
// 
// Parámetros:
//   - handle: Handle del pipe retornado por ConnectPipe
//   - data: String a enviar (JSON line-delimited, debe terminar en \n)
// 
// Retorna:
//   - Número de bytes escritos si éxito (> 0)
//   - -1 si error
// 
extern "C" __declspec(dllexport) int __stdcall WritePipe(int handle, const char* data)
{
    if (handle <= 0 || data == NULL) {
        return -1;
    }

    HANDLE hPipe = (HANDLE)(INT_PTR)handle;
    DWORD bytesWritten = 0;
    DWORD dataLen = (DWORD)strlen(data);

    BOOL result = WriteFile(
        hPipe,
        data,
        dataLen,
        &bytesWritten,
        NULL // Sin overlapped I/O
    );

    if (!result) {
        return -1;
    }

    // Flush para asegurar que datos se envían inmediatamente
    FlushFileBuffers(hPipe);

    return (int)bytesWritten;
}

// ============================================================================
// FUNCIÓN 3: ReadPipeLine
// ============================================================================
// Lee una línea completa del pipe (hasta \n o hasta llenar buffer)
// 
// Parámetros:
//   - handle: Handle del pipe
//   - buffer: Buffer donde se almacenarán los datos leídos
//   - bufferSize: Tamaño máximo del buffer
// 
// Retorna:
//   - Número de bytes leídos si éxito (> 0)
//   - 0 si no hay datos disponibles (timeout)
//   - -1 si error
// 
// Nota: Esta función lee byte a byte hasta encontrar \n o llenar el buffer.
//       Es ineficiente pero simple para i0. En i1+ optimizar con buffering.
// 
extern "C" __declspec(dllexport) int __stdcall ReadPipeLine(int handle, char* buffer, int bufferSize)
{
    if (handle <= 0 || buffer == NULL || bufferSize <= 0) {
        return -1;
    }

    HANDLE hPipe = (HANDLE)(INT_PTR)handle;
    int totalBytesRead = 0;

    // Leer byte a byte hasta encontrar \n o llenar buffer
    while (totalBytesRead < bufferSize - 1) {
        DWORD bytesRead = 0;
        char byte;

        BOOL result = ReadFile(
            hPipe,
            &byte,
            1,
            &bytesRead,
            NULL
        );

        if (!result) {
            // Error de lectura
            if (totalBytesRead > 0) {
                break; // Retornar lo que se leyó hasta ahora
            }
            return -1;
        }

        if (bytesRead == 0) {
            // No hay más datos disponibles (pipe cerrado o timeout)
            break;
        }

        buffer[totalBytesRead++] = byte;

        // Si encontramos \n, terminamos la línea (incluimos el \n)
        if (byte == '\n') {
            break;
        }
    }

    // Null-terminate el string
    buffer[totalBytesRead] = '\0';

    return totalBytesRead;
}

// ============================================================================
// FUNCIÓN 4: ClosePipe
// ============================================================================
// Cierra el handle del pipe
// 
// Parámetros:
//   - handle: Handle del pipe retornado por ConnectPipe
// 
extern "C" __declspec(dllexport) void __stdcall ClosePipe(int handle)
{
    if (handle <= 0) {
        return;
    }

    HANDLE hPipe = (HANDLE)(INT_PTR)handle;
    CloseHandle(hPipe);
}

// ============================================================================
// DllMain - Punto de entrada de la DLL
// ============================================================================
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved)
{
    switch (ul_reason_for_call)
    {
    case DLL_PROCESS_ATTACH:
        // Inicialización (si fuera necesaria)
        break;
    case DLL_THREAD_ATTACH:
    case DLL_THREAD_DETACH:
    case DLL_PROCESS_DETACH:
        break;
    }
    return TRUE;
}
```

---

### 4.1.3 Instrucciones de Compilación

#### Opción A: Visual Studio (Recomendado)

**Requisitos**:
- Visual Studio 2019+ (Community Edition es gratuita)
- Windows SDK instalado

**Pasos**:
1. Abrir "Developer Command Prompt for VS 2019" (o tu versión)
2. Navegar al directorio con `echo_pipe.cpp`
3. Compilar para x64:
```cmd
cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe_x64.dll /link /DEF:echo_pipe.def
```
4. Compilar para x86 (si tu broker usa MT4 de 32-bit):
```cmd
cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe_x86.dll /link /DEF:echo_pipe.def
```

**Flags explicados**:
- `/LD`: Crear DLL
- `/O2`: Optimización máxima
- `/EHsc`: Habilitar excepciones C++
- `/Fe:`: Nombre del archivo de salida
- `/link /DEF:`: Usar archivo .def para exports (opcional, ya usamos `__declspec(dllexport)`)

#### Opción B: MinGW (Para desarrollo en Linux/WSL)

```bash
# Instalar MinGW
sudo apt-get install mingw-w64

# Compilar para x64
x86_64-w64-mingw32-g++ -shared -o echo_pipe_x64.dll echo_pipe.cpp -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias

# Compilar para x86
i686-w64-mingw32-g++ -shared -o echo_pipe_x86.dll echo_pipe.cpp -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias
```

**Flags explicados**:
- `-shared`: Crear shared library (DLL)
- `-static-libgcc -static-libstdc++`: Linkear estáticamente (no requiere runtime DLLs)
- `-Wl,--add-stdcall-alias`: Generar aliases para convenciones de llamada

#### Opción C: CMake (Para proyectos más grandes)

Crear `CMakeLists.txt`:
```cmake
cmake_minimum_required(VERSION 3.10)
project(EchoPipeDLL)

add_library(echo_pipe SHARED echo_pipe.cpp)

# Configurar para Windows
if(WIN32)
    target_compile_definitions(echo_pipe PRIVATE _WIN32)
endif()

# Output name
set_target_properties(echo_pipe PROPERTIES OUTPUT_NAME "echo_pipe")
```

Compilar:
```bash
mkdir build && cd build
cmake .. -G "Visual Studio 16 2019" -A x64
cmake --build . --config Release
```

---

### 4.1.4 Verificación de la DLL

#### 1. Verificar exports con `dumpbin` (Visual Studio)

```cmd
dumpbin /exports echo_pipe_x64.dll
```

**Salida esperada**:
```
    ordinal hint RVA      name
          1    0 00001000 ClosePipe
          2    1 00001020 ConnectPipe
          3    2 00001040 ReadPipeLine
          4    3 00001060 WritePipe
```

#### 2. Verificar exports con `objdump` (MinGW)

```bash
objdump -p echo_pipe_x64.dll | grep "Export"
```

#### 3. Test program en C++ (`test_pipe.cpp`)

```cpp
#include <windows.h>
#include <stdio.h>

// Import de funciones de la DLL
typedef int (__stdcall *ConnectPipeFunc)(const wchar_t*);
typedef int (__stdcall *WritePipeFunc)(int, const char*);
typedef int (__stdcall *ReadPipeLineFunc)(int, char*, int);
typedef void (__stdcall *ClosePipeFunc)(int);

int main() {
    // Cargar DLL
    HMODULE hDll = LoadLibrary(L"echo_pipe_x64.dll");
    if (!hDll) {
        printf("ERROR: No se pudo cargar la DLL\n");
        return 1;
    }

    // Obtener funciones
    ConnectPipeFunc ConnectPipe = (ConnectPipeFunc)GetProcAddress(hDll, "ConnectPipe");
    WritePipeFunc WritePipe = (WritePipeFunc)GetProcAddress(hDll, "WritePipe");
    ReadPipeLineFunc ReadPipeLine = (ReadPipeLineFunc)GetProcAddress(hDll, "ReadPipeLine");
    ClosePipeFunc ClosePipe = (ClosePipeFunc)GetProcAddress(hDll, "ClosePipe");

    if (!ConnectPipe || !WritePipe || !ReadPipeLine || !ClosePipe) {
        printf("ERROR: No se pudieron obtener las funciones\n");
        FreeLibrary(hDll);
        return 1;
    }

    // Test 1: Conectar a pipe (debe fallar si Agent no está corriendo)
    int handle = ConnectPipe(L"\\\\.\\pipe\\echo_master_12345");
    if (handle <= 0) {
        printf("INFO: No se pudo conectar al pipe (esperado si Agent no corre)\n");
    } else {
        printf("OK: Conectado al pipe, handle=%d\n", handle);

        // Test 2: Escribir JSON
        const char* json = "{\"type\":\"handshake\",\"timestamp_ms\":12345}\n";
        int written = WritePipe(handle, json);
        printf("OK: Escritos %d bytes\n", written);

        // Test 3: Leer respuesta (si Agent responde)
        char buffer[1024];
        int read = ReadPipeLine(handle, buffer, sizeof(buffer));
        if (read > 0) {
            printf("OK: Leídos %d bytes: %s\n", read, buffer);
        }

        // Test 4: Cerrar
        ClosePipe(handle);
        printf("OK: Pipe cerrado\n");
    }

    FreeLibrary(hDll);
    printf("OK: Todos los tests pasaron\n");
    return 0;
}
```

Compilar y ejecutar:
```cmd
cl test_pipe.cpp
test_pipe.exe
```

---

### 4.1.5 Instalación en MetaTrader

1. **Copiar DLL a la carpeta correcta**:
   - Abrir MT4/MT5
   - Menú: File → Open Data Folder
   - Navegar a: `MQL4/Libraries/` (o `MQL5/Libraries/` para MT5)
   - Copiar `echo_pipe_x64.dll` o `echo_pipe_x86.dll` (según tu MT4)
   - **Renombrar** a `echo_pipe.dll` (sin sufijo x64/x86)

2. **Habilitar DLL imports en MT4**:
   - Tools → Options → Expert Advisors
   - ✅ Marcar "Allow DLL imports"
   - ✅ Marcar "Allow WebRequest for listed URL" (opcional)

3. **Verificar que MT4 puede cargar la DLL**:
   Crear EA de prueba (`TestDLL.mq4`):
   ```mql4
   #property strict
   
   #import "echo_pipe.dll"
      int ConnectPipe(string pipeName);
      void ClosePipe(int handle);
   #import
   
   void OnInit() {
       Print("Intentando cargar echo_pipe.dll...");
       int handle = ConnectPipe("\\\\.\\pipe\\echo_test");
       if (handle > 0) {
           Print("OK: DLL cargada, handle=", handle);
           ClosePipe(handle);
       } else {
           Print("INFO: DLL cargada pero pipe no existe (esperado)");
       }
   }
   ```

   Compilar y ejecutar en un chart. Revisar pestaña "Experts" en MT4.

---

### 4.1.6 Troubleshooting Común

| Problema | Causa | Solución |
|----------|-------|----------|
| MT4 no carga la DLL | DLL de 64-bit en MT4 de 32-bit (o viceversa) | Compilar DLL con la arquitectura correcta (x86 vs x64) |
| "The specified module could not be found" | DLL depende de runtime DLLs no instaladas | Compilar con `-static-libgcc -static-libstdc++` (MinGW) o usar Visual Studio con runtime estático |
| Crash al llamar función | Convención de llamada incorrecta | Usar `__stdcall` en C++ y verificar import en MQL4 |
| `ConnectPipe()` retorna -1 | Agent no está corriendo o pipe name incorrecto | Verificar que Agent creó el pipe con el nombre exacto |
| Lectura se bloquea indefinidamente | `ReadPipeLine` sin timeout | Implementar timeout en versión futura (i1+) |

---

### 4.1.7 Checklist: ¿Cómo llegar a tener la DLL funcionando?

**Paso 1: Setup de desarrollo** (30min)
- [ ] Instalar Visual Studio 2019+ Community Edition (o MinGW en Linux)
- [ ] Verificar que Windows SDK está instalado
- [ ] Abrir "Developer Command Prompt for VS"

**Paso 2: Código fuente** (15min)
- [ ] Crear archivo `echo_pipe.cpp` con el código de la sección 4.1.2
- [ ] Guardar en un directorio limpio (ej: `C:\dev\echo-pipe-dll\`)

**Paso 3: Compilación** (15min)
- [ ] Compilar para x64: `cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe_x64.dll`
- [ ] (Opcional) Compilar para x86: usar "x86 Native Tools Command Prompt"
- [ ] Verificar que se generó `echo_pipe_x64.dll` en el directorio

**Paso 4: Verificación** (15min)
- [ ] Ejecutar `dumpbin /exports echo_pipe_x64.dll`
- [ ] Verificar que aparecen 4 funciones: ConnectPipe, WritePipe, ReadPipeLine, ClosePipe
- [ ] (Opcional) Compilar y ejecutar `test_pipe.cpp`

**Paso 5: Instalación en MT4** (10min)
- [ ] Abrir MT4 → File → Open Data Folder
- [ ] Copiar `echo_pipe_x64.dll` a `MQL4/Libraries/`
- [ ] Renombrar a `echo_pipe.dll`
- [ ] Habilitar "Allow DLL imports" en Tools → Options

**Paso 6: Test en MT4** (15min)
- [ ] Compilar EA de prueba `TestDLL.mq4` (código en 4.1.5)
- [ ] Cargar EA en un chart
- [ ] Verificar en pestaña "Experts" que no hay errores de carga de DLL
- [ ] Si DLL carga OK, ver mensaje "DLL cargada pero pipe no existe (esperado)"

**Paso 7: Distribuir** (5min)
- [ ] Copiar `echo_pipe.dll` a carpeta del proyecto: `echo/clients/libraries/`
- [ ] Commit en git con mensaje: "feat: add echo_pipe.dll for Named Pipes IPC"
- [ ] Documentar versión de compilador y flags usados en README

**Tiempo total estimado: 2-3 horas** (incluyendo instalación de Visual Studio si no está instalado)

---

### 4.1.8 Mejoras Futuras (Post-i0)

**Iteración 1+**:
- [ ] Agregar función `ReadPipeLineTimeout(handle, buffer, bufferSize, timeoutMs)` con timeout configurable
- [ ] Buffering interno para lectura más eficiente (leer bloques grandes, consumir línea a línea)
- [ ] Logging interno de errores a archivo (para debugging sin acceso a MT4 logs)
- [ ] Función `GetLastPipeError()` que retorne código de error Win32 detallado
- [ ] Soporte para Named Pipes en modo asíncrono (overlapped I/O)
- [ ] Cross-platform: Linux con Unix Domain Sockets (wrapper compatible)

---

## 4.2 Contratos Proto V0

### 4.2.1 common.proto

```protobuf
syntax = "proto3";

package echo.v0;

option go_package = "github.com/xKoRx/echo/sdk/proto/v0;echov0";

// OrderSide indica la dirección de la orden (compra o venta)
enum OrderSide {
  ORDER_SIDE_UNSPECIFIED = 0;  // Forzar validación explícita
  BUY = 1;                      // Orden de compra
  SELL = 2;                     // Orden de venta
}

// OrderStatus indica el estado de una orden
enum OrderStatus {
  ORDER_STATUS_UNSPECIFIED = 0;
  PENDING = 1;                  // Esperando procesamiento
  FILLED = 2;                   // Ejecutada exitosamente
  REJECTED = 3;                 // Rechazada por broker/validación
  CANCELLED = 4;                // Cancelada
}

// ErrorCode mapea códigos de error de brokers MT4/MT5
enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;
  ERR_NO_ERROR = 1;              // Éxito (MT4: 0, pero 0 no es válido en proto3 enum)
  ERR_INVALID_PRICE = 129;       // MT4: 129
  ERR_INVALID_STOPS = 130;       // MT4: 130
  ERR_OFF_QUOTES = 136;          // MT4: 136
  ERR_REQUOTE = 138;             // MT4: 138
  ERR_BROKER_BUSY = 137;         // MT4: 137
  ERR_TIMEOUT = 141;             // MT4: 141
  ERR_TRADE_DISABLED = 133;      // MT4: 133
  ERR_MARKET_CLOSED = 132;       // MT4: 132
  ERR_NOT_ENOUGH_MONEY = 134;    // MT4: 134
  ERR_PRICE_CHANGED = 135;       // MT4: 135
  ERR_UNKNOWN = 999;             // Error desconocido
}

// Metadata de tiempo para latencia E2E
message TimestampMetadata {
  int64 t0_master_ea_ms = 1;      // Master EA genera intent
  int64 t1_agent_recv_ms = 2;     // Agent recibe de pipe
  int64 t2_core_recv_ms = 3;      // Core recibe de stream
  int64 t3_core_send_ms = 4;      // Core envía ExecuteOrder
  int64 t4_agent_recv_ms = 5;     // Agent recibe ExecuteOrder
  int64 t5_slave_ea_recv_ms = 6;  // Slave EA recibe comando
  int64 t6_order_send_ms = 7;     // Slave EA llama OrderSend
  int64 t7_order_filled_ms = 8;   // Slave EA recibe ticket/fill
}
```

---

### 4.2.2 trade.proto

```protobuf
syntax = "proto3";

package echo.v0;

option go_package = "github.com/xKoRx/echo/sdk/proto/v0;echov0";

import "common.proto";

// TradeIntent: mensaje del Master EA al Core vía Agent
message TradeIntent {
  string trade_id = 1;            // UUIDv7 (generado por Master EA)
  int64 timestamp_ms = 2;         // Timestamp Unix ms (GetTickCount en MQL)
  string client_id = 3;           // ID del Master EA (ej: "master_12345")
  string account_id = 4;          // ID de la cuenta master (ej: "12345")
  string symbol = 5;              // Símbolo (ej: "XAUUSD")
  OrderSide order_side = 6;       // BUY o SELL
  double lot_size = 7;            // Tamaño en lotes del master (ignorado en i0)
  double price = 8;               // Precio de entrada del master
  int64 magic_number = 9;         // MagicNumber de la estrategia
  int32 ticket = 10;              // Ticket MT4/MT5 del master
  
  // Campos opcionales para i1+ (SL/TP)
  optional double stop_loss = 11;
  optional double take_profit = 12;
  
  int32 attempt = 13;             // Número de intento (para reintentos en i1+)
  
  // Metadata de latencia
  TimestampMetadata timestamps = 14;
}

// ExecuteOrder: comando del Core al Slave EA vía Agent
message ExecuteOrder {
  string command_id = 1;          // UUID único del comando (generado por Core)
  string trade_id = 2;            // trade_id original del TradeIntent
  string client_id = 3;           // ID del Slave EA (ej: "slave_67890")
  string account_id = 4;          // ID de la cuenta slave
  string symbol = 5;              // Símbolo (XAUUSD en i0)
  OrderSide order_side = 6;       // BUY o SELL
  double lot_size = 7;            // Tamaño calculado (0.10 en i0)
  int64 magic_number = 8;         // MagicNumber a replicar (mismo del master)
  
  // Campos opcionales para i1+ (SL/TP)
  optional double stop_loss = 9;
  optional double take_profit = 10;
  
  // Metadata de latencia
  TimestampMetadata timestamps = 11;
}

// ExecutionResult: respuesta del Slave EA al Core vía Agent
message ExecutionResult {
  string command_id = 1;          // command_id del ExecuteOrder
  string trade_id = 2;            // trade_id original
  string client_id = 3;           // ID del Slave EA
  bool success = 4;               // true si OrderSend retornó ticket > 0
  int32 ticket = 5;               // Ticket MT4/MT5 del slave (0 si error)
  ErrorCode error_code = 6;       // Código de error (ERR_NO_ERROR si success)
  string error_message = 7;       // Mensaje de error detallado
  optional double executed_price = 8; // Precio de ejecución (Bid/Ask)
  
  // Metadata de latencia
  TimestampMetadata timestamps = 9;
}

// TradeClose: evento de cierre del Master EA
message TradeClose {
  string close_id = 1;            // UUID único del evento de cierre
  int64 timestamp_ms = 2;         // Timestamp del cierre
  string client_id = 3;           // ID del Master EA
  string account_id = 4;          // ID de la cuenta master
  int32 ticket = 5;               // Ticket del master que cerró
  int64 magic_number = 6;         // MagicNumber de la estrategia
  double close_price = 7;         // Precio de cierre
  string symbol = 8;              // Símbolo
}

// CloseOrder: comando del Core al Slave EA para cerrar posición
message CloseOrder {
  string command_id = 1;          // UUID único del comando
  string close_id = 2;            // close_id del TradeClose original
  string client_id = 3;           // ID del Slave EA
  string account_id = 4;          // ID de la cuenta slave
  int32 ticket = 5;               // Ticket del slave a cerrar (0 = cerrar por magic)
  int64 magic_number = 6;         // MagicNumber (si ticket=0, cerrar por magic)
  string symbol = 7;              // Símbolo
  
  // Metadata de latencia
  TimestampMetadata timestamps = 8;
}

// CloseResult: respuesta del Slave EA tras cerrar
message CloseResult {
  string command_id = 1;          // command_id del CloseOrder
  string close_id = 2;            // close_id original
  string client_id = 3;           // ID del Slave EA
  bool success = 4;               // true si OrderClose retornó true
  int32 ticket = 5;               // Ticket cerrado
  ErrorCode error_code = 6;       // Código de error
  string error_message = 7;       // Mensaje de error
  double close_price = 8;         // Precio de cierre
  
  // Metadata de latencia
  TimestampMetadata timestamps = 9;
}
```

---

### 4.2.3 agent.proto

```protobuf
syntax = "proto3";

package echo.v0;

option go_package = "github.com/xKoRx/echo/sdk/proto/v0;echov0";

import "trade.proto";
import "common.proto";

// AgentService: servicio gRPC bidireccional
service AgentService {
  // StreamBidi: stream persistente Agent ↔ Core
  rpc StreamBidi(stream AgentMessage) returns (stream CoreMessage);
  
  // Ping: healthcheck simple
  rpc Ping(PingRequest) returns (PingResponse);
}

// AgentMessage: mensajes enviados por Agent al Core
message AgentMessage {
  oneof payload {
    TradeIntent trade_intent = 1;
    ExecutionResult execution_result = 2;
    TradeClose trade_close = 3;
    CloseResult close_result = 4;
    AgentHeartbeat heartbeat = 5;  // Para i1+
  }
}

// CoreMessage: mensajes enviados por Core al Agent
message CoreMessage {
  oneof payload {
    ExecuteOrder execute_order = 1;
    CloseOrder close_order = 2;
    CoreHeartbeat heartbeat = 3;  // Para i1+
  }
}

// AgentHeartbeat: heartbeat del Agent (i1+)
message AgentHeartbeat {
  string agent_id = 1;
  int64 timestamp_ms = 2;
  // Estado de cuentas, equity, etc. (i1+)
}

// CoreHeartbeat: heartbeat del Core (i1+)
message CoreHeartbeat {
  int64 timestamp_ms = 1;
}

// Ping/Pong para healthcheck
message PingRequest {
  int64 timestamp_ms = 1;
}

message PingResponse {
  int64 timestamp_ms = 1;
  int64 server_time_ms = 2;
}
```

---

## 5. Protocolo Named Pipes (IPC EA ↔ Agent)

### 5.1 Especificación

**Transporte**: Named Pipes de Windows
**Formato**: JSON line-delimited (cada mensaje termina con `\n`)
**Codificación**: UTF-8
**Nombres de pipes**:
- Master: `\\.\pipe\echo_master_<account_id>`
- Slave: `\\.\pipe\echo_slave_<account_id>`

### 5.2 Estructura de Mensajes

#### Mensaje Base (JSON)
```json
{
  "type": "string",          // Tipo de mensaje (ver tipos abajo)
  "timestamp_ms": 0,         // Timestamp Unix ms
  "payload": {}              // Contenido específico del tipo
}
```

#### Tipos de Mensajes

**1. Handshake (EA → Agent al conectar)**
```json
{
  "type": "handshake",
  "timestamp_ms": 1698345600000,
  "payload": {
    "client_id": "master_12345",
    "account_id": "12345",
    "broker": "icmarkets",
    "role": "master",          // "master" o "slave"
    "symbol": "XAUUSD",
    "version": "0.1.0"
  }
}
```

**2. TradeIntent (Master EA → Agent)**
```json
{
  "type": "trade_intent",
  "timestamp_ms": 1698345601000,
  "payload": {
    "trade_id": "01HKQV8Y9GJ3F5R6WN8P2M4D1E",  // UUIDv7
    "client_id": "master_12345",
    "account_id": "12345",
    "symbol": "XAUUSD",
    "order_side": "BUY",       // "BUY" o "SELL"
    "lot_size": 0.01,
    "price": 2045.50,
    "magic_number": 123456,
    "ticket": 987654,
    "timestamps": {
      "t0_master_ea_ms": 1698345601000
    }
  }
}
```

**3. ExecuteOrder (Agent → Slave EA)**
```json
{
  "type": "execute_order",
  "timestamp_ms": 1698345601050,
  "payload": {
    "command_id": "01HKQV8YABCDEF123456789XYZ",
    "trade_id": "01HKQV8Y9GJ3F5R6WN8P2M4D1E",
    "client_id": "slave_67890",
    "account_id": "67890",
    "symbol": "XAUUSD",
    "order_side": "BUY",
    "lot_size": 0.10,
    "magic_number": 123456,
    "timestamps": {
      "t0_master_ea_ms": 1698345601000,
      "t1_agent_recv_ms": 1698345601010,
      "t2_core_recv_ms": 1698345601020,
      "t3_core_send_ms": 1698345601030,
      "t4_agent_recv_ms": 1698345601040
    }
  }
}
```

**4. ExecutionResult (Slave EA → Agent)**
```json
{
  "type": "execution_result",
  "timestamp_ms": 1698345601150,
  "payload": {
    "command_id": "01HKQV8YABCDEF123456789XYZ",
    "trade_id": "01HKQV8Y9GJ3F5R6WN8P2M4D1E",
    "client_id": "slave_67890",
    "success": true,
    "ticket": 111222,
    "error_code": "ERR_NO_ERROR",
    "error_message": "",
    "executed_price": 2045.52,
    "timestamps": {
      "t0_master_ea_ms": 1698345601000,
      "t1_agent_recv_ms": 1698345601010,
      "t2_core_recv_ms": 1698345601020,
      "t3_core_send_ms": 1698345601030,
      "t4_agent_recv_ms": 1698345601040,
      "t5_slave_ea_recv_ms": 1698345601050,
      "t6_order_send_ms": 1698345601100,
      "t7_order_filled_ms": 1698345601140
    }
  }
}
```

**5. TradeClose (Master EA → Agent)**
```json
{
  "type": "trade_close",
  "timestamp_ms": 1698345700000,
  "payload": {
    "close_id": "01HKQV9Z1A2B3C4D5E6F7G8H9I",
    "client_id": "master_12345",
    "account_id": "12345",
    "ticket": 987654,
    "magic_number": 123456,
    "close_price": 2048.75,
    "symbol": "XAUUSD"
  }
}
```

**6. CloseOrder (Agent → Slave EA)**
```json
{
  "type": "close_order",
  "timestamp_ms": 1698345700050,
  "payload": {
    "command_id": "01HKQV9Z1CLOSECMD123456",
    "close_id": "01HKQV9Z1A2B3C4D5E6F7G8H9I",
    "client_id": "slave_67890",
    "account_id": "67890",
    "ticket": 111222,
    "magic_number": 123456,
    "symbol": "XAUUSD"
  }
}
```

**7. CloseResult (Slave EA → Agent)**
```json
{
  "type": "close_result",
  "timestamp_ms": 1698345700150,
  "payload": {
    "command_id": "01HKQV9Z1CLOSECMD123456",
    "close_id": "01HKQV9Z1A2B3C4D5E6F7G8H9I",
    "client_id": "slave_67890",
    "success": true,
    "ticket": 111222,
    "error_code": "ERR_NO_ERROR",
    "error_message": "",
    "close_price": 2048.74
  }
}
```

### 5.3 Manejo de Errores en Named Pipes

- **Timeout lectura**: 5 segundos
- **Buffer**: 8192 bytes por mensaje
- **Reconexión**: EA intenta reconectar cada 5s si pipe se cierra
- **Logs**: Todos los errores de IPC van a logs del Agent

---

## 6. Arquitectura del Agent

**Principio**: Agent es un **thin layer** que solo hace routing. Toda la lógica está en SDK.

### 6.1 Componentes

```
Agent (1 binario, Windows native)
├── gRPC Client (sdk/grpc.StreamClient)
│   ├── Stream Bidi persistente
│   ├── Goroutine de lectura (recibe CoreMessage)
│   └── Goroutine de escritura (envía AgentMessage via canal)
├── Named Pipe Server (sdk/ipc)
│   ├── Pipe por EA: echo_master_<id>, echo_slave_<id>
│   ├── sdk/ipc.JSONReader por pipe
│   ├── sdk/ipc.JSONWriter por pipe
│   └── Registry de clientes conectados
├── Router (lógica de routing, NO transformaciones)
│   ├── Pipe → Stream: usa sdk/domain.JSONToProto()
│   └── Stream → Pipe: usa sdk/domain.ProtoToJSON()
└── Telemetry (sdk/telemetry.EchoMetrics)
    ├── EchoMetrics: funnel completo
    └── Logs estructurados
```

**Dependencias del Agent**:
- `github.com/xKoRx/echo/sdk/grpc` → cliente gRPC
- `github.com/xKoRx/echo/sdk/ipc` → Named Pipes
- `github.com/xKoRx/echo/sdk/domain` → transformers Proto ↔ JSON
- `github.com/xKoRx/echo/sdk/telemetry` → observabilidad
- `github.com/xKoRx/echo/sdk/proto/v0` → tipos proto
- `github.com/xKoRx/echo/sdk/utils` → timestamps, UUIDs

### 6.2 Flujo de Datos

**Master EA → Core**:
1. Master EA escribe JSON en pipe `echo_master_12345`
2. Agent lee de pipe, parsea JSON
3. Agent agrega timestamp `t1_agent_recv_ms`
4. Agent transforma JSON → proto `AgentMessage{TradeIntent}`
5. Agent envía por stream gRPC al Core

**Core → Slave EA**:
1. Core envía proto `CoreMessage{ExecuteOrder}` por stream
2. Agent recibe de stream, agrega timestamp `t4_agent_recv_ms`
3. Agent transforma proto → JSON
4. Agent escribe JSON en pipe `echo_slave_67890`
5. Slave EA lee de pipe y ejecuta

### 6.3 Pseudocódigo (usando SDK)

```go
import (
    "github.com/xKoRx/echo/sdk/grpc"
    "github.com/xKoRx/echo/sdk/ipc"
    "github.com/xKoRx/echo/sdk/domain"
    "github.com/xKoRx/echo/sdk/telemetry"
    pb "github.com/xKoRx/echo/sdk/proto/v0"
    "github.com/xKoRx/echo/sdk/utils"
)

type Agent struct {
    // gRPC usando SDK
    coreClient *grpc.StreamClient      // SDK abstraction
    sendCh     chan *pb.AgentMessage   // Canal para serializar writes
    
    // Named Pipes usando SDK
    pipes      map[string]*ipc.Pipe    // SDK abstraction
    pipesMu    sync.RWMutex
    
    // Telemetría usando SDK
    telemetry  *telemetry.Client
    metrics    *telemetry.EchoMetrics
}

func (a *Agent) Start(ctx context.Context) error {
    // 1. Conectar a Core via gRPC
    if err := a.connectToCore(ctx); err != nil {
        return err
    }
    
    // 2. Arrancar goroutines de stream
    go a.receiveFromCore(ctx)
    go a.sendToCore(ctx)
    
    // 3. Crear Named Pipes
    // TODO: configuración de IDs de cuentas (hardcoded en i0)
    accounts := []string{"12345", "67890"} // Master, Slave ejemplo
    for _, accID := range accounts {
        go a.servePipe(ctx, fmt.Sprintf("echo_master_%s", accID))
        go a.servePipe(ctx, fmt.Sprintf("echo_slave_%s", accID))
    }
    
    return nil
}

func (a *Agent) sendToCore(ctx context.Context) {
    for {
        select {
        case msg := <-a.sendCh:
            if err := a.coreStream.Send(msg); err != nil {
                // Log error, intentar reconectar
                a.telemetry.Error(ctx, "Failed to send to core", err)
            }
        case <-ctx.Done():
            return
        }
    }
}

func (a *Agent) receiveFromCore(ctx context.Context) {
    for {
        msg, err := a.coreStream.Recv()
        if err != nil {
            // Log error, reconectar
            a.telemetry.Error(ctx, "Failed to receive from core", err)
            // TODO: Implement reconnection logic in i1
            return
        }
        
        // Agregar timestamp t4
        a.addTimestamp(msg, "t4_agent_recv_ms")
        
        // Rutear según tipo
        switch payload := msg.Payload.(type) {
        case *pb.CoreMessage_ExecuteOrder:
            a.routeExecuteOrder(ctx, payload.ExecuteOrder)
        case *pb.CoreMessage_CloseOrder:
            a.routeCloseOrder(ctx, payload.CloseOrder)
        }
    }
}

func (a *Agent) servePipe(ctx context.Context, pipeName string) {
    // Crear Named Pipe usando SDK
    pipe, err := ipc.NewWindowsPipe(pipeName) // SDK
    if err != nil {
        a.telemetry.Error(ctx, "Failed to create pipe", err)
        return
    }
    defer pipe.Close()
    
    // Esperar conexión de EA
    if err := pipe.WaitForConnection(ctx); err != nil {
        a.telemetry.Error(ctx, "Pipe connection failed", err)
        return
    }
    
    // Crear JSONReader usando SDK
    reader := ipc.NewJSONReader(pipe) // SDK
    
    // Leer handshake
    handshake, err := a.readHandshake(reader) // Usa SDK reader
    if err != nil {
        a.telemetry.Error(ctx, "Handshake failed", err)
        return
    }
    
    // Registrar cliente
    a.registerClient(handshake.ClientID, pipe)
    defer a.unregisterClient(handshake.ClientID)
    
    // Loop de lectura usando SDK
    for {
        msg, err := reader.ReadMessage() // SDK: retorna map[string]interface{}
        if err != nil {
            a.telemetry.Error(ctx, "Failed to read from pipe", err)
            return
        }
        a.handlePipeMessage(ctx, handshake.ClientID, msg)
    }
}

func (a *Agent) handlePipeMessage(ctx context.Context, clientID string, msgMap map[string]interface{}) {
    // Extraer tipo de mensaje
    msgType, _ := msgMap["type"].(string)
    
    // Agregar timestamp t1
    t1 := utils.NowUnixMilli() // SDK
    
    // Rutear según tipo usando transformers de SDK
    switch msgType {
    case "trade_intent":
        // Transformar JSON → Proto usando SDK
        protoIntent, err := domain.JSONToTradeIntent(msgMap) // SDK
        if err != nil {
            a.telemetry.Error(ctx, "Failed to parse trade_intent", err)
            return
        }
        
        // Agregar timestamp usando SDK helper
        utils.AddTimestamp(protoIntent.Timestamps, "t1_agent_recv_ms", t1) // SDK
        
        // Registrar métrica
        a.metrics.RecordIntentReceived(ctx, 
            telemetry.EchoTradeID.String(protoIntent.TradeId),
            telemetry.EchoClientID.String(clientID),
            telemetry.EchoSymbol.String(protoIntent.Symbol),
        )
        
        // Enviar a Core
        a.sendCh <- &pb.AgentMessage{
            Payload: &pb.AgentMessage_TradeIntent{TradeIntent: protoIntent},
        }
        
        // Registrar métrica de forwarding
        a.metrics.RecordIntentForwarded(ctx, 
            telemetry.EchoTradeID.String(protoIntent.TradeId),
        )
        
    case "execution_result":
        protoResult, err := domain.JSONToExecutionResult(msgMap) // SDK
        if err != nil {
            a.telemetry.Error(ctx, "Failed to parse execution_result", err)
            return
        }
        a.sendCh <- &pb.AgentMessage{
            Payload: &pb.AgentMessage_ExecutionResult{ExecutionResult: protoResult},
        }
        
    case "trade_close":
        protoClose, err := domain.JSONToTradeClose(msgMap) // SDK
        if err != nil {
            a.telemetry.Error(ctx, "Failed to parse trade_close", err)
            return
        }
        a.sendCh <- &pb.AgentMessage{
            Payload: &pb.AgentMessage_TradeClose{TradeClose: protoClose},
        }
    }
}

func (a *Agent) routeExecuteOrder(ctx context.Context, order *pb.ExecuteOrder) {
    // Buscar pipe del slave
    a.pipesMu.RLock()
    pipe, exists := a.pipes[order.ClientId]
    a.pipesMu.RUnlock()
    
    if !exists {
        a.telemetry.Error(ctx, "Slave pipe not found", nil)
        return
    }
    
    // Agregar timestamp t5 usando SDK
    t5 := utils.NowUnixMilli() // SDK
    utils.AddTimestamp(order.Timestamps, "t5_slave_ea_recv_ms", t5) // SDK
    
    // Transformar proto → JSON usando SDK
    jsonMsg, err := domain.ExecuteOrderToJSON(order) // SDK
    if err != nil {
        a.telemetry.Error(ctx, "Failed to transform ExecuteOrder", err)
        return
    }
    
    // Crear JSONWriter usando SDK
    writer := ipc.NewJSONWriter(pipe) // SDK
    
    // Escribir mensaje (line-delimited)
    if err := writer.WriteMessage(jsonMsg); err != nil { // SDK
        a.telemetry.Error(ctx, "Failed to write to pipe", err)
        return
    }
    
    // Registrar métrica
    a.metrics.RecordExecutionDispatched(ctx,
        telemetry.EchoCommandID.String(order.CommandId),
        telemetry.EchoClientID.String(order.ClientId),
    )
}
```

---

## 7. Arquitectura del Core

**Principio**: Core es un **orchestrator** que solo hace routing y validación. Toda la lógica está en SDK.

### 7.1 Componentes

```
Core (1 binario Go)
├── gRPC Server (sdk/grpc.StreamServer)
│   ├── Stream Bidi por Agent
│   └── Goroutines de lectura/escritura por stream
├── Router/Orchestrator (lógica de routing, NO transformaciones)
│   ├── Recibe AgentMessage
│   ├── Valida con sdk/domain.Validate() (símbolo == XAUUSD)
│   ├── Dedupe (map in-memory)
│   ├── Procesa SECUENCIALMENTE (TODO: concurrencia en i1)
│   └── Genera ExecuteOrder/CloseOrder con sdk/domain.Transform()
├── Dedupe Store (in-memory)
│   └── map[trade_id]*DedupeEntry con TTL
└── Telemetry (sdk/telemetry.EchoMetrics)
    ├── EchoMetrics: funnel completo
    └── Logs estructurados
```

**Dependencias del Core**:
- `github.com/xKoRx/echo/sdk/grpc` → servidor gRPC
- `github.com/xKoRx/echo/sdk/domain` → validaciones + transformers
- `github.com/xKoRx/echo/sdk/telemetry` → observabilidad
- `github.com/xKoRx/echo/sdk/proto/v0` → tipos proto
- `github.com/xKoRx/echo/sdk/utils` → timestamps, UUIDs

### 7.2 Flujo de Procesamiento

**TradeIntent → ExecuteOrder**:
1. Core recibe `AgentMessage{TradeIntent}` del stream
2. Agregar timestamp `t2_core_recv_ms`
3. **Validar símbolo** (debe ser XAUUSD en i0)
4. **Dedupe**: buscar `trade_id` en map
   - Si existe y status != PENDING: rechazar duplicado
   - Si no existe: agregar con status=PENDING
5. **Transformar**: `TradeIntent` → `ExecuteOrder`
   - Copiar campos base
   - `lot_size = 0.10` (hardcoded, TODO: MM en i1)
   - `command_id = UUIDv7`
6. **Lookup slaves**: buscar streams de Agents que tengan slaves para ese símbolo
   - En i0: enviar a TODOS los slaves conocidos (TODO: config en i1)
7. **Enviar** `CoreMessage{ExecuteOrder}` por cada stream de Agent
8. Agregar timestamp `t3_core_send_ms`
9. **Actualizar dedupe**: status=PENDING → SENT (o mantener PENDING hasta recibir ack)

### 7.3 Pseudocódigo (usando SDK)

```go
import (
    "github.com/xKoRx/echo/sdk/grpc"
    "github.com/xKoRx/echo/sdk/domain"
    "github.com/xKoRx/echo/sdk/telemetry"
    pb "github.com/xKoRx/echo/sdk/proto/v0"
    "github.com/xKoRx/echo/sdk/utils"
)

type Core struct {
    // gRPC usando SDK
    grpcServer *grpc.Server // SDK abstraction
    
    agents     map[string]*AgentConnection // Key: agent_id, Value: conexión
    agentsMu   sync.RWMutex
    
    dedupe     map[string]*DedupeEntry
    dedupeMu   sync.RWMutex
    
    // TODO: En i0 procesamiento secuencial, en i1 concurrente
    processCh  chan *pb.AgentMessage // Canal para serializar procesamiento
    
    // Telemetría usando SDK
    telemetry  *telemetry.Client
    metrics    *telemetry.EchoMetrics
}

type AgentConnection struct {
    AgentID string
    Stream  pb.AgentService_StreamBidiServer
    SendCh  chan *pb.CoreMessage
}

type DedupeEntry struct {
    TradeID   string
    Status    pb.OrderStatus
    Timestamp int64
}

func (c *Core) Start(ctx context.Context) error {
    // 1. Inicializar telemetría
    // 2. Arrancar gRPC server
    lis, _ := net.Listen("tcp", ":50051") // TODO: puerto configurable
    c.grpcServer = grpc.NewServer()
    pb.RegisterAgentServiceServer(c.grpcServer, c)
    
    go c.grpcServer.Serve(lis)
    
    // 3. Arrancar processor secuencial
    go c.processLoop(ctx)
    
    // 4. Arrancar cleanup de dedupe
    go c.dedupeCleanup(ctx)
    
    return nil
}

func (c *Core) StreamBidi(stream pb.AgentService_StreamBidiServer) error {
    ctx := stream.Context()
    agentID := fmt.Sprintf("agent_%d", time.Now().UnixNano()) // TODO: ID real
    
    conn := &AgentConnection{
        AgentID: agentID,
        Stream:  stream,
        SendCh:  make(chan *pb.CoreMessage, 100),
    }
    
    // Registrar agent
    c.registerAgent(agentID, conn)
    defer c.unregisterAgent(agentID)
    
    // Goroutine de escritura (envía CoreMessage)
    go func() {
        for msg := range conn.SendCh {
            if err := stream.Send(msg); err != nil {
                c.telemetry.Error(ctx, "Failed to send to agent", err)
                return
            }
        }
    }()
    
    // Goroutine de lectura (recibe AgentMessage)
    for {
        msg, err := stream.Recv()
        if err != nil {
            c.telemetry.Error(ctx, "Agent disconnected", err)
            return err
        }
        
        // Agregar timestamp t2
        c.addTimestamp(msg, "t2_core_recv_ms")
        
        // Enviar a procesador secuencial
        c.processCh <- msg
    }
}

// TODO: En i0 procesamiento secuencial, en i1 concurrente
func (c *Core) processLoop(ctx context.Context) {
    for {
        select {
        case msg := <-c.processCh:
            c.processMessage(ctx, msg)
        case <-ctx.Done():
            return
        }
    }
}

func (c *Core) processMessage(ctx context.Context, msg *pb.AgentMessage) {
    switch payload := msg.Payload.(type) {
    case *pb.AgentMessage_TradeIntent:
        c.handleTradeIntent(ctx, payload.TradeIntent)
    case *pb.AgentMessage_ExecutionResult:
        c.handleExecutionResult(ctx, payload.ExecutionResult)
    case *pb.AgentMessage_TradeClose:
        c.handleTradeClose(ctx, payload.TradeClose)
    case *pb.AgentMessage_CloseResult:
        c.handleCloseResult(ctx, payload.CloseResult)
    }
}

func (c *Core) handleTradeIntent(ctx context.Context, intent *pb.TradeIntent) {
    // Agregar timestamp t2
    t2 := utils.NowUnixMilli() // SDK
    utils.AddTimestamp(intent.Timestamps, "t2_core_recv_ms", t2) // SDK
    
    // 1. Validar usando SDK
    if err := domain.ValidateSymbol(intent.Symbol, []string{"XAUUSD"}); err != nil { // SDK
        c.telemetry.Warn(ctx, "Invalid symbol", map[string]interface{}{
            "symbol":   intent.Symbol,
            "trade_id": intent.TradeId,
            "error":    err.Error(),
        })
        // TODO: enviar rechazo al Agent en i1
        return
    }
    
    // 2. Dedupe
    c.dedupeMu.Lock()
    if entry, exists := c.dedupe[intent.TradeId]; exists {
        if entry.Status != pb.OrderStatus_PENDING {
            c.dedupeMu.Unlock()
            c.telemetry.Warn(ctx, "Duplicate trade", map[string]interface{}{
                "trade_id": intent.TradeId,
                "status":   entry.Status,
            })
            return
        }
        // Si está PENDING, permitir (podría ser reintento)
    } else {
        // Nuevo trade
        c.dedupe[intent.TradeId] = &DedupeEntry{
            TradeID:   intent.TradeId,
            Status:    pb.OrderStatus_PENDING,
            Timestamp: time.Now().Unix(),
        }
    }
    c.dedupeMu.Unlock()
    
    // 3. Transformar TradeIntent → ExecuteOrder usando SDK
    orders := c.createExecuteOrders(intent)
    
    // 4. Agregar timestamp t3
    t3 := utils.NowUnixMilli() // SDK
    
    // 5. Enviar a todos los agents (broadcast en i0, routing en i1)
    c.agentsMu.RLock()
    for _, agent := range c.agents {
        for _, order := range orders {
            utils.AddTimestamp(order.Timestamps, "t3_core_send_ms", t3) // SDK
            agent.SendCh <- &pb.CoreMessage{
                Payload: &pb.CoreMessage_ExecuteOrder{ExecuteOrder: order},
            }
        }
    }
    c.agentsMu.RUnlock()
    
    // 6. Telemetría
    c.metrics.RecordOrderCreated(ctx,
        telemetry.EchoTradeID.String(intent.TradeId),
        telemetry.EchoSymbol.String(intent.Symbol),
    )
    c.metrics.RecordOrderSent(ctx,
        telemetry.EchoTradeID.String(intent.TradeId),
    )
}

func (c *Core) createExecuteOrders(intent *pb.TradeIntent) []*pb.ExecuteOrder {
    // TODO: En i0 broadcast a todos los slaves
    // TODO: En i1 lookup de configuración account → slaves
    
    // Transformar usando SDK con lot size hardcoded
    order := domain.TradeIntentToExecuteOrder(intent, &domain.TransformOptions{ // SDK
        LotSize:     0.10, // TODO: Hardcoded en i0, MM en i1
        CommandID:   utils.GenerateUUIDv7(), // SDK
        ClientID:    "", // TODO: lookup del slave real en i1
        AccountID:   "", // TODO: configuración de slaves
    })
    
    return []*pb.ExecuteOrder{order}
}

func (c *Core) handleExecutionResult(ctx context.Context, result *pb.ExecutionResult) {
    // Actualizar dedupe
    c.dedupeMu.Lock()
    if entry, exists := c.dedupe[result.TradeId]; exists {
        if result.Success {
            entry.Status = pb.OrderStatus_FILLED
        } else {
            entry.Status = pb.OrderStatus_REJECTED
        }
    }
    c.dedupeMu.Unlock()
    
    // Telemetría E2E usando SDK
    t9 := utils.NowUnixMilli() // SDK
    latencyE2E := float64(t9 - result.Timestamps.T0MasterEaMs)
    
    // Registrar métricas
    c.metrics.RecordExecutionCompleted(ctx,
        telemetry.EchoTradeID.String(result.TradeId),
        telemetry.EchoCommandID.String(result.CommandId),
        telemetry.EchoStatus.String(statusToString(result.Success)),
        telemetry.EchoErrorCode.String(result.ErrorCode.String()),
    )
    
    c.metrics.RecordLatencyE2E(ctx, latencyE2E,
        telemetry.EchoTradeID.String(result.TradeId),
    )
    
    // Calcular latencias por hop usando SDK
    if ts := result.Timestamps; ts != nil {
        c.recordHopLatencies(ctx, ts)
    }
    
    // TODO: Persistir en Postgres en i1
}

func (c *Core) recordHopLatencies(ctx context.Context, ts *pb.TimestampMetadata) {
    // Agent → Core
    if ts.T2CoreRecvMs > 0 && ts.T1AgentRecvMs > 0 {
        c.metrics.RecordLatencyAgentToCore(ctx, float64(ts.T2CoreRecvMs - ts.T1AgentRecvMs))
    }
    // Core processing
    if ts.T3CoreSendMs > 0 && ts.T2CoreRecvMs > 0 {
        c.metrics.RecordLatencyCoreProcess(ctx, float64(ts.T3CoreSendMs - ts.T2CoreRecvMs))
    }
    // Core → Agent
    if ts.T4AgentRecvMs > 0 && ts.T3CoreSendMs > 0 {
        c.metrics.RecordLatencyCoreToAgent(ctx, float64(ts.T4AgentRecvMs - ts.T3CoreSendMs))
    }
    // Slave execution
    if ts.T7OrderFilledMs > 0 && ts.T6OrderSendMs > 0 {
        c.metrics.RecordLatencySlaveExecution(ctx, float64(ts.T7OrderFilledMs - ts.T6OrderSendMs))
    }
}

func statusToString(success bool) string {
    if success {
        return "success"
    }
    return "rejected"
}

func (c *Core) dedupeCleanup(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            now := time.Now().Unix()
            c.dedupeMu.Lock()
            for tradeID, entry := range c.dedupe {
                // Limpiar entries > 1 hora
                if now-entry.Timestamp > 3600 && entry.Status != pb.OrderStatus_PENDING {
                    delete(c.dedupe, tradeID)
                }
            }
            c.dedupeMu.Unlock()
        case <-ctx.Done():
            return
        }
    }
}
```

---

## 8. SDK Telemetry — Ajustes para i0

### 8.1 Bundle EchoMetrics

Crear nuevo archivo `sdk/telemetry/bundle_echo.go`:

```go
package telemetry

import (
    "context"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/metric"
)

// EchoMetrics expone métricas específicas del copiador de operaciones
type EchoMetrics struct {
    IntentReceived       metric.Int64Counter    // Agent recibe TradeIntent
    IntentForwarded      metric.Int64Counter    // Agent envía al Core
    OrderCreated         metric.Int64Counter    // Core crea ExecuteOrder
    OrderSent            metric.Int64Counter    // Core envía al Agent
    ExecutionDispatched  metric.Int64Counter    // Agent envía a Slave EA
    ExecutionCompleted   metric.Int64Counter    // Resultado final (success/error)
    
    LatencyE2E           metric.Float64Histogram // Latencia extremo a extremo
    LatencyAgentToCore   metric.Float64Histogram // t2 - t1
    LatencyCoreProcess   metric.Float64Histogram // t3 - t2
    LatencyCoreToAgent   metric.Float64Histogram // t4 - t3
    LatencyAgentToSlave  metric.Float64Histogram // t5 - t4
    LatencySlaveExecution metric.Float64Histogram // t7 - t6
}

// NewEchoMetrics crea una instancia de EchoMetrics
func (c *Client) NewEchoMetrics() (*EchoMetrics, error) {
    meter := c.meterProvider.Meter("echo.metrics")
    
    intentReceived, err := meter.Int64Counter(
        "echo.intent.received",
        metric.WithDescription("TradeIntents recibidos por Agent desde Master EA"),
    )
    if err != nil {
        return nil, err
    }
    
    // ... similar para todas las métricas
    
    latencyE2E, err := meter.Float64Histogram(
        "echo.latency.e2e",
        metric.WithDescription("Latencia E2E desde Master EA hasta ejecución en Slave"),
        metric.WithUnit("ms"),
    )
    if err != nil {
        return nil, err
    }
    
    // ... histogramas adicionales
    
    return &EchoMetrics{
        IntentReceived:       intentReceived,
        // ...
        LatencyE2E:           latencyE2E,
        // ...
    }, nil
}

// RecordIntentReceived registra recepción de TradeIntent
func (m *EchoMetrics) RecordIntentReceived(ctx context.Context, attrs ...attribute.KeyValue) {
    m.IntentReceived.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordLatencyE2E registra latencia extremo a extremo
func (m *EchoMetrics) RecordLatencyE2E(ctx context.Context, latencyMs float64, attrs ...attribute.KeyValue) {
    m.LatencyE2E.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}

// ... métodos adicionales
```

### 8.2 Convenciones Semánticas (SemConv)

Agregar atributos específicos de Echo en `sdk/telemetry/semconv.go`:

```go
// Echo attributes
var (
    EchoTradeID      = attribute.Key("echo.trade_id")
    EchoCommandID    = attribute.Key("echo.command_id")
    EchoClientID     = attribute.Key("echo.client_id")
    EchoAccountID    = attribute.Key("echo.account_id")
    EchoSymbol       = attribute.Key("echo.symbol")
    EchoOrderSide    = attribute.Key("echo.order_side")
    EchoMagicNumber  = attribute.Key("echo.magic_number")
    EchoComponent    = attribute.Key("echo.component") // "agent", "core", "master_ea", "slave_ea"
    EchoStatus       = attribute.Key("echo.status")    // "success", "rejected", "timeout"
    EchoErrorCode    = attribute.Key("echo.error_code")
)
```

### 8.3 Inicialización en Agent/Core

```go
// En Agent
func main() {
    ctx := context.Background()
    
    // Inicializar telemetría
    telClient, err := telemetry.NewClient(ctx, telemetry.Config{
        ServiceName:    "echo-agent",
        ServiceVersion: "0.1.0",
        Environment:    "dev",
        OTLPEndpoint:   "localhost:4317", // TODO: config en i1
    })
    if err != nil {
        log.Fatalf("Failed to init telemetry: %v", err)
    }
    defer telClient.Shutdown(ctx)
    
    // Crear bundle EchoMetrics
    echoMetrics, err := telClient.NewEchoMetrics()
    if err != nil {
        log.Fatalf("Failed to init metrics: %v", err)
    }
    
    // Usar en el Agent
    agent := &Agent{
        telemetry:   telClient,
        echoMetrics: echoMetrics,
    }
    
    // ...
}
```

---

## 9. Prompts Detallados para Generación de EAs

### 9.1 Prompt para Master EA (MQL4)

```
# PROMPT PARA GENERACIÓN DE MASTER EA MQL4 — ECHO TRADE COPIER i0

## 1. CONTEXTO DEL PROYECTO ECHO

### 1.1 Qué es Echo Trade Copier
Echo es un sistema de replicación de operaciones en trading algorítmico. Replica en tiempo real las operaciones de **cuentas Master** (origen de señales) hacia **cuentas Slave** (seguidoras).

**Arquitectura del sistema**:
```
Master EA (MQL4) → Named Pipe → Agent (Go) → gRPC → Core (Go) → gRPC → Agent → Named Pipe → Slave EA (MQL4)
```

**Flujo de una operación**:
1. Master EA ejecuta orden en broker (manualmente o por estrategia)
2. Master EA genera `TradeIntent` con metadatos (trade_id único, precio, lote, magic number)
3. Master EA envía TradeIntent por Named Pipe al Agent local
4. Agent local reenvía al Core central via gRPC
5. Core valida, deduplica, y transforma en `ExecuteOrder`
6. Core envía ExecuteOrder a Agent(s) de las cuentas Slave
7. Agent entrega ExecuteOrder al Slave EA via Named Pipe
8. Slave EA ejecuta la orden en su broker
9. Slave EA reporta `ExecutionResult` de vuelta al Core
10. Core registra métricas de latencia extremo a extremo

**Objetivo de esta iteración (i0)**:
- Símbolo único: XAUUSD hardcoded
- Solo órdenes a mercado (BUY/SELL market orders)
- Sin SL/TP (Stop Loss / Take Profit)
- Lot size fijo: 0.10 (hardcoded en Core, no en Master EA)
- 2 masters → 2 slaves para testing

### 1.2 Rol del Master EA
El Master EA es el **origen de señales**. Su responsabilidad es:
1. **Detectar** cuando se ejecuta una orden (manual o algorítmica)
2. **Generar** un TradeIntent con metadatos completos
3. **Enviar** el TradeIntent al Agent local via Named Pipe
4. **Detectar** cierres de posiciones y reportarlos
5. **Registrar observabilidad**: logs estructurados para debugging y métricas

El Master EA **NO decide** qué slaves copian, **NO calcula** lot sizes de slaves, **NO maneja** errores de ejecución de slaves. Solo reporta intents.

---

## 2. CONTEXTO TÉCNICO DEL MASTER EA

### 2.1 Problema a Resolver
MetaTrader 4 no tiene capacidad nativa de comunicarse con procesos externos. Necesitamos:
1. **IPC via Named Pipes**: Usar DLL externa para conectar el EA con el Agent local
2. **Serialización JSON**: Construir mensajes JSON válidos desde MQL4 (no tiene JSON nativo)
3. **UUIDv7 generation**: Generar trade_id únicos ordenables por tiempo
4. **State tracking**: Mantener array de órdenes abiertas para detectar cierres
5. **Observabilidad**: Logs estructurados JSON que el Agent puede recopilar

---

## 3. REQUISITOS FUNCIONALES

### 3.1 Conexión a Named Pipe

**Named Pipe**:
- Nombre: `\\.\pipe\echo_master_<account_id>` donde `<account_id>` = `AccountNumber()`
  - Ejemplo: Si AccountNumber() = 12345, pipe = `\\.\pipe\echo_master_12345`
- Protocolo: JSON line-delimited (cada mensaje termina con `\n`)
- Codificación: UTF-8
- Timeout escritura: 5 segundos
- Reconexión: intentar cada 5 segundos si pipe se cierra o falla la escritura

**Importante**: El EA **NO crea** el pipe, **solo se conecta** a un pipe existente creado por el Agent.

### 3.2 Handshake Inicial

**Propósito**: Registrar el EA con el Agent para que sepa qué cuenta/role/símbolo maneja.

**Cuándo**: Al conectarse exitosamente al Named Pipe (en `OnInit()` o tras reconexión).

**Formato JSON**:
```json
{
  "type": "handshake",
  "timestamp_ms": 1698345600000,
  "payload": {
    "client_id": "master_12345",
    "account_id": "12345",
    "broker": "IC Markets",
    "role": "master",
    "symbol": "XAUUSD",
    "version": "0.1.0"
  }
}
```

**Campos**:
- `type`: Siempre `"handshake"`
- `timestamp_ms`: `GetTickCount()` (milisegundos desde arranque del sistema)
- `payload.client_id`: `"master_" + AccountNumber()` → ej: `"master_12345"`
- `payload.account_id`: `AccountNumber()` como string → ej: `"12345"`
- `payload.broker`: `AccountCompany()` → ej: `"IC Markets"`
- `payload.role`: Siempre `"master"` para este EA
- `payload.symbol`: Siempre `"XAUUSD"` en i0 (hardcoded)
- `payload.version`: Versión del EA, ej: `"0.1.0"`

**Respuesta esperada**: El Agent **NO responde** al handshake (unidireccional). Si el pipe está conectado y el write() no falla, asumir éxito.

**Validación**: Verificar que `WritePipe()` retorna > 0 (bytes escritos). Si retorna <= 0, loggear error.

---

### 3.3 Generación de UUIDv7

**Propósito**: Generar identificadores únicos **ordenables** por tiempo para cada trade intent. Esto permite dedupe en el Core y trazabilidad.

**Especificación UUIDv7 (simplificada para MQL4)**:
- Formato: `8-4-4-4-12` caracteres hexadecimales separados por guiones
- Ejemplo: `01HKQV8Y-9GJ3-F5R6-WN8P-2M4D1E123456`
- Primeros 48 bits: Unix timestamp en milisegundos
- Resto: random o sequence counter

**Implementación en MQL4** (función helper):
```mql4
string GenerateUUIDv7() {
    // Parte 1: timestamp Unix ms (48 bits = 12 hex chars)
    ulong timestamp_ms = (ulong)(TimeLocal() * 1000 + (GetTickCount() % 1000));
    
    // Parte 2-5: random (usando MathRand())
    string uuid = "";
    uuid += StringFormat("%012X", timestamp_ms); // 12 chars
    uuid += "-";
    uuid += StringFormat("%04X", MathRand() % 65536); // 4 chars
    uuid += "-";
    uuid += StringFormat("7%03X", MathRand() % 4096); // 4 chars (versión 7)
    uuid += "-";
    uuid += StringFormat("%04X", MathRand() % 65536); // 4 chars
    uuid += "-";
    uuid += StringFormat("%012X", MathRand() * MathRand()); // 12 chars
    
    return uuid;
}
```

**Nota**: Esta implementación es simplificada. Para producción, considera usar mejor generador random o secuencias monotónicas.

**Validación**: El trade_id debe ser **único** por cada operación. No reutilizar.

---

### 3.4 Detección y Envío de TradeIntent

**Propósito**: Cuando el EA ejecuta una orden (manual o automática), debe generar un TradeIntent y enviarlo al Agent.

**Trigger**: Después de un `OrderSend()` exitoso (ticket > 0).

**Botón Manual para BUY (Testing)**:
Para facilitar testing en i0, crear un botón gráfico en el chart que ejecute una orden BUY al hacer clic.
- Crear botón gráfico en el chart: "BUY XAUUSD"
- Al hacer clic: ejecutar OrderSend market buy (0.01 lote)
- Tras ejecución exitosa: generar TradeIntent

**Código ejemplo** (simplificado):
```mql4
void OnChartEvent(const int id, const long& lparam, const double& dparam, const string& sparam) {
    if (id == CHARTEVENT_OBJECT_CLICK && sparam == "BtnBuy") {
        ExecuteBuyOrder();
    }
}

void ExecuteBuyOrder() {
    int ticket = OrderSend(Symbol(), OP_BUY, 0.01, Ask, 3, 0, 0, "Echo Master", g_MagicNumber, 0, clrGreen);
    if (ticket > 0) {
        Log("INFO", "Order executed", "ticket=" + IntegerToString(ticket) + ",price=" + DoubleToString(Ask, Digits));
        SendTradeIntent(ticket);
    } else {
        Log("ERROR", "OrderSend failed", "error=" + IntegerToString(GetLastError()));
    }
}
```

---

### 3.5 Generación y Envío de TradeIntent

**Propósito**: Notificar al Core que se ejecutó una orden en el Master para que se replique en los Slaves.

**Cuándo**: Inmediatamente después de un `OrderSend()` exitoso (ticket > 0).

**Formato JSON completo**:
```json
{
  "type": "trade_intent",
  "timestamp_ms": 1698345601000,
  "payload": {
    "trade_id": "01HKQV8Y-9GJ3-F5R6-WN8P-2M4D1E123456",
    "client_id": "master_12345",
    "account_id": "12345",
    "symbol": "XAUUSD",
    "order_side": "BUY",
    "lot_size": 0.01,
    "price": 2045.50,
    "magic_number": 123456,
    "ticket": 987654,
    "timestamps": {
      "t0_master_ea_ms": 1698345601000
    }
  }
}
```

**Campos explicados**:
- `type`: Siempre `"trade_intent"`
- `timestamp_ms`: `GetTickCount()` al momento de generar el mensaje
- `payload.trade_id`: **UUIDv7 único** generado con `GenerateUUIDv7()`
- `payload.client_id`: `"master_" + AccountNumber()`
- `payload.account_id`: `AccountNumber()` como string
- `payload.symbol`: `Symbol()` → siempre `"XAUUSD"` en i0
- `payload.order_side`: 
  - `"BUY"` si `OrderType() == OP_BUY`
  - `"SELL"` si `OrderType() == OP_SELL`
- `payload.lot_size`: `OrderLots()` de la orden ejecutada
- `payload.price`: `OrderOpenPrice()` de la orden ejecutada
- `payload.magic_number`: MagicNumber del EA (input parameter)
- `payload.ticket`: `OrderTicket()` del master (MT4 ticket number)
- `payload.timestamps.t0_master_ea_ms`: **Timestamp de generación** del intent (`GetTickCount()`)

**Validaciones antes de enviar**:
1. `Symbol() == "XAUUSD"` → si no, no enviar (loggear warning)
2. `ticket > 0` → validar que OrderSelect(ticket) retorna true
3. `OrderType()` debe ser `OP_BUY` o `OP_SELL` (no OP_BUYSTOP, etc.)
4. `trade_id` no vacío

**Código ejemplo**:
```mql4
void SendTradeIntent(int ticket) {
    if (!OrderSelect(ticket, SELECT_BY_TICKET)) {
        Log("ERROR", "OrderSelect failed", "ticket=" + IntegerToString(ticket));
        return;
    }
    
    if (OrderSymbol() != "XAUUSD") {
        Log("WARN", "Symbol not supported", "symbol=" + OrderSymbol());
        return;
    }
    
    string orderSide = "";
    if (OrderType() == OP_BUY) orderSide = "BUY";
    else if (OrderType() == OP_SELL) orderSide = "SELL";
    else {
        Log("WARN", "Order type not supported", "type=" + IntegerToString(OrderType()));
        return;
    }
    
    string tradeId = GenerateUUIDv7();
    ulong t0 = GetTickCount();
    
    string json = BuildTradeIntentJSON(tradeId, orderSide, OrderLots(), OrderOpenPrice(), OrderTicket(), t0);
    
    if (WritePipe(g_PipeHandle, json) > 0) {
        Log("INFO", "TradeIntent sent", "trade_id=" + tradeId + ",ticket=" + IntegerToString(ticket));
    } else {
        Log("ERROR", "WritePipe failed", "trade_id=" + tradeId);
        // TODO: reintentar o queue local para reenviar
    }
}

string BuildTradeIntentJSON(string tradeId, string orderSide, double lots, double price, int ticket, ulong t0) {
    string json = "{";
    json += "\"type\":\"trade_intent\",";
    json += "\"timestamp_ms\":" + IntegerToString(t0) + ",";
    json += "\"payload\":{";
    json += "\"trade_id\":\"" + tradeId + "\",";
    json += "\"client_id\":\"master_" + IntegerToString(AccountNumber()) + "\",";
    json += "\"account_id\":\"" + IntegerToString(AccountNumber()) + "\",";
    json += "\"symbol\":\"XAUUSD\",";
    json += "\"order_side\":\"" + orderSide + "\",";
    json += "\"lot_size\":" + DoubleToString(lots, 2) + ",";
    json += "\"price\":" + DoubleToString(price, Digits) + ",";
    json += "\"magic_number\":" + IntegerToString(g_MagicNumber) + ",";
    json += "\"ticket\":" + IntegerToString(ticket) + ",";
    json += "\"timestamps\":{";
    json += "\"t0_master_ea_ms\":" + IntegerToString(t0);
    json += "}}}";
    return json + "\n";  // IMPORTANTE: agregar \n al final
}
```

**Observabilidad**: Loggear **siempre** el `trade_id` cuando se envía el intent para poder tracear en logs del Agent/Core.

---

### 3.6 Detección y Reporte de Cierres

**Propósito**: Cuando una posición se cierra en el Master, notificar al Core para que cierre las posiciones correspondientes en los Slaves.

**Estrategia de detección**:
1. Mantener un **array global** de tickets abiertos: `int g_OpenTickets[]`
2. En `OnTick()`: iterar sobre `g_OpenTickets`
3. Para cada ticket: verificar si `OrderCloseTime() > 0` (indica cierre)
4. Si cerró: generar `TradeClose`, enviarlo, y remover ticket del array

**Formato JSON**:
```json
{
  "type": "trade_close",
  "timestamp_ms": 1698345700000,
  "payload": {
    "close_id": "01HKQV9Z-1A2B-3C4D-5E6F-7G8H9I123456",
    "client_id": "master_12345",
    "account_id": "12345",
    "ticket": 987654,
    "magic_number": 123456,
    "close_price": 2048.75,
    "symbol": "XAUUSD"
  }
}
```

**Campos**:
- `type`: Siempre `"trade_close"`
- `timestamp_ms`: `GetTickCount()` al detectar el cierre
- `payload.close_id`: **UUIDv7 único** para este evento de cierre
- `payload.client_id`: `"master_" + AccountNumber()`
- `payload.account_id`: `AccountNumber()` como string
- `payload.ticket`: Ticket del master que cerró
- `payload.magic_number`: MagicNumber del EA
- `payload.close_price`: `OrderClosePrice()` de la orden cerrada
- `payload.symbol`: `OrderSymbol()` → `"XAUUSD"`

**Código ejemplo**:
```mql4
int g_OpenTickets[]; // Array dinámico de tickets abiertos
int g_OpenTicketsCount = 0;

void OnTick() {
    CheckForClosedOrders();
    // ... resto de lógica OnTick
}

void CheckForClosedOrders() {
    for (int i = g_OpenTicketsCount - 1; i >= 0; i--) {  // Iterar en reversa para poder eliminar
        int ticket = g_OpenTickets[i];
        
        if (!OrderSelect(ticket, SELECT_BY_TICKET)) {
            // Ticket no existe, remover del array
            RemoveTicketFromArray(i);
            continue;
        }
        
        if (OrderCloseTime() > 0) {
            // Orden cerrada
            Log("INFO", "Order closed", "ticket=" + IntegerToString(ticket) + ",close_price=" + DoubleToString(OrderClosePrice(), Digits));
            SendTradeClose(ticket, OrderClosePrice(), OrderSymbol());
            RemoveTicketFromArray(i);
        }
    }
}

void SendTradeClose(int ticket, double closePrice, string symbol) {
    string closeId = GenerateUUIDv7();
    ulong ts = GetTickCount();
    
    string json = "{";
    json += "\"type\":\"trade_close\",";
    json += "\"timestamp_ms\":" + IntegerToString(ts) + ",";
    json += "\"payload\":{";
    json += "\"close_id\":\"" + closeId + "\",";
    json += "\"client_id\":\"master_" + IntegerToString(AccountNumber()) + "\",";
    json += "\"account_id\":\"" + IntegerToString(AccountNumber()) + "\",";
    json += "\"ticket\":" + IntegerToString(ticket) + ",";
    json += "\"magic_number\":" + IntegerToString(g_MagicNumber) + ",";
    json += "\"close_price\":" + DoubleToString(closePrice, Digits) + ",";
    json += "\"symbol\":\"" + symbol + "\"";
    json += "}}";
    json += "\n";
    
    if (WritePipe(g_PipeHandle, json) > 0) {
        Log("INFO", "TradeClose sent", "close_id=" + closeId + ",ticket=" + IntegerToString(ticket));
    } else {
        Log("ERROR", "WritePipe failed for TradeClose", "ticket=" + IntegerToString(ticket));
    }
}

void AddTicketToArray(int ticket) {
    ArrayResize(g_OpenTickets, g_OpenTicketsCount + 1);
    g_OpenTickets[g_OpenTicketsCount] = ticket;
    g_OpenTicketsCount++;
    Log("DEBUG", "Ticket added to tracking", "ticket=" + IntegerToString(ticket) + ",count=" + IntegerToString(g_OpenTicketsCount));
}

void RemoveTicketFromArray(int index) {
    if (index < 0 || index >= g_OpenTicketsCount) return;
    
    // Shift elementos
    for (int i = index; i < g_OpenTicketsCount - 1; i++) {
        g_OpenTickets[i] = g_OpenTickets[i + 1];
    }
    g_OpenTicketsCount--;
    ArrayResize(g_OpenTickets, g_OpenTicketsCount);
    Log("DEBUG", "Ticket removed from tracking", "index=" + IntegerToString(index) + ",count=" + IntegerToString(g_OpenTicketsCount));
}
```

**Importante**: Llamar `AddTicketToArray(ticket)` inmediatamente después de cada `OrderSend()` exitoso y **antes** de enviar el TradeIntent.

---

## 4. OBSERVABILIDAD (CRÍTICO)

### 4.1 Sistema de Logging Estructurado

**Propósito**: Generar logs en formato **casi-JSON** que el Agent puede recopilar y parsear para métricas y debugging.

**Formato de log**:
```
[LEVEL] timestamp_ms | event | key1=value1,key2=value2,...
```

**Niveles**:
- `DEBUG`: Información de debugging (activar/desactivar con input param)
- `INFO`: Eventos normales (conexiones, órdenes, mensajes enviados)
- `WARN`: Advertencias (símbolo no soportado, validación fallida)
- `ERROR`: Errores (pipe cerrado, OrderSend falló, WritePipe falló)

**Función Log**:
```mql4
void Log(string level, string event, string details) {
    ulong ts = GetTickCount();
    string logLine = "[" + level + "] " + IntegerToString(ts) + " | " + event + " | " + details;
    Print(logLine);  // Va al Expert tab de MT4
    
    // Opcional: escribir a archivo local para persistencia
    // int handle = FileOpen("echo_master_" + IntegerToString(AccountNumber()) + ".log", FILE_WRITE|FILE_READ|FILE_TXT, '\t');
    // FileSeek(handle, 0, SEEK_END);
    // FileWrite(handle, logLine);
    // FileClose(handle);
}
```

**Eventos clave a loggear**:
1. **Conexión** al pipe: `"Pipe connected"` (INFO)
2. **Handshake** enviado: `"Handshake sent"` (INFO)
3. **Orden ejecutada**: `"Order executed"` con ticket y precio (INFO)
4. **TradeIntent enviado**: `"TradeIntent sent"` con trade_id (INFO)
5. **Orden cerrada**: `"Order closed"` con ticket y precio de cierre (INFO)
6. **TradeClose enviado**: `"TradeClose sent"` con close_id (INFO)
7. **Error de pipe**: `"WritePipe failed"` con código de error (ERROR)
8. **Error de OrderSend**: `"OrderSend failed"` con código de error MT4 (ERROR)
9. **Reconexión**: `"Attempting pipe reconnection"` (WARN)
10. **Symbol no soportado**: `"Symbol not supported"` (WARN)

**Ejemplo de uso**:
```mql4
Log("INFO", "Pipe connected", "pipe=\\\\.\\pipe\\echo_master_12345");
Log("INFO", "Order executed", "ticket=987654,price=2045.50,side=BUY,lots=0.01");
Log("INFO", "TradeIntent sent", "trade_id=01HKQV8Y...,ticket=987654");
Log("ERROR", "OrderSend failed", "error=134,desc=Not enough money");
```

### 4.2 Métricas Implícitas

El Agent puede parsear los logs del EA para extraer métricas:
- **Tasa de órdenes**: count de `"Order executed"`
- **Tasa de errores**: count de `ERROR` level
- **Latencia de envío**: diff entre `"Order executed"` y `"TradeIntent sent"` (mismo ticket)
- **Tasa de reconexiones**: count de `"Attempting pipe reconnection"`

### 4.3 Configuración de Observabilidad

Input parameters del EA:
```mql4
input bool   EnableDebugLogs = false;     // Activar logs DEBUG
input bool   LogToFile = false;           // Escribir logs a archivo local
```

---

## 5. REQUISITOS NO FUNCIONALES

### 5.1 Named Pipe en MQL4 (DLL Externa)

MQL4 **NO tiene soporte nativo** de Named Pipes. Se requiere DLL externa en C++.

**Opción A: DLL Mínima** (recomendada para i0):
Crear `echo_pipe.dll` con 3 funciones:

```cpp
// echo_pipe.dll (C++ simplificado)
#include <windows.h>

extern "C" __declspec(dllexport) int ConnectPipe(wchar_t* pipeName) {
    HANDLE hPipe = CreateFile(
        pipeName,
        GENERIC_WRITE,
        0,
        NULL,
        OPEN_EXISTING,
        0,
        NULL
    );
    return (hPipe == INVALID_HANDLE_VALUE) ? -1 : (int)hPipe;
}

extern "C" __declspec(dllexport) int WritePipe(int handle, char* data) {
    DWORD written;
    BOOL result = WriteFile((HANDLE)handle, data, strlen(data), &written, NULL);
    return result ? written : -1;
}

extern "C" __declspec(dllexport) void ClosePipe(int handle) {
    CloseHandle((HANDLE)handle);
}
```

**Import en MQL4**:
```mql4
#import "echo_pipe.dll"
   int ConnectPipe(string pipeName);
   int WritePipe(int handle, string data);
   void ClosePipe(int handle);
#import
```

**Opción B**: Usar librería existente si disponible (ej: winsock wrappers).

---

### 5.2 JSON en MQL4

MQL4 **NO tiene JSON nativo**. Serialización manual obligatoria.

**Importante**:
- Escapar caracteres especiales en strings (`\"`, `\\`, `\n`)
- No usar floats con notación científica (DoubleToString con precisión fija)
- Agregar `\n` al final de cada mensaje

**Helpers recomendados**:
```mql4
string EscapeJSON(string input) {
    StringReplace(input, "\\", "\\\\");
    StringReplace(input, "\"", "\\\"");
    StringReplace(input, "\n", "\\n");
    return input;
}
```

---

### 5.3 Input Parameters

```mql4
input int    MagicNumber = 123456;                    // MagicNumber único de la estrategia
input string PipeBaseName = "\\\\.\\\pipe\\\\echo_master_";  // Base del pipe name (se concatena con AccountNumber)
input string Symbol = "XAUUSD";                        // Símbolo hardcoded para i0
input bool   EnableDebugLogs = false;                  // Activar logs DEBUG
input bool   LogToFile = false;                        // Escribir logs a archivo
```

**Nota**: PipeBaseName tiene escapes dobles (`\\\\`) porque MQL4 requiere escaparlos.

---

### 5.4 Error Handling

**Errores de pipe**:
- Si `ConnectPipe()` retorna -1: loggear error, esperar 5s, reintentar
- Si `WritePipe()` retorna -1: loggear error, intentar reconectar pipe
- Máximo 3 reintentos antes de entrar en "modo degradado" (solo loggear, no enviar)

**Errores de OrderSend**:
- Loggear código de error y descripción (GetLastError())
- NO enviar TradeIntent si OrderSend falla
- Ejemplos de errores comunes:
  - `ERR_NOT_ENOUGH_MONEY (134)`: balance insuficiente
  - `ERR_INVALID_STOPS (130)`: SL/TP inválidos (no aplica en i0)
  - `ERR_OFF_QUOTES (136)`: broker no cotiza
  - `ERR_REQUOTE (138)`: precio cambió, requote

**Código ejemplo**:
```mql4
int g_PipeHandle = -1;
int g_ReconnectAttempts = 0;

void OnInit() {
    ConnectToPipe();
}

void ConnectToPipe() {
    string pipeName = PipeBaseName + IntegerToString(AccountNumber());
    g_PipeHandle = ConnectPipe(pipeName);
    
    if (g_PipeHandle > 0) {
        Log("INFO", "Pipe connected", "pipe=" + pipeName);
        SendHandshake();
        g_ReconnectAttempts = 0;
    } else {
        Log("ERROR", "Pipe connection failed", "pipe=" + pipeName + ",error=" + IntegerToString(GetLastError()));
        g_ReconnectAttempts++;
        if (g_ReconnectAttempts < 3) {
            Sleep(5000);
            ConnectToPipe(); // Recursivo con límite
        }
    }
}
```

---

### 5.6 Estado Interno y Persistencia

**Variables globales necesarias**:
```mql4
int g_PipeHandle = -1;                 // Handle del pipe
int g_MagicNumber = 123456;             // MagicNumber (de input param)
int g_OpenTickets[];                   // Array de tickets abiertos
int g_OpenTicketsCount = 0;            // Count de tickets
```

**NO hay persistencia** en i0. Si el EA se reinicia, pierde el estado de tickets abiertos.

**Workaround**: Al arrancar en `OnInit()`, escanear todas las órdenes abiertas con MagicNumber == g_MagicNumber y agregar sus tickets al array.

```mql4
void OnInit() {
    // Reconstruir array de tickets al iniciar
    for (int i = OrdersTotal() - 1; i >= 0; i--) {
        if (OrderSelect(i, SELECT_BY_POS) && OrderMagicNumber() == g_MagicNumber && OrderCloseTime() == 0) {
            AddTicketToArray(OrderTicket());
        }
    }
    Log("INFO", "EA initialized", "tracked_tickets=" + IntegerToString(g_OpenTicketsCount));
    
    ConnectToPipe();
}
```

---

## 6. ESTRUCTURA DE ARCHIVOS

```
MQL4/
├── Experts/
│   └── EchoMasterEA.mq4              // Código principal del EA
├── Libraries/
│   └── echo_pipe.dll                 // DLL para Named Pipes (Windows x64)
└── Files/
    └── echo_master_<account>.log     // Logs locales (opcional, si LogToFile=true)
```

---

## 7. FLUJO COMPLETO DE EJECUCIÓN (EJEMPLO)

**Paso 1: Usuario carga EA en chart XAUUSD**
- `OnInit()` ejecuta
- EA conecta a pipe `\\.\pipe\echo_master_12345`
- EA envía handshake
- EA reconstruye array de tickets abiertos
- Log: `[INFO] ... | EA initialized | tracked_tickets=0`

**Paso 2: Usuario hace clic en botón "BUY"**
- `OnChartEvent()` detecta click
- `ExecuteBuyOrder()` llama `OrderSend(...)`
- OrderSend retorna ticket 987654
- Log: `[INFO] ... | Order executed | ticket=987654,price=2045.50,side=BUY`
- EA agrega ticket al array: `AddTicketToArray(987654)`
- EA genera UUIDv7: `"01HKQV8Y-..."`
- EA construye TradeIntent JSON
- EA llama `WritePipe(g_PipeHandle, json)`
- Log: `[INFO] ... | TradeIntent sent | trade_id=01HKQV8Y...,ticket=987654`

**Paso 3: Cada tick, EA monitorea**
- `OnTick()` ejecuta `CheckForClosedOrders()`
- EA itera sobre `g_OpenTickets`
- Orden 987654 sigue abierta (OrderCloseTime() == 0)

**Paso 4: Usuario cierra manualmente la posición**
- Broker cierra orden 987654 a precio 2048.75
- En siguiente `OnTick()`, EA detecta `OrderCloseTime() > 0`
- Log: `[INFO] ... | Order closed | ticket=987654,close_price=2048.75`
- EA genera close_id: `"01HKQV9Z-..."`
- EA construye TradeClose JSON
- EA llama `WritePipe(g_PipeHandle, json)`
- Log: `[INFO] ... | TradeClose sent | close_id=01HKQV9Z...,ticket=987654`
- EA remueve ticket del array: `RemoveTicketFromArray(0)`

---

## 8. CRITERIOS DE ACEPTACIÓN

### Compilación
- [ ] EA compila sin errores ni warnings en MetaEditor
- [ ] DLL `echo_pipe.dll` colocada en carpeta `MQL4/Libraries/`
- [ ] EA se carga en chart XAUUSD sin crashear MT4

### Conectividad
- [ ] EA conecta exitosamente a Named Pipe al arrancar
- [ ] Handshake JSON enviado y visible en logs del Agent
- [ ] Reconexión automática tras fallo de pipe (máx 3 intentos)

### Funcionalidad
- [ ] Click en botón "BUY" ejecuta OrderSend y genera TradeIntent
- [ ] TradeIntent tiene JSON válido (parseable por Agent)
- [ ] Campo `trade_id` es único por cada operación
- [ ] Cierre manual de posición genera TradeClose
- [ ] TradeClose tiene JSON válido con `close_id` único

### Observabilidad
- [ ] Logs estructurados visibles en Expert tab de MT4
- [ ] Logs incluyen: nivel, timestamp, evento, detalles
- [ ] Logs de errores incluyen códigos de error de MT4
- [ ] Todos los mensajes enviados loggeados con sus IDs (trade_id, close_id)

### Robustez
- [ ] EA no crashea si pipe está desconectado (solo loggea error)
- [ ] EA continúa funcionando tras error de OrderSend
- [ ] EA reconstruye array de tickets al reiniciar
- [ ] EA valida symbol == "XAUUSD" antes de enviar intents

### Validación E2E
- [ ] Ejecutar 10 operaciones BUY/SELL manualmente
- [ ] Verificar 10 TradeIntents en logs del Agent
- [ ] Cerrar las 10 posiciones manualmente
- [ ] Verificar 10 TradeCloses en logs del Agent
- [ ] 0 duplicados de trade_id o close_id
- [ ] 0 crasheos de MT4

---

## 9. NOTAS ADICIONALES

- **Symbol hardcoded**: Validar `Symbol() == "XAUUSD"` en i0. No enviar intents de otros símbolos.
- **Timestamps**: Usar `GetTickCount()` (ms desde arranque del sistema). Suficiente para latencia relativa.
- **Unidireccional**: El EA **NO recibe** mensajes del Agent en i0. Solo envía.
- **MagicNumber**: Debe ser único por estrategia. Usar mismo magic para todas las órdenes del EA.
- **Testing**: Usar cuenta demo. NO usar dinero real en i0.
- **Logs de MT4**: Revisar pestaña "Experts" en MT4 para ver logs del EA.
- **DLL permissions**: MT4 debe tener permitido el uso de DLLs (Tools → Options → Expert Advisors → "Allow DLL imports").

---

**FIN DEL PROMPT MASTER EA**
```

---

### 9.2 Prompt para Slave EA (MQL4)

```
# PROMPT PARA GENERACIÓN DE SLAVE EA MQL4 — ECHO TRADE COPIER i0

## 1. CONTEXTO DEL PROYECTO ECHO

### 1.1 Qué es Echo Trade Copier
Echo es un sistema de replicación de operaciones en trading algorítmico. Replica en tiempo real las operaciones de **cuentas Master** (origen de señales) hacia **cuentas Slave** (seguidoras).

**Arquitectura del sistema**:
```
Master EA (MQL4) → Named Pipe → Agent (Go) → gRPC → Core (Go) → gRPC → Agent → Named Pipe → Slave EA (MQL4)
```

**Flujo de una operación**:
1. Master EA ejecuta orden y envía TradeIntent
2. Core valida y transforma en ExecuteOrder
3. Core envía ExecuteOrder a Agent del Slave
4. Agent entrega ExecuteOrder al Slave EA via Named Pipe
5. **Slave EA ejecuta OrderSend** en su broker
6. **Slave EA reporta ExecutionResult** con timestamps completos
7. Core registra métricas E2E (latencia total desde Master hasta Slave)

**Objetivo de esta iteración (i0)**:
- Símbolo único: XAUUSD hardcoded
- Solo órdenes a mercado (BUY/SELL market orders)
- Sin SL/TP (Stop Loss / Take Profit)
- Lot size: 0.10 (viene en ExecuteOrder desde Core)
- 2 slaves para testing

### 1.2 Rol del Slave EA
El Slave EA es el **ejecutor de órdenes**. Su responsabilidad es:
1. **Conectarse** al Named Pipe del Agent (bidireccional: lee y escribe)
2. **Recibir** comandos del Agent: ExecuteOrder, CloseOrder
3. **Ejecutar** las órdenes en el broker local (OrderSend, OrderClose)
4. **Reportar** resultados con timestamps completos (para métricas de latencia)
5. **Registrar observabilidad**: logs estructurados para debugging y métricas

El Slave EA **NO decide** qué copiar, **NO valida** estrategia, **NO calcula** lot sizes. Solo ejecuta comandos.

---

## 2. CONTEXTO TÉCNICO DEL SLAVE EA

### 2.1 Problema a Resolver
Generar un Expert Advisor en MQL4 para MetaTrader 4 que actúe como **Slave** en el sistema Echo Trade Copier. El EA debe:
- Conectarse a Named Pipe **bidireccional** del Agent local
- **Leer comandos** (ExecuteOrder, CloseOrder) en JSON
- **Ejecutar órdenes** en el broker (OrderSend, OrderClose)
- **Escribir resultados** (ExecutionResult, CloseResult) en JSON
- Registrar **timestamps en cada hop** (t5, t6, t7) para métricas de latencia

---

## 3. REQUISITOS FUNCIONALES

### 3.1 Conexión a Named Pipe (Bidireccional)

**Named Pipe**:
- Nombre: `\\.\pipe\echo_slave_<account_id>` donde `<account_id>` = `AccountNumber()`
  - Ejemplo: Si AccountNumber() = 67890, pipe = `\\.\pipe\echo_slave_67890`
- Protocolo: JSON line-delimited (cada mensaje termina con `\n`)
- Codificación: UTF-8
- **Bidireccional**: EA **lee** comandos del pipe Y **escribe** resultados al pipe
- Timeout lectura: 1 segundo (polling con OnTimer)
- Reconexión: intentar cada 5 segundos si pipe se cierra

**Importante**: El EA **NO crea** el pipe, **solo se conecta** a un pipe existente creado por el Agent.

---

### 3.2 Handshake Inicial

**Propósito**: Registrar el EA con el Agent para que sepa qué cuenta/role/símbolo maneja.

**Cuándo**: Al conectarse exitosamente al Named Pipe (en `OnInit()` o tras reconexión).

**Formato JSON**:
```json
{
  "type": "handshake",
  "timestamp_ms": 1698345600000,
  "payload": {
    "client_id": "slave_67890",
    "account_id": "67890",
    "broker": "IC Markets",
    "role": "slave",
    "symbol": "XAUUSD",
    "version": "0.1.0"
  }
}
```

**Campos**:
- `type`: Siempre `"handshake"`
- `timestamp_ms`: `GetTickCount()`
- `payload.client_id`: `"slave_" + AccountNumber()` → ej: `"slave_67890"`
- `payload.account_id`: `AccountNumber()` como string
- `payload.broker`: `AccountCompany()`
- `payload.role`: Siempre `"slave"` para este EA
- `payload.symbol`: Siempre `"XAUUSD"` en i0
- `payload.version`: Versión del EA, ej: `"0.1.0"`

**Respuesta esperada**: El Agent **NO responde** al handshake. Si WritePipe() no falla, asumir éxito.

---

### 3.3 Recepción de Comandos

El Slave EA debe **leer continuamente** del pipe en busca de comandos. Usar `OnTimer()` con período de 100ms-1s para polling.

**Tipos de comandos**:
1. **ExecuteOrder**: Abrir posición (BUY/SELL market)
2. **CloseOrder**: Cerrar posición por ticket o magic_number

**Parsing**: Leer línea completa del pipe (hasta `\n`), parsear JSON, extraer `type` y `payload`.

**Código ejemplo** (simplificado):
```mql4
void OnTimer() {
    // Leer del pipe (Named Pipe bidireccional con DLL)
    string line = ReadPipeLine(g_PipeHandle); // Helper DLL
    if (line != "") {
        HandleCommand(line);
    }
}

void HandleCommand(string jsonLine) {
    // Parsear tipo de comando
    string type = ExtractJSONField(jsonLine, "type");
    
    if (type == "execute_order") {
        HandleExecuteOrder(jsonLine);
    } else if (type == "close_order") {
        HandleCloseOrder(jsonLine);
    } else {
        Log("WARN", "Unknown command type", "type=" + type);
    }
}
```

---

### 3.4 Comando: ExecuteOrder

**Propósito**: Recibir orden de abrir posición BUY/SELL market y ejecutarla.

**Formato JSON recibido**:
```json
{
  "type": "execute_order",
  "timestamp_ms": 1698345601050,
  "payload": {
    "command_id": "01HKQV8Y-ABCD-EF12-3456-789XYZABCDEF",
    "trade_id": "01HKQV8Y-9GJ3-F5R6-WN8P-2M4D1E123456",
    "client_id": "slave_67890",
    "account_id": "67890",
    "symbol": "XAUUSD",
    "order_side": "BUY",
    "lot_size": 0.10,
    "magic_number": 123456,
    "timestamps": {
      "t0_master_ea_ms": 1698345601000,
      "t1_agent_recv_ms": 1698345601010,
      "t2_core_recv_ms": 1698345601020,
      "t3_core_send_ms": 1698345601030,
      "t4_agent_recv_ms": 1698345601040
    }
  }
}
```

**Campos clave**:
- `command_id`: UUID único del comando (para idempotencia)
- `trade_id`: UUID del TradeIntent original del Master
- `symbol`: Siempre `"XAUUSD"` en i0
- `order_side`: `"BUY"` o `"SELL"`
- `lot_size`: Tamaño calculado por Core (ej: 0.10)
- `magic_number`: **Mismo MagicNumber del Master** (para trazabilidad)
- `timestamps`: Timestamps acumulados de los hops previos

**Acción del EA**:
1. Agregar timestamp `t5_slave_ea_recv_ms = GetTickCount()`
2. Validar `symbol == "XAUUSD"`
3. Determinar precio: BUY → Ask, SELL → Bid
4. Determinar op: BUY → OP_BUY, SELL → OP_SELL
5. Agregar timestamp `t6_order_send_ms = GetTickCount()`
6. Ejecutar `OrderSend(symbol, op, lot_size, price, 3, 0, 0, "Echo Slave", magic_number)`
7. Agregar timestamp `t7_order_filled_ms = GetTickCount()`
8. Construir `ExecutionResult` con timestamps completos
9. Enviar `ExecutionResult` al pipe

**Código ejemplo**:
```mql4
void HandleExecuteOrder(string jsonLine) {
    // Timestamp t5: recepción en EA
    ulong t5 = GetTickCount();
    
    // Parsear payload
    string commandId = ExtractJSONField(jsonLine, "command_id");
    string tradeId = ExtractJSONField(jsonLine, "trade_id");
    string symbol = ExtractJSONField(jsonLine, "symbol");
    string orderSide = ExtractJSONField(jsonLine, "order_side");
    double lotSize = StringToDouble(ExtractJSONField(jsonLine, "lot_size"));
    int magicNumber = (int)StringToInteger(ExtractJSONField(jsonLine, "magic_number"));
    
    // Timestamps previos (del Agent/Core/Master)
    ulong t0 = (ulong)StringToInteger(ExtractNestedJSONField(jsonLine, "timestamps.t0_master_ea_ms"));
    ulong t1 = (ulong)StringToInteger(ExtractNestedJSONField(jsonLine, "timestamps.t1_agent_recv_ms"));
    ulong t2 = (ulong)StringToInteger(ExtractNestedJSONField(jsonLine, "timestamps.t2_core_recv_ms"));
    ulong t3 = (ulong)StringToInteger(ExtractNestedJSONField(jsonLine, "timestamps.t3_core_send_ms"));
    ulong t4 = (ulong)StringToInteger(ExtractNestedJSONField(jsonLine, "timestamps.t4_agent_recv_ms"));
    
    // Validar símbolo
    if (symbol != "XAUUSD") {
        Log("ERROR", "Invalid symbol in ExecuteOrder", "symbol=" + symbol);
        SendExecutionResult(commandId, tradeId, false, 0, 999, "Invalid symbol", 0, t0, t1, t2, t3, t4, t5, 0, 0);
        return;
    }
    
    // Determinar precio y operación
    int op = (orderSide == "BUY") ? OP_BUY : OP_SELL;
    double price = (op == OP_BUY) ? Ask : Bid;
    
    // Timestamp t6: antes de OrderSend
    ulong t6 = GetTickCount();
    
    // Ejecutar orden
    int ticket = OrderSend(symbol, op, lotSize, price, 3, 0, 0, "Echo Slave", magicNumber, 0, clrGreen);
    
    // Timestamp t7: después de OrderSend
    ulong t7 = GetTickCount();
    
    // Preparar resultado
    bool success = (ticket > 0);
    int errorCode = success ? 0 : GetLastError();
    string errorMsg = success ? "" : ErrorDescription(errorCode);
    double executedPrice = success ? OrderOpenPrice() : 0;
    
    // Log
    if (success) {
        Log("INFO", "Order executed", "command_id=" + commandId + ",ticket=" + IntegerToString(ticket) + ",price=" + DoubleToString(executedPrice, Digits));
    } else {
        Log("ERROR", "OrderSend failed", "command_id=" + commandId + ",error=" + IntegerToString(errorCode) + ",desc=" + errorMsg);
    }
    
    // Enviar resultado
    SendExecutionResult(commandId, tradeId, success, ticket, errorCode, errorMsg, executedPrice, t0, t1, t2, t3, t4, t5, t6, t7);
}
```

---

### 3.5 Reporte: ExecutionResult

**Propósito**: Reportar al Agent el resultado de la ejecución (éxito o error) con timestamps completos.

**Formato JSON a enviar**:
```json
{
  "type": "execution_result",
  "timestamp_ms": 1698345601150,
  "payload": {
    "command_id": "01HKQV8Y-ABCD-EF12-3456-789XYZABCDEF",
    "trade_id": "01HKQV8Y-9GJ3-F5R6-WN8P-2M4D1E123456",
    "client_id": "slave_67890",
    "success": true,
    "ticket": 111222,
    "error_code": "ERR_NO_ERROR",
    "error_message": "",
    "executed_price": 2045.52,
    "timestamps": {
      "t0_master_ea_ms": 1698345601000,
      "t1_agent_recv_ms": 1698345601010,
      "t2_core_recv_ms": 1698345601020,
      "t3_core_send_ms": 1698345601030,
      "t4_agent_recv_ms": 1698345601040,
      "t5_slave_ea_recv_ms": 1698345601050,
      "t6_order_send_ms": 1698345601100,
      "t7_order_filled_ms": 1698345601140
    }
  }
}
```

**Campos clave**:
- `command_id`: Mismo del ExecuteOrder (idempotencia)
- `trade_id`: Trade ID original del Master
- `success`: `true` si ticket > 0, `false` si error
- `ticket`: Ticket MT4 del slave (0 si fallo)
- `error_code`: String del error code (ej: `"ERR_NO_ERROR"`, `"ERR_NOT_ENOUGH_MONEY"`)
- `error_message`: Descripción del error (vacío si success)
- `executed_price`: Precio de ejecución (`OrderOpenPrice()` si success)
- `timestamps`: **Todos los timestamps** desde t0 hasta t7

**Código ejemplo**:
```mql4
void SendExecutionResult(string commandId, string tradeId, bool success, int ticket, int errorCode, string errorMsg, double execPrice,
                         ulong t0, ulong t1, ulong t2, ulong t3, ulong t4, ulong t5, ulong t6, ulong t7) {
    string json = "{";
    json += "\"type\":\"execution_result\",";
    json += "\"timestamp_ms\":" + IntegerToString(GetTickCount()) + ",";
    json += "\"payload\":{";
    json += "\"command_id\":\"" + commandId + "\",";
    json += "\"trade_id\":\"" + tradeId + "\",";
    json += "\"client_id\":\"slave_" + IntegerToString(AccountNumber()) + "\",";
    json += "\"success\":" + (success ? "true" : "false") + ",";
    json += "\"ticket\":" + IntegerToString(ticket) + ",";
    json += "\"error_code\":\"" + MapErrorCode(errorCode) + "\",";
    json += "\"error_message\":\"" + EscapeJSON(errorMsg) + "\",";
    json += "\"executed_price\":" + DoubleToString(execPrice, Digits) + ",";
    json += "\"timestamps\":{";
    json += "\"t0_master_ea_ms\":" + IntegerToString(t0) + ",";
    json += "\"t1_agent_recv_ms\":" + IntegerToString(t1) + ",";
    json += "\"t2_core_recv_ms\":" + IntegerToString(t2) + ",";
    json += "\"t3_core_send_ms\":" + IntegerToString(t3) + ",";
    json += "\"t4_agent_recv_ms\":" + IntegerToString(t4) + ",";
    json += "\"t5_slave_ea_recv_ms\":" + IntegerToString(t5) + ",";
    json += "\"t6_order_send_ms\":" + IntegerToString(t6) + ",";
    json += "\"t7_order_filled_ms\":" + IntegerToString(t7);
    json += "}}}";
    json += "\n";
    
    if (WritePipe(g_PipeHandle, json) > 0) {
        Log("INFO", "ExecutionResult sent", "command_id=" + commandId + ",success=" + (success ? "true" : "false"));
    } else {
        Log("ERROR", "WritePipe failed for ExecutionResult", "command_id=" + commandId);
    }
}

string MapErrorCode(int err) {
    switch(err) {
        case 0: return "ERR_NO_ERROR";
        case 129: return "ERR_INVALID_PRICE";
        case 130: return "ERR_INVALID_STOPS";
        case 136: return "ERR_OFF_QUOTES";
        case 138: return "ERR_REQUOTE";
        case 137: return "ERR_BROKER_BUSY";
        case 141: return "ERR_TIMEOUT";
        case 133: return "ERR_TRADE_DISABLED";
        case 132: return "ERR_MARKET_CLOSED";
        case 134: return "ERR_NOT_ENOUGH_MONEY";
        case 135: return "ERR_PRICE_CHANGED";
        default: return "ERR_UNKNOWN";
    }
}
```

---

### 3.6 Comando: CloseOrder

**Propósito**: Recibir orden de cerrar posición y ejecutarla.

**Formato JSON recibido**:
```json
{
  "type": "close_order",
  "timestamp_ms": 1698345700050,
  "payload": {
    "command_id": "01HKQV9Z-1CLO-SECM-D123-456789ABCDEF",
    "close_id": "01HKQV9Z-1A2B-3C4D-5E6F-7G8H9I123456",
    "client_id": "slave_67890",
    "account_id": "67890",
    "ticket": 111222,
    "magic_number": 123456,
    "symbol": "XAUUSD"
  }
}
```

**Campos clave**:
- `command_id`: UUID único del comando de cierre
- `close_id`: Close ID del evento de cierre del Master
- `ticket`: Ticket del slave a cerrar (si 0, buscar por magic_number)
- `magic_number`: Para buscar posición si ticket=0
- `symbol`: Símbolo (validación)

**Acción del EA**:
1. Buscar orden: si `ticket > 0`, usar `OrderSelect(ticket)`, sino buscar por `magic_number` y `symbol`
2. Obtener precio de cierre: BUY → Bid, SELL → Ask
3. Ejecutar `OrderClose(ticket, lots, closePrice, 3)`
4. Reportar `CloseResult`

**Código ejemplo**:
```mql4
void HandleCloseOrder(string jsonLine) {
    string commandId = ExtractJSONField(jsonLine, "command_id");
    string closeId = ExtractJSONField(jsonLine, "close_id");
    int ticket = (int)StringToInteger(ExtractJSONField(jsonLine, "ticket"));
    int magicNumber = (int)StringToInteger(ExtractJSONField(jsonLine, "magic_number"));
    string symbol = ExtractJSONField(jsonLine, "symbol");
    
    // Buscar orden
    if (ticket > 0) {
        if (!OrderSelect(ticket, SELECT_BY_TICKET)) {
            Log("ERROR", "OrderSelect failed for CloseOrder", "ticket=" + IntegerToString(ticket));
            SendCloseResult(commandId, closeId, false, ticket, 4108, "Ticket not found", 0);
            return;
        }
    } else {
        // Buscar por magic y symbol
        ticket = FindOpenOrderByMagicAndSymbol(magicNumber, symbol);
        if (ticket <= 0) {
            Log("ERROR", "No open order found", "magic=" + IntegerToString(magicNumber) + ",symbol=" + symbol);
            SendCloseResult(commandId, closeId, false, 0, 4108, "No order found", 0);
            return;
        }
        OrderSelect(ticket, SELECT_BY_TICKET);
    }
    
    // Obtener precio de cierre
    double closePrice = (OrderType() == OP_BUY) ? Bid : Ask;
    double lots = OrderLots();
    
    // Cerrar orden
    bool success = OrderClose(ticket, lots, closePrice, 3, clrRed);
    int errorCode = success ? 0 : GetLastError();
    string errorMsg = success ? "" : ErrorDescription(errorCode);
    double actualClosePrice = success ? OrderClosePrice() : 0;
    
    // Log
    if (success) {
        Log("INFO", "Order closed", "ticket=" + IntegerToString(ticket) + ",close_price=" + DoubleToString(actualClosePrice, Digits));
    } else {
        Log("ERROR", "OrderClose failed", "ticket=" + IntegerToString(ticket) + ",error=" + IntegerToString(errorCode));
    }
    
    // Enviar resultado
    SendCloseResult(commandId, closeId, success, ticket, errorCode, errorMsg, actualClosePrice);
}

int FindOpenOrderByMagicAndSymbol(int magic, string symbol) {
    for (int i = OrdersTotal() - 1; i >= 0; i--) {
        if (OrderSelect(i, SELECT_BY_POS) && OrderMagicNumber() == magic && OrderSymbol() == symbol && OrderCloseTime() == 0) {
            return OrderTicket();
        }
    }
    return -1;
}
```

---

### 3.7 Reporte: CloseResult

**Formato JSON a enviar**:
```json
{
  "type": "close_result",
  "timestamp_ms": 1698345700150,
  "payload": {
    "command_id": "01HKQV9Z-1CLO-SECM-D123-456789ABCDEF",
    "close_id": "01HKQV9Z-1A2B-3C4D-5E6F-7G8H9I123456",
    "client_id": "slave_67890",
    "success": true,
    "ticket": 111222,
    "error_code": "ERR_NO_ERROR",
    "error_message": "",
    "close_price": 2048.74
  }
}
```

**Código ejemplo** (similar a ExecutionResult, más simple):
```mql4
void SendCloseResult(string commandId, string closeId, bool success, int ticket, int errorCode, string errorMsg, double closePrice) {
    string json = "{";
    json += "\"type\":\"close_result\",";
    json += "\"timestamp_ms\":" + IntegerToString(GetTickCount()) + ",";
    json += "\"payload\":{";
    json += "\"command_id\":\"" + commandId + "\",";
    json += "\"close_id\":\"" + closeId + "\",";
    json += "\"client_id\":\"slave_" + IntegerToString(AccountNumber()) + "\",";
    json += "\"success\":" + (success ? "true" : "false") + ",";
    json += "\"ticket\":" + IntegerToString(ticket) + ",";
    json += "\"error_code\":\"" + MapErrorCode(errorCode) + "\",";
    json += "\"error_message\":\"" + EscapeJSON(errorMsg) + "\",";
    json += "\"close_price\":" + DoubleToString(closePrice, Digits);
    json += "}}";
    json += "\n";
    
    if (WritePipe(g_PipeHandle, json) > 0) {
        Log("INFO", "CloseResult sent", "command_id=" + commandId);
    } else {
        Log("ERROR", "WritePipe failed for CloseResult", "command_id=" + commandId);
    }
}
```

---

## 4. OBSERVABILIDAD (CRÍTICO)

### 4.1 Sistema de Logging Estructurado

**Formato idéntico al Master EA**:
```
[LEVEL] timestamp_ms | event | key1=value1,key2=value2,...
```

**Función Log** (igual que Master):
```mql4
void Log(string level, string event, string details) {
    ulong ts = GetTickCount();
    string logLine = "[" + level + "] " + IntegerToString(ts) + " | " + event + " | " + details;
    Print(logLine);
}
```

**Eventos clave a loggear**:
1. **Conexión** al pipe: `"Pipe connected"` (INFO)
2. **Handshake** enviado: `"Handshake sent"` (INFO)
3. **Comando recibido**: `"Command received"` con tipo y command_id (INFO)
4. **Orden ejecutada**: `"Order executed"` con ticket y precio (INFO)
5. **ExecutionResult enviado**: `"ExecutionResult sent"` con command_id y success (INFO)
6. **Orden cerrada**: `"Order closed"` con ticket (INFO)
7. **CloseResult enviado**: `"CloseResult sent"` con command_id (INFO)
8. **Error de OrderSend/OrderClose**: con código de error MT4 (ERROR)
9. **Error de pipe**: `"ReadPipe failed"` o `"WritePipe failed"` (ERROR)
10. **Reconexión**: `"Attempting pipe reconnection"` (WARN)

---

### 4.2 Input Parameters

```mql4
input string PipeBaseName = "\\\\.\\\pipe\\\\echo_slave_";   // Base del pipe name
input string Symbol = "XAUUSD";                              // Símbolo hardcoded
input bool   EnableDebugLogs = false;                        // Activar logs DEBUG
input bool   LogToFile = false;                              // Escribir logs a archivo
input int    TimerPeriodMs = 1000;                           // Período del timer (ms) para polling del pipe
```

---

## 5. REQUISITOS NO FUNCIONALES

### 5.1 Named Pipe Bidireccional en MQL4 (DLL Externa)

**Diferencia con Master EA**: El Slave necesita **leer Y escribir**, no solo escribir.

**DLL necesaria** (extender `echo_pipe.dll`):
```cpp
// Agregar función ReadPipe
extern "C" __declspec(dllexport) int ReadPipeLine(int handle, char* buffer, int bufferSize) {
    DWORD bytesRead;
    BOOL result = ReadFile((HANDLE)handle, buffer, bufferSize - 1, &bytesRead, NULL);
    if (result && bytesRead > 0) {
        buffer[bytesRead] = '\0';
        return bytesRead;
    }
    return -1;
}
```

**Import en MQL4**:
```mql4
#import "echo_pipe.dll"
   int ConnectPipe(string pipeName);
   int WritePipe(int handle, string data);
   int ReadPipeLine(int handle, uchar &buffer[], int bufferSize);  // Nueva función
   void ClosePipe(int handle);
#import
```

**Helper de lectura**:
```mql4
string ReadPipeLineString(int handle) {
    uchar buffer[8192];
    ArrayInitialize(buffer, 0);
    int bytesRead = ReadPipeLine(handle, buffer, 8192);
    if (bytesRead > 0) {
        return CharArrayToString(buffer, 0, bytesRead);
    }
    return "";
}
```

---

### 5.2 Polling con OnTimer

```mql4
void OnInit() {
    ConnectToPipe();
    SendHandshake();
    EventSetTimer(TimerPeriodMs / 1000);  // Convertir ms a segundos
    Log("INFO", "EA initialized", "timer_period_ms=" + IntegerToString(TimerPeriodMs));
}

void OnTimer() {
    string line = ReadPipeLineString(g_PipeHandle);
    while (line != "") {
        HandleCommand(line);
        line = ReadPipeLineString(g_PipeHandle);  // Leer siguiente línea si hay
    }
}
```

---

## 6. ESTRUCTURA DE ARCHIVOS

```
MQL4/
├── Experts/
│   └── EchoSlaveEA.mq4              // Código principal del EA
├── Libraries/
│   └── echo_pipe.dll                // DLL bidireccional (lectura + escritura)
└── Files/
    └── echo_slave_<account>.log     // Logs locales (opcional)
```

---

## 7. FLUJO COMPLETO DE EJECUCIÓN (EJEMPLO)

**Paso 1: Usuario carga EA en chart XAUUSD**
- `OnInit()` ejecuta
- EA conecta a pipe `\\.\pipe\echo_slave_67890`
- EA envía handshake
- EA arranca timer con período 1s
- Log: `[INFO] ... | EA initialized | timer_period_ms=1000`

**Paso 2: Core envía ExecuteOrder al Agent**
- Agent escribe JSON en pipe del slave

**Paso 3: OnTimer() del Slave EA lee del pipe**
- `ReadPipeLineString()` retorna línea JSON de ExecuteOrder
- EA parsea: command_id, trade_id, symbol=XAUUSD, order_side=BUY, lot_size=0.10, magic=123456
- Log: `[INFO] ... | Command received | type=execute_order,command_id=01HKQV8Y...`
- EA valida symbol == "XAUUSD"
- EA registra timestamps: t5=GetTickCount()
- EA determina precio: Ask=2045.50
- EA registra t6=GetTickCount()
- EA ejecuta: `OrderSend("XAUUSD", OP_BUY, 0.10, 2045.50, 3, 0, 0, "Echo Slave", 123456)`
- OrderSend retorna ticket=111222
- EA registra t7=GetTickCount()
- Log: `[INFO] ... | Order executed | command_id=01HKQV8Y...,ticket=111222,price=2045.50`
- EA construye ExecutionResult con timestamps completos (t0-t7)
- EA envía ExecutionResult al pipe
- Log: `[INFO] ... | ExecutionResult sent | command_id=01HKQV8Y...,success=true`

**Paso 4: Master cierra posición, Core envía CloseOrder**
- Agent escribe JSON de CloseOrder en pipe del slave

**Paso 5: OnTimer() lee CloseOrder**
- EA parsea: command_id, close_id, ticket=111222, magic=123456
- Log: `[INFO] ... | Command received | type=close_order,command_id=01HKQV9Z...`
- EA selecciona orden 111222
- EA obtiene precio de cierre: Bid=2048.74
- EA ejecuta: `OrderClose(111222, 0.10, 2048.74, 3)`
- OrderClose retorna true
- Log: `[INFO] ... | Order closed | ticket=111222,close_price=2048.74`
- EA construye CloseResult
- EA envía CloseResult al pipe
- Log: `[INFO] ... | CloseResult sent | command_id=01HKQV9Z...`

---

## 8. CRITERIOS DE ACEPTACIÓN

### Compilación
- [ ] EA compila sin errores ni warnings en MetaEditor
- [ ] DLL `echo_pipe.dll` (con funciones read/write) en `MQL4/Libraries/`
- [ ] EA se carga en chart XAUUSD sin crashear MT4

### Conectividad
- [ ] EA conecta exitosamente a Named Pipe al arrancar
- [ ] Handshake JSON enviado y visible en logs del Agent
- [ ] Polling funciona (OnTimer lee del pipe sin bloquear)

### Funcionalidad
- [ ] EA recibe ExecuteOrder y ejecuta OrderSend correctamente
- [ ] ExecutionResult tiene JSON válido con timestamps t0-t7 completos
- [ ] EA recibe CloseOrder y ejecuta OrderClose correctamente
- [ ] CloseResult tiene JSON válido

### Observabilidad
- [ ] Logs estructurados visibles en Expert tab de MT4
- [ ] Logs incluyen: nivel, timestamp, evento, detalles
- [ ] Todos los command_id loggeados para trazabilidad

### Robustez
- [ ] EA no crashea si pipe está desconectado (solo loggea error)
- [ ] EA continúa funcionando tras error de OrderSend/OrderClose
- [ ] EA valida symbol == "XAUUSD" antes de ejecutar
- [ ] MagicNumber replicado correctamente en todas las órdenes

### Validación E2E
- [ ] 10 órdenes ejecutadas desde Master
- [ ] 10 ExecutionResults con success=true
- [ ] 10 cierres desde Master
- [ ] 10 CloseResults con success=true
- [ ] Timestamps completos (t0-t7) en todos los ExecutionResults
- [ ] 0 crasheos de MT4

---

## 9. NOTAS ADICIONALES

- **Symbol hardcoded**: Validar `symbol == "XAUUSD"`. Rechazar otros.
- **MagicNumber del Master**: Replicar exacto en OrderSend para trazabilidad.
- **Timestamps**: Críticos para métricas de latencia E2E. Incluir todos (t0-t7).
- **Error codes**: Usar `MapErrorCode()` para convertir int a string legible.
- **Polling period**: 1s por defecto, ajustable con input param.
- **Testing**: Usar cuenta demo. NO dinero real en i0.
- **DLL permissions**: Habilitar en MT4 (Tools → Options → Expert Advisors).

---

**FIN DEL PROMPT SLAVE EA**
```

---
#### Test 4: Dedupe (Reintento de TradeIntent)
**Objetivo**: Validar que Core rechaza duplicados
1. Arrancar Agent con logging DEBUG
2. Simular envío duplicado de TradeIntent (mismo trade_id)
   - Opción: modificar Master EA para enviar 2 veces
   - Opción: replay manual desde logs
3. Verificar logs Core: primer intent procesado, segundo rechazado
4. Verificar: solo 1 ExecuteOrder generado

**Criterios**:
- 1 ejecución en slaves
- Log de rechazo visible: "Duplicate trade"

---

#### Test 5: Cierre de Posición (Master cierra → Slaves cierran)
**Objetivo**: Validar propagación de cierres
1. Master1 abre BUY (ejecutado en slaves)
2. Esperar 5 segundos
3. Master1 cierra posición manualmente
4. Verificar: Master EA envía TradeClose
5. Verificar logs Core: TradeClose procesado, 2 CloseOrders generados
6. Verificar: Slave1 y Slave2 cierran posiciones
7. Verificar: 2 CloseResults recibidos en Core

**Criterios**:
- Posiciones cerradas en slaves en < 2 segundos tras cierre del master
- 0 órdenes abiertas remanentes

---

#### Test 6: Error Handling (Orden Rechazada por Broker)
**Objetivo**: Validar manejo de errores de ejecución
1. Desconectar 1 Slave EA de la red (simular broker offline)
2. Master1 envía BUY
3. Verificar: Slave1 rechaza con error (ERR_NO_CONNECTION o similar)
4. Verificar: ExecutionResult con success=false, error_code != ERR_NO_ERROR
5. Verificar logs: error registrado con detalle

**Criterios**:
- Core recibe ExecutionResult con éxito del Slave2 y error del Slave1
- No crashea ningún componente

---

#### Test 7: Latencia E2E (10 Ejecuciones)
**Objetivo**: Medir latencia y estabilidad
1. Ejecutar 10 órdenes BUY desde Master1 (esperar 5s entre cada una)
2. Recolectar timestamps t0...t7 de cada ejecución
3. Calcular latencias:
   - E2E: t7 - t0
   - Agent→Core: t2 - t1
   - Core processing: t3 - t2
   - Core→Agent: t4 - t3
   - Slave execution: t7 - t5
4. Calcular p50, p95, p99

**Criterios**:
- p95 latency E2E < 120ms
- 0 outliers > 500ms
- 0 errores en 10 ejecuciones

---

#### Test 8: Símbolo Inválido (Validación)
**Objetivo**: Validar whitelist de símbolos
1. Modificar Master EA para enviar TradeIntent con symbol="EURUSD"
2. Verificar logs Core: intent rechazado (symbol no en whitelist)
3. Verificar: 0 ExecuteOrders generados

**Criterios**:
- Log de rechazo: "Invalid symbol: EURUSD"
- 0 ejecuciones en slaves

---

#### Test 9: Desconexión y Reconexión de Agent
**Objetivo**: Validar reconexión (básico, sin persistencia)
1. Master1 abre BUY (ejecutado en slaves)
2. Detener Agent (kill process)
3. Verificar: Core detecta desconexión del stream
4. Reiniciar Agent
5. Verificar: Agent reconecta al Core
6. Master1 abre SELL
7. Verificar: ejecutado en slaves

**Criterios**:
- Reconexión exitosa en < 10s
- Segunda orden ejecutada correctamente
- **Nota**: sin persistencia, órdenes enviadas durante caída se pierden (esperado en i0)

---

#### Test 10: Procesamiento Secuencial (Orden de Ejecución)
**Objetivo**: Validar que Core procesa intents en orden FIFO
1. Master1 envía BUY (intent1)
2. Inmediatamente Master2 envía SELL (intent2)
3. Verificar logs Core: intent1 procesado antes que intent2
4. Verificar timestamps t2 y t3: intent1.t3 < intent2.t2 (intent1 sale antes de que intent2 llegue)

**Criterios**:
- Orden FIFO respetado
- 0 race conditions

---

### 10.3 Métricas a Recolectar

Por cada test:
- **Latencia E2E** (ms): t7 - t0
- **Latencia por hop**:
  - Agent recv: t1 - t0
  - Agent→Core: t2 - t1
  - Core process: t3 - t2
  - Core→Agent: t4 - t3
  - Agent→Slave: t5 - t4
  - Slave exec: t7 - t6
- **Tasa de éxito**: órdenes ejecutadas / órdenes enviadas
- **Errores**: count por tipo (ERR_REQUOTE, ERR_OFF_QUOTES, etc.)
- **Duplicados detectados**: count

### 10.4 Logs Esperados (Ejemplo)

**Agent**:
```
[INFO] Agent started, connecting to Core at localhost:50051
[INFO] gRPC stream connected
[INFO] Named Pipe created: \\.\pipe\echo_master_12345
[INFO] Named Pipe created: \\.\pipe\echo_slave_67890
[INFO] Handshake received from master_12345
[INFO] TradeIntent received: trade_id=01HKQV8Y..., symbol=XAUUSD, side=BUY
[INFO] TradeIntent forwarded to Core
[INFO] ExecuteOrder received from Core: command_id=01HKQV8Z..., slave=67890
[INFO] ExecuteOrder sent to slave_67890
[INFO] ExecutionResult received from slave_67890: success=true, ticket=111222
```

**Core**:
```
[INFO] Core started, listening on :50051
[INFO] Agent connected: agent_1698345600
[INFO] TradeIntent received: trade_id=01HKQV8Y..., from=master_12345
[INFO] Validation passed: symbol=XAUUSD
[INFO] Dedupe check: new trade
[INFO] ExecuteOrder created: command_id=01HKQV8Z..., lot_size=0.10
[INFO] ExecuteOrder sent to agent_1698345600
[INFO] ExecutionResult received: command_id=01HKQV8Z..., success=true, latency_e2e=85ms
```

---

## 11. Criterios de Aceptación Final (Iteración 0)

### Funcionales
- [ ] 10 ejecuciones consecutivas sin duplicados
- [ ] 0 cruces de datos entre 2 masters y 2 slaves
- [ ] MagicNumber replicado correctamente en todas las órdenes
- [ ] Cierres del master propagados a slaves en < 2s
- [ ] Símbolo XAUUSD validado (otros rechazados)

### No Funcionales
- [ ] Latencia p95 E2E < 120ms (10 muestras mínimo)
- [ ] Latencia p99 E2E < 200ms
- [ ] 0 memory leaks (ejecutar 100 órdenes, monitorear RAM)
- [ ] 0 goroutine leaks (Agent y Core estables tras 1h)

### Telemetría
- [ ] Métricas EchoMetrics visibles en logs o Prometheus
- [ ] Timestamps completos (t0-t7) en todos los ExecutionResults
- [ ] Logs estructurados (JSON) en Agent y Core
- [ ] Traces OTEL propagados (trace_id compartido)

### Código
- [ ] Todos los TODOs marcados para i1 (config, concurrencia, persistencia)
- [ ] Proto generado sin warnings
- [ ] Linters OK (`golangci-lint`, `staticcheck`)
- [ ] Tests unitarios de serialización proto (Go)

---

## 12. Riesgos y Mitigaciones

| Riesgo | Probabilidad | Impacto | Mitigación |
|--------|--------------|---------|------------|
| Named Pipes inestables en Windows | Media | Alto | Implementar watchdog + reconexión automática; logs detallados de errores de pipe |
| Parsing JSON en MQL4 frágil | Alta | Medio | Validar JSON con tests extensivos; agregar logs de JSON raw en caso de error |
| Latencia > 120ms en i0 | Media | Medio | Optimizar serialización; reducir overhead de Named Pipes; usar buffers adecuados |
| Cruce de datos por race condition | Baja | Crítico | Tests exhaustivos con 2 masters concurrentes; locks adecuados en dedupe |
| Dedupe map crece indefinidamente | Baja | Medio | Implementar cleanup cada 1 min (TTL 1h) |
| DLL de Named Pipes no disponible | Alta | Crítico | Proveer DLL precompilada o código fuente C++; documentar compilación |
| gRPC stream se cae sin reconexión | Media | Alto | Implementar reconnection backoff en Agent (TODO i1, pero puede ser necesario antes) |

---

## 13. Entregables Finales de Iteración 0

### Código
- [ ] `sdk/proto/v0/*.proto` (contratos)
- [ ] `sdk/telemetry/bundle_echo.go` (métricas)
- [ ] `agent/main.go` + paquetes internos
- [ ] `core/main.go` + paquetes internos
- [ ] `clients/master_ea/EchoMasterEA.mq4`
- [ ] `clients/slave_ea/EchoSlaveEA.mq4`
- [ ] `clients/libraries/echo_pipe.dll` (o código fuente C++)

### Documentación
- [ ] Este RFC-002 (actualizado tras implementación)
- [ ] README con instrucciones de setup
- [ ] Guía de compilación de DLL
- [ ] Logs de testing con resultados de 10 ejecuciones

### Artefactos de Testing
- [ ] Logs de Agent (10 ejecuciones)
- [ ] Logs de Core (10 ejecuciones)
- [ ] Screenshots de terminales MT4 mostrando órdenes
- [ ] Métricas de latencia (CSV o JSON)

---

## 14. Próximos Pasos (Post-i0 → i1)

Una vez completada la iteración 0, los próximos pasos son:

1. **Persistencia** (Postgres):
   - Tabla `orders` con estado
   - Tabla `dedupe` para sobrevivir reinicio
2. **Configuración** (etcd):
   - Políticas por cuenta
   - Mapeo de slaves
3. **Concurrencia** en Core:
   - Procesamiento paralelo con locks por trade_id
4. **Reintentos**:
   - Backoff exponencial
   - Filtros de errores retriables
5. **Telemetría avanzada**:
   - Dashboards Grafana
   - Alertas por latencia/errores

---

## 15. Referencias

- [RFC-001: Arquitectura](../RFC-001-architecture.md)
- [ADR-002: gRPC Transport](../adr/002-grpc-transport.md)
- [ADR-003: Named Pipes IPC](../adr/003-named-pipes-ipc.md)
- [MQL4 Documentation](https://docs.mql4.com/)
- [gRPC Go Documentation](https://grpc.io/docs/languages/go/)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/instrumentation/go/)

---

## 16. Resumen de Arquitectura SDK-First

### Principio Fundamental
**Todo código reutilizable vive en SDK**. Agent y Core son **thin layers** que solo hacen routing y orquestación.

### División de Responsabilidades

| Componente | Responsabilidad | Usa SDK |
|------------|-----------------|---------|
| **SDK** | Contratos, validaciones, transformaciones, gRPC, Named Pipes, telemetría, utils | - |
| **Agent** | Routing Named Pipes ↔ gRPC | ✅ Todo via SDK |
| **Core** | Orquestación, dedupe, routing | ✅ Todo via SDK |
| **EAs** | Generación de intents (Master), ejecución (Slave) | ❌ MQL4 nativo |

### Paquetes SDK Desarrollados en i0

```
sdk/
├── proto/v0/          ✅ Contratos Proto generados
├── domain/            ✅ Validaciones + Transformers
├── grpc/              ✅ Cliente/Server genérico
├── ipc/               ✅ Named Pipes abstraction
├── telemetry/         ✅ EchoMetrics bundle
└── utils/             ✅ UUIDv7, timestamps, JSON
```

### Ejemplo de Uso (Agent)

```go
// ❌ MAL: Reimplementar en Agent
func (a *Agent) parseTradeIntent(json string) (*pb.TradeIntent, error) {
    // Parsing manual...
}

// ✅ BIEN: Usar SDK
import "github.com/xKoRx/echo/sdk/domain"

func (a *Agent) handlePipeMessage(msg map[string]interface{}) {
    intent, err := domain.JSONToTradeIntent(msg) // SDK
    // ...
}
```

### Beneficios

1. **Reutilización**: Agent y Core comparten 100% de lógica común
2. **Testing**: SDK se testea independientemente (>80% coverage)
3. **Mantenibilidad**: Cambios en transformers/validaciones en 1 solo lugar
4. **Escalabilidad**: Futuros componentes (ej: Dashboard) usan misma SDK
5. **Documentación**: SDK documenta contratos y comportamientos

### Validación en Code Review

Al revisar Agent/Core, verificar:
- [ ] ❌ No hay parsing manual de JSON (usar `sdk/domain`)
- [ ] ❌ No hay validaciones de negocio (usar `sdk/domain.Validate()`)
- [ ] ❌ No hay creación manual de pipes (usar `sdk/ipc`)
- [ ] ❌ No hay lógica de gRPC custom (usar `sdk/grpc`)
- [ ] ❌ No hay generación de UUIDs custom (usar `sdk/utils.GenerateUUIDv7()`)
- [ ] ✅ Solo lógica de routing y orquestación específica del componente

---

**Fin RFC-002 v1.0**

**Autores**: Aranea Labs - Trading Copier Team  
**Fecha**: 2025-10-24  
**Status**: Draft (Pendiente de aprobación)

---

## Apéndice: Checklist de Desarrollo SDK-First

### Antes de escribir código en Agent/Core:
1. ¿Esta lógica es reutilizable? → Va en SDK
2. ¿Es transformación de datos? → `sdk/domain`
3. ¿Es validación de negocio? → `sdk/domain`
4. ¿Es I/O (gRPC, pipes)? → `sdk/grpc` o `sdk/ipc`
5. ¿Es telemetría? → `sdk/telemetry`
6. ¿Es utilidad (UUID, timestamp)? → `sdk/utils`
7. ¿Es routing/orquestación específica? → OK en Agent/Core

### Cuando crees un PR:
- Asegúrate que `go.mod` de Agent/Core solo dependa de SDK (no reimplemente)
- Tests unitarios de SDK completos antes de integrar en Agent/Core
- Documentación godoc en SDK actualizada

