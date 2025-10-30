# Implementación i2b: Ping/Pong y Reconexión Robusta

**Fecha:** 2025-10-29  
**Versión:** i2b  
**Estado:** ✅ Implementado y compilado

---

## 📋 Resumen Ejecutivo

Se implementó el sistema completo de **ping/pong** y **reconexión robusta** según la guía `echo-agent-ea-integration-guide.md`:

- ✅ **Master EA**: ping/pong, reconexión con backoff exponencial, lectura no bloqueante
- ✅ **Slave EA**: ping/pong, watchdog de silencio, wrapper robusto de escritura, higiene de trading
- ✅ **Agent (Go)**: handler de ping/pong, respuesta automática con `pong`
- ✅ **Pipe DLL**: ya tenía `ReadPipeLine` implementado (sin cambios necesarios)
- ⚠️ **Core**: sin cambios (ping/pong es local EA↔Agent)

---

## 🎯 Componentes Modificados

### 1. Master EA (`clients/mt4/master.mq4`)

#### Cambios Implementados:
- **Lectura no bloqueante del pipe**
  - `ReadPipeLineString()` con buffer de 8KB
  - Cupo de lectura por tick: `MAX_READ_PER_TICK = 64`

- **Ping/Pong**
  - `SendPing()`: envía ping cada 5 segundos
  - `TryHandlePong()`: procesa respuesta del Agent
  - Correlación por UUID, un solo ping en vuelo
  - Timeout de pong: 3 segundos → reconexión

- **Reconexión robusta**
  - `SafeReconnect()` con backoff exponencial (100ms → 5000ms)
  - Jitter aleatorio (+250ms) para evitar avalanchas
  - **SIN `Sleep()` bloqueante** - basado en `GetTickCount()`
  - Handshake tras cada reconexión exitosa

- **Watchdog de silencio**
  - `MAX_SILENCE_MS = 15000`
  - Desconexión si Agent no responde

- **Wrapper de escritura robusto**
  - `PipeWriteLn()` con manejo de errores
  - Cierre automático de handle si fallo
  - Marca desconexión con `PipeMarkDisconnected()`

#### Parámetros Configurados:
```mql4
PING_INTERVAL_MS      = 5000
PONG_TIMEOUT_MS       = 3000
MAX_SILENCE_MS        = 15000
RECONNECT_MIN_MS      = 100
RECONNECT_MAX_MS      = 5000
RECONNECT_JITTER_MS   = 250
MAX_READ_PER_TICK     = 64
READ_BUFFER_BYTES     = 8192
```

---

### 2. Slave EA (`clients/mt4/slave.mq4`)

#### Cambios Implementados:
- **Ping/Pong activo**
  - Slave inicia pings (no solo responde)
  - Funciones idénticas a Master para simetría
  - Watchdog de silencio implementado

- **Wrapper robusto de escritura**
  - `PipeWriteLn()` usado en todas las respuestas
  - Cierre de handle en fallo de escritura
  - Tracking de `g_LastTxMs`

- **Higiene de trading**
  - `ClampLot(lot, symbol)`: ajusta volumen a `MINLOT/MAXLOT/LOTSTEP`
  - `IsSymbolValid(symbol)`: valida disponibilidad de símbolo
  - Validaciones antes de ejecutar órdenes
  - Logging mejorado con `MapErrorCode()`

- **Reconexión robusta**
  - Backoff exponencial + jitter
  - Sin `Sleep()` bloqueante

- **OnTimer mejorado**
  - Estructura completa:
    1. Verificar conexión → reconectar si es necesario
    2. Lectura limitada (`MAX_READ_PER_TICK`)
    3. Verificar timeout de pong → reconectar
    4. Watchdog de silencio → reconectar
    5. Enviar ping si es debido

#### Mejoras en HandleExecuteOrder:
- Validación de símbolo antes de ejecutar
- `ClampLot()` aplicado a `lot_size`
- Logging con descripción de error (`MapErrorCode`)
- `trade_id` incluido en comentario de la orden

---

### 3. Agent (Go) - `agent/internal/pipe_manager.go`

#### Cambios Implementados:
- **Handler de ping**
  - Nueva función `handlePing(msgMap)` en `PipeHandler`
  - Extrae `id` y `timestamp_ms` del ping
  - Construye respuesta `pong`:
    ```json
    {
      "type": "pong",
      "id": "<ping_id>",
      "timestamp_ms": <now>,
      "echo_ms": <timestamp_from_ping>
    }
    ```
  - Calcula RTT para logging

- **Integración en dispatchers**
  - `handleMasterMessage`: añadido case "ping"
  - `handleSlaveMessage`: añadido case "ping"

- **Logging mejorado**
  - Log de ping recibido con `ping_id` y `echo_ms`
  - Log de pong enviado con RTT calculado
  - Warnings si ping sin `id`

#### Código clave:
```go
func (h *PipeHandler) handlePing(msgMap map[string]interface{}) error {
    pingID := utils.ExtractString(msgMap, "id")
    echoMs := utils.ExtractInt64(msgMap, "timestamp_ms")
    
    pongMsg := map[string]interface{}{
        "type":         "pong",
        "id":           pingID,
        "timestamp_ms": utils.NowUnixMilli(),
        "echo_ms":      echoMs,
    }
    
    writer := ipc.NewJSONWriter(h.server)
    return writer.WriteMessage(pongMsg)
}
```

---

### 4. Pipe DLL (`pipe/echo_pipe.cpp`)

#### Estado:
✅ **Sin cambios necesarios**

La DLL ya tenía implementado:
- `ReadPipeLine()`: lectura no bloqueante con `PeekNamedPipe`
- `WritePipeW()`: escritura con conversión UTF-16 → UTF-8
- Manejo robusto de handles

**Versión actual:** 1.1.0 (85KB)

---

### 5. Core

#### Estado:
✅ **Sin cambios necesarios**

El ping/pong implementado es **completamente local** entre EA y Agent para:
- Detectar desconexiones de EAs
- Mantener liveness de la conexión Named Pipe
- Evitar timeouts silenciosos

El Core no participa en este mecanismo.

---

## 🚀 Compilados Generados

### Ubicación: `/home/kor/go/src/github.com/xKoRx/echo/bin/`

| Archivo | Plataforma | Tamaño | Estado |
|---------|-----------|--------|--------|
| `echo-agent-windows-amd64.exe` | Windows x64 | 19 MB | ✅ Compilado |
| `echo_pipe_x86.dll` | Windows x86 | 85 KB | ✅ Existente (sin cambios) |

### Comandos de compilación:

```bash
# Agent para Windows x64
cd /home/kor/go/src/github.com/xKoRx/echo/agent
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags="-s -w" \
  -o bin/echo-agent.exe \
  ./cmd/echo-agent

# Copiar a echo/bin
cp bin/echo-agent.exe /home/kor/go/src/github.com/xKoRx/echo/bin/echo-agent-windows-amd64.exe
```

**Nota:** Pipe DLL no requirió recompilación (sin cambios en código C++).

---

## 🔍 Verificación de Implementación

### Master EA - Checklist
- [x] `ReadPipeLine` importado en DLL
- [x] `ReadPipeLineString()` implementado
- [x] `SendPing()` con UUID y timestamp
- [x] `TryHandlePong()` con correlación por ID
- [x] `PongTimedOut()` detecta timeout
- [x] `SilenceExceeded()` watchdog implementado
- [x] `SafeReconnect()` sin `Sleep()`
- [x] Backoff exponencial + jitter
- [x] `OnTimer()` con lectura limitada
- [x] Handshake tras reconexión

### Slave EA - Checklist
- [x] Ping/pong completo (mismas funciones que Master)
- [x] `PipeWriteLn()` usado en todos los envíos
- [x] `ClampLot()` implementado y usado
- [x] `IsSymbolValid()` implementado y usado
- [x] Validaciones en `HandleExecuteOrder`
- [x] Watchdog de silencio implementado
- [x] `OnTimer()` estructurado según guía

### Agent - Checklist
- [x] `handlePing()` implementado
- [x] Respuesta `pong` con estructura correcta
- [x] Integrado en `handleMasterMessage`
- [x] Integrado en `handleSlaveMessage`
- [x] Logging de RTT

---

## 📊 Flujo de Ping/Pong

```
Master/Slave EA                    Agent (Go)
     |                                |
     |--- ping (id, timestamp_ms) --->|
     |                                | [handlePing]
     |                                | [calcula RTT]
     |<-- pong (id, echo_ms) ---------|
     |                                |
     | [TryHandlePong]                |
     | [verifica ID]                  |
     | [actualiza g_LastRxMs]         |
     |                                |
```

### Timeouts y Reconexión

```
EA                           Condición                 Acción
|                                                      |
|--- ping enviado --------------------------->         |
|                         [espera 3s]                  |
|                    [no llega pong] --------> PipeMarkDisconnected()
|                                              SafeReconnect()
|                         [delay con backoff + jitter] |
|--- intento de reconexión ----------------------->    |
|<-- éxito -------------------------------     Reset backoff
|--- handshake ------------------------------------>   |
```

---

## ⚙️ Parámetros de Configuración

### Valores Recomendados (implementados)

```mql4
// Liveness
PING_INTERVAL_MS      = 5000   // Ping cada 5 segundos
PONG_TIMEOUT_MS       = 3000   // Espera pong 3 segundos
MAX_SILENCE_MS        = 15000  // Desconecta si no hay datos por 15s

// Reconexión
RECONNECT_MIN_MS      = 100    // Delay inicial
RECONNECT_MAX_MS      = 5000   // Delay máximo
RECONNECT_JITTER_MS   = 250    // Jitter aleatorio

// Performance
MAX_READ_PER_TICK     = 64     // Máximo de lecturas por tick
READ_BUFFER_BYTES     = 8192   // Buffer de lectura (8KB)
```

---

## 🎯 Beneficios de la Implementación

### 1. Detección Rápida de Desconexiones
- Timeout de pong: **3 segundos**
- Watchdog de silencio: **15 segundos**
- Sin esperas indefinidas

### 2. Reconexión Robusta
- Backoff exponencial evita saturación
- Jitter evita avalanchas coordinadas
- Sin bloqueo del hilo de MT4/MT5

### 3. Sin Hambruna
- Lectura limitada por tick (`MAX_READ_PER_TICK`)
- EA sigue respondiendo a eventos de trading
- No congela la UI de MT4/MT5

### 4. Higiene de Trading (Slave EA)
- Validación de símbolos
- Ajuste de volumen a especificaciones del broker
- Logging mejorado de errores

---

## 🔧 Próximos Pasos (Opcional)

### Mejoras Futuras (fuera de i2b):
- [ ] Métricas de reconexión en Agent
- [ ] Dashboard de liveness por EA
- [ ] Alertas automáticas si reconexión > N veces
- [ ] Buffer circular más eficiente en dedupe (Slave EA)

---

## ✅ Estado Final

| Componente | Estado | Archivo | Compilado |
|------------|--------|---------|-----------|
| Master EA | ✅ Implementado | `clients/mt4/master.mq4` | Manual (MT4 Editor) |
| Slave EA | ✅ Implementado | `clients/mt4/slave.mq4` | Manual (MT4 Editor) |
| Agent | ✅ Implementado | `agent/internal/pipe_manager.go` | ✅ `bin/echo-agent-windows-amd64.exe` |
| Pipe DLL | ✅ Sin cambios | `pipe/echo_pipe.cpp` | ✅ `bin/echo_pipe_x86.dll` (existente) |
| Core | ⚠️ Sin cambios | - | ⚠️ No requerido |

---

## 📝 Notas de Despliegue

### Requisitos:
1. **Agent:** `echo-agent-windows-amd64.exe` en servidor Windows
2. **Pipe DLL:** `echo_pipe_x86.dll` en `C:\Program Files\MetaTrader 4\MQL4\Libraries\`
3. **Master EA:** Compilar `master.mq4` en MetaEditor (MT4/MT5)
4. **Slave EA:** Compilar `slave.mq4` en MetaEditor (MT4/MT5) con `JAson.mqh`

### Configuración ETCD:
- Agent debe tener configuradas las cuentas master y slave
- Pipe prefix: `\\.\pipe\echo_`

### Testing:
1. Iniciar Agent
2. Adjuntar Master EA a gráfico
3. Adjuntar Slave EA a gráfico
4. Verificar en logs:
   - "Handshake received"
   - "Ping sent" / "Pong received"
   - RTT calculado

---

**Fin del documento - Implementación i2b completada** ✅

