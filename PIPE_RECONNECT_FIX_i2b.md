# 🔧 Fix i2b: Reconexión Automática de Named Pipes

**Fecha:** 2025-10-30  
**Versión:** i2b (Iteración 2b - Reconexión robusta)  
**Problema:** Después de ~30 minutos, todos los EAs se desconectan y no pueden reconectar

---

## 🔴 Problema Raíz Identificado

### Diagnóstico Confirmado

**Hipótesis 1 era correcta:** El listener se cerraba después del EOF y no volvía a aceptar conexiones.

#### Flujo del problema:

1. **EA se desconecta** (timeout, cierre inesperado, etc.) → Agent recibe `EOF`
2. **`PipeHandler.Run()` retorna** → Sale completamente del handler
3. **`PipeHandler.Close()` se llama** → Cierra **TANTO** `currentConn` **COMO** `listener`
4. **Listener cerrado** → Ya no puede aceptar nuevas conexiones
5. **EAs intentan reconectar** → Pipe **no existe** → Bucle infinito de reconexión

#### Evidencia en logs:

```
02:16:10 - Client disconnected (EOF detected) - master_2089126182
02:16:11 - Client disconnected (EOF detected) - master_2089126187
02:16:12 - Client disconnected (EOF detected) - slave_2089126186
02:52:55 - Client disconnected (EOF detected) - slave_2089126183
```

**Nota:** Después de los EOF, **NO** aparece "Waiting for EA connection" → Confirma que el listener murió.

---

## ✅ Solución Implementada

### 1. **SDK (`windows_pipe.go`)** - Nuevo método `DisconnectClient()`

**Archivo:** `/home/kor/go/src/github.com/xKoRx/echo/sdk/ipc/windows_pipe.go`

```go
// DisconnectClient cierra solo la conexión actual sin cerrar el listener.
//
// Esto permite que el servidor continúe aceptando nuevas conexiones después
// de que un cliente se desconecte. Útil para reconexión automática.
func (s *WindowsPipeServer) DisconnectClient() error {
	if s.currentConn != nil {
		err := s.currentConn.Close()
		s.currentConn = nil
		return err
	}
	return nil
}
```

**Interfaz actualizada:** `PipeServer` ahora incluye `DisconnectClient()`.

---

### 2. **Agent (`pipe_manager.go`)** - Loop externo para reconexión

**Archivo:** `/home/kor/go/src/github.com/xKoRx/echo/agent/internal/pipe_manager.go`

#### Cambios principales:

**a) Loop externo en `PipeHandler.Run()`:**

```go
func (h *PipeHandler) Run() error {
	// Loop externo: aceptar conexiones indefinidamente
	for {
		// 1. Esperar conexión del EA
		err := h.server.WaitForConnection(h.ctx)
		
		// 2. Notificar AccountConnected
		h.notifyAccountConnected()
		
		// 3. Procesar sesión (loop interno de lectura)
		sessionErr := h.handleSession()
		
		// 4. Notificar AccountDisconnected
		h.notifyAccountDisconnected("client_disconnected")
		
		// 5. i2b FIX CRÍTICO: Cerrar SOLO la conexión, NO el listener
		h.server.DisconnectClient()
		
		// 6. Log y volver al inicio (esperar nueva conexión)
		h.logInfo("Client disconnected, listener still open", ...)
	}
}
```

**b) Nueva función `handleSession()`:**
- Contiene el loop de lectura (antes estaba dentro de `Run()`)
- Retorna cuando hay EOF o error fatal
- Permite que `Run()` continue en el loop externo

**c) Backoff exponencial con jitter:**
- Si `WaitForConnection()` falla repetidamente
- Delay inicial: 100ms
- Delay máximo: 5s
- Jitter: ±25%

**d) Flush explícito en pong:**

```go
// Escribir pong
writer.WriteMessage(pongMsg)
// i2b FIX: Flush explícito para asegurar envío inmediato
writer.Flush()
```

---

### 3. **DLL (`echo_pipe.cpp`)** - WaitNamedPipe + GetPipeLastError

**Archivo:** `/home/kor/go/src/github.com/xKoRx/echo/pipe/echo_pipe.cpp`

#### a) `ConnectPipe()` - Agregar `WaitNamedPipeW()`

```cpp
extern "C" __declspec(dllexport) INT_PTR __stdcall ConnectPipe(const wchar_t* pipeName)
{
    // i2b FIX: Esperar a que el pipe esté disponible antes de conectar
    // Esto evita ERROR_PIPE_BUSY si el servidor no ha llamado a Accept() aún
    if (!WaitNamedPipeW(pipeName, 2000)) {
        return (INT_PTR)INVALID_HANDLE_VALUE;
    }
    
    HANDLE hPipe = CreateFileW(pipeName, ...);
    // ...
}
```

**Beneficio:** Evita `ERROR_PIPE_BUSY` (231) cuando el servidor está en transición entre conexiones.

#### b) `ClosePipe()` - Cancelar I/O pendiente

```cpp
extern "C" __declspec(dllexport) void __stdcall ClosePipe(INT_PTR handle)
{
    HANDLE hPipe = (HANDLE)handle;
    
    // i2b FIX: Cancelar I/O pendiente antes de cerrar
    CancelIoEx(hPipe, NULL);
    
    CloseHandle(hPipe);
}
```

**Beneficio:** Evita que el handle quede en estado inconsistente con operaciones pendientes.

#### c) Nueva función `GetPipeLastError()`

```cpp
extern "C" __declspec(dllexport) DWORD __stdcall GetPipeLastError()
{
    return GetLastError();
}
```

**Códigos de error Win32 comunes:**
- `2` - `ERROR_FILE_NOT_FOUND`: Pipe no existe
- `5` - `ERROR_ACCESS_DENIED`: Permisos
- `109` - `ERROR_BROKEN_PIPE`: Pipe cerrado por el otro lado
- `121` - `ERROR_SEM_TIMEOUT`: WaitNamedPipe timeout
- `231` - `ERROR_PIPE_BUSY`: Servidor no aceptando conexiones
- `233` - `ERROR_NO_PROCESS_ON_OTHER_END`: Servidor cerrado

---

### 4. **EAs (`master.mq4` y `slave.mq4`)** - Diagnóstico mejorado

**Archivos:**
- `/home/kor/go/src/github.com/xKoRx/echo/clients/mt4/master.mq4`
- `/home/kor/go/src/github.com/xKoRx/echo/clients/mt4/slave.mq4`

#### Cambios:

**a) Import de `GetPipeLastError()`:**

```mql4
#import "echo_pipe_x86.dll"
   int  ConnectPipe(string pipeName);
   int  WritePipeW(int handle, string data);
   int  ReadPipeLine(int handle, uchar &buffer[], int bufferSize);
   void ClosePipe(int handle);
   int  GetPipeLastError();  // i2b: Nuevo
#import
```

**b) Logging mejorado en reconexión:**

```mql4
else
{
   // i2b FIX: Obtener código de error Win32 para diagnóstico
   int winErr = GetPipeLastError();
   string errDesc = "";
   if(winErr == 2) errDesc = "FILE_NOT_FOUND";
   else if(winErr == 5) errDesc = "ACCESS_DENIED";
   else if(winErr == 109) errDesc = "BROKEN_PIPE";
   else if(winErr == 121) errDesc = "SEM_TIMEOUT";
   else if(winErr == 231) errDesc = "PIPE_BUSY";
   else if(winErr == 233) errDesc = "NO_PROCESS_ON_OTHER_END";
   
   Log("WARN","Pipe reconnection failed","pipe="+pipeName+", win32_error="+IntegerToString(winErr)+" ("+errDesc+")");
   // Backoff...
}
```

---

## 🧪 Cómo Probar

### Logs esperados (Agent)

**Conexión inicial:**
```
[INFO] Waiting for EA connection | pipe_name=echo_master_2089126182
[INFO] EA connected | pipe_name=echo_master_2089126182
[INFO] AccountConnected sent to Core (i2)
```

**Desconexión y reconexión:**
```
[INFO] Client disconnected (EOF detected) | pipe_name=echo_master_2089126182
[INFO] Session ended | reason=EOF
[INFO] AccountDisconnected sent to Core (i2)
[INFO] Client disconnected, listener still open | pipe_name=echo_master_2089126182
[INFO] Waiting for EA connection | pipe_name=echo_master_2089126182  ← ✅ CLAVE
[INFO] EA connected | pipe_name=echo_master_2089126182  ← ✅ RECONECTÓ
```

**Si NO aparece "Waiting for EA connection" después del EOF → listener cerrado (bug no resuelto).**

### Logs esperados (EAs)

**Reconexión exitosa:**
```
[INFO] 123456 | Attempting pipe reconnection | delay_ms=100
[INFO] 123460 | Pipe reconnected | pipe=\\.\pipe\echo_master_2089126182
[INFO] 123465 | Handshake sent
```

**Reconexión fallida con diagnóstico:**
```
[WARN] 123456 | Attempting pipe reconnection | delay_ms=200
[WARN] 123460 | Pipe reconnection failed | pipe=\\.\pipe\echo_master_2089126182, win32_error=231 (PIPE_BUSY)
[WARN] 123660 | Attempting pipe reconnection | delay_ms=400
```

**Códigos comunes durante reconexión:**
- `ERROR_FILE_NOT_FOUND (2)` → Agent no ha creado el pipe aún (normal en startup)
- `ERROR_PIPE_BUSY (231)` → Agent en transición (debería desaparecer con `WaitNamedPipe`)
- `ERROR_SEM_TIMEOUT (121)` → `WaitNamedPipe` timeout (2s)

---

## 📋 Checklist de Validación

### Funcionamiento Correcto:

- [ ] **Logs del Agent:** "Waiting for EA connection" aparece **DESPUÉS** de cada EOF
- [ ] **Logs del Agent:** "EA connected" aparece después de cada "Waiting for EA connection"
- [ ] **Logs de EAs:** Reconexión exitosa con código 0 (sin error)
- [ ] **Sin `ERROR_PIPE_BUSY` persistente:** Máximo 1-2 intentos, luego conecta
- [ ] **Estabilidad >1 hora:** Sistema funciona sin desconexiones masivas

### Señales de Problemas:

- [ ] ❌ **"Waiting for EA connection" NO aparece** después del EOF → listener cerrado
- [ ] ❌ **`ERROR_PIPE_BUSY (231)` persistente** (>10 intentos) → `WaitNamedPipe` no funciona
- [ ] ❌ **Desconexiones masivas cada ~30 min** → problema de pong timeout (aumentar `PONG_TIMEOUT_MS`)

---

## 🔧 Configuración Recomendada (EAs)

### Si hay desconexiones frecuentes:

```mql4
#define PING_INTERVAL_MS      5000   // ✅ OK (cada 5s)
#define PONG_TIMEOUT_MS       5000   // ⚠️  Aumentar de 3000 a 5000
#define MAX_SILENCE_MS        15000  // ✅ OK (watchdog 15s)
#define RECONNECT_MIN_MS      100    // ✅ OK (inicio rápido)
#define RECONNECT_MAX_MS      5000   // ✅ OK (máximo 5s)
#define RECONNECT_JITTER_MS   250    // ✅ OK (desincroniza EAs)
```

**Rationale:**
- `PONG_TIMEOUT_MS=5000`: Da margen para GC pauses o I/O lento
- `RECONNECT_JITTER_MS=250`: Evita tormenta de reconexión

---

## 🚀 Compilación y Despliegue

### 1. Compilar DLL

```bash
cd /home/kor/go/src/github.com/xKoRx/echo/pipe

# x86 (MT4)
i686-w64-mingw32-g++ -shared -o ../bin/echo_pipe_x86.dll echo_pipe.cpp \
  -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias

# x64 (MT5 - opcional)
x86_64-w64-mingw32-g++ -shared -o ../bin/echo_pipe_x64.dll echo_pipe.cpp \
  -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias
```

### 2. Compilar Agent

```bash
cd /home/kor/go/src/github.com/xKoRx/echo/agent
GOOS=windows GOARCH=amd64 go build -o ../bin/echo-agent.exe ./cmd/agent
```

### 3. Compilar EAs (MetaEditor)

**Manual:**
1. Abrir MetaEditor
2. Compilar `master.mq4` → `master.ex4`
3. Compilar `slave.mq4` → `slave.ex4`
4. Copiar `.ex4` a `echo/bin/`

**Nota:** Los EAs MQL4 deben compilarse en MetaEditor (no hay compilador CLI confiable).

---

## 📦 Archivos Modificados

### SDK (Go):
- `sdk/ipc/pipe.go` - Agregar `DisconnectClient()` a interfaz
- `sdk/ipc/windows_pipe.go` - Implementar `DisconnectClient()`

### Agent (Go):
- `agent/internal/pipe_manager.go` - Loop externo + `handleSession()` + flush de pong

### DLL (C++):
- `pipe/echo_pipe.cpp` - `WaitNamedPipeW()`, `CancelIoEx()`, `GetPipeLastError()`

### EAs (MQL4):
- `clients/mt4/master.mq4` - Import `GetPipeLastError()` + logging mejorado
- `clients/mt4/slave.mq4` - Import `GetPipeLastError()` + logging mejorado

---

## 🎯 Resultado Esperado

- ✅ **Reconexión automática:** EAs reconectan en <5 segundos después de EOF
- ✅ **Sin `PIPE_BUSY`:** `WaitNamedPipe` espera hasta que el listener esté listo
- ✅ **Diagnóstico claro:** Logs muestran códigos de error Win32 específicos
- ✅ **Estabilidad:** Sistema funciona >1 hora sin desconexiones masivas
- ✅ **Listener persistente:** Agent acepta conexiones indefinidamente

---

## 📞 Próximos Pasos

1. **Compilar y desplegar** DLL + Agent + EAs
2. **Probar reconexión:** Cerrar MT4 de un EA y ver si reconecta
3. **Stress test:** Dejar correr >1 hora y monitorear logs
4. **Si falla:** Revisar códigos de error Win32 en logs de EAs

---

**Fin del documento i2b** 🎉
