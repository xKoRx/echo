# ADR-003: Named Pipes para IPC Agent ↔ EAs

## Estado
**Aprobado** - 2025-10-23

## Contexto

Los **Expert Advisors** (EAs) en MT4/MT5 corren dentro del terminal de trading en Windows y necesitan comunicarse con el **Agent** (servicio Go).

Opciones consideradas:

1. **Named Pipes** (Windows IPC)
2. **TCP Sockets** (localhost)
3. **HTTP REST** (localhost)
4. **Archivos compartidos** (file-based)

## Decisión

Usaremos **Named Pipes** (Windows IPC).

### Nomenclatura

```
\\.\pipe\echo_master_{EA_ID}
\\.\pipe\echo_slave_{EA_ID}
```

Ejemplos:
- `\\.\pipe\echo_master_001`
- `\\.\pipe\echo_slave_MT5_002`

### Protocolo

Mensajes **JSON** delimitados por línea nueva (`\n`):

```json
{"type":"trade_intent","trade_id":"uuid","symbol":"XAUUSD",...}\n
```

## Consecuencias

### Positivas
- ✅ **Nativo Windows**: Sin drivers adicionales
- ✅ **Baja latencia**: IPC kernel (~5-10ms)
- ✅ **Seguro**: No expuesto en red
- ✅ **MQL friendly**: Librería estándar MT4/MT5 (`kernel32.dll`)
- ✅ **Múltiples clientes**: Un pipe por EA, sin colisiones

### Negativas
- ⚠️ **Windows-only**: No funciona en Linux/Mac (no crítico, EAs solo en Windows)
- ⚠️ **Debugging complejo**: No podemos usar curl/Postman
- ⚠️ **Serialización manual**: JSON en MQL4/MQL5 (no nativo)

### Alternativas Descartadas

**TCP Sockets (localhost)**:
- ❌ Más lento que Named Pipes
- ❌ Requiere gestión de puertos
- ❌ Riesgo de firewall/antivirus

**HTTP REST**:
- ❌ Overhead HTTP
- ❌ Servidor HTTP en Agent (complejidad)
- ❌ Latencia mayor

**Archivos compartidos**:
- ❌ Latencia muy alta (I/O disco)
- ❌ File locking issues
- ❌ Sin streaming

## Implementación

### Agent (Go Server)

```go
import "github.com/xKoRx/echo/sdk/ipc"

listener, err := ipc.NewPipeServer(`\\.\pipe\echo_master_001`)
defer listener.Close()

for {
    conn, err := listener.Accept()
    go handleConnection(conn)
}
```

### Master EA (MQL4)

```mql4
#include <EchoIPC.mqh>

int OnInit() {
    if (!EchoConnect("\\.\pipe\echo_master_001")) {
        Print("Failed to connect to agent");
        return INIT_FAILED;
    }
    return INIT_SUCCEEDED;
}

void OnTick() {
    string json = BuildTradeIntent(...);
    EchoSend(json);
}
```

### Slave EA (MQL4)

```mql4
void OnTimer() {
    string json = EchoReceive();
    if (json != "") {
        ProcessCommand(json);
    }
}
```

## Librería MQL (EchoIPC.mqh)

Wrapper sobre `kernel32.dll`:

```mql4
int CreateFileW(...);
int ReadFile(...);
int WriteFile(...);
int CloseHandle(...);
```

## Formato JSON

Mínimo necesario:

```json
// Master → Agent
{
  "type": "trade_intent",
  "trade_id": "uuid-v7",
  "timestamp_ms": 1234567890,
  "symbol": "XAUUSD",
  "side": "BUY",
  "lot_size": 0.10,
  "price": 1950.25,
  "magic_number": 12345,
  "ticket": 987654
}

// Agent → Slave
{
  "type": "execute_order",
  "command_id": "uuid",
  "trade_id": "uuid-v7",
  "symbol": "XAUUSD",
  "side": "BUY",
  "lot_size": 0.05,
  "magic_number": 12345
}

// Slave → Agent
{
  "type": "execution_result",
  "command_id": "uuid",
  "success": true,
  "ticket": 123456,
  "executed_price": 1950.30
}
```

## Fallback

Si Named Pipes presentan problemas en producción:
- **Plan B**: TCP sockets localhost con puertos dinámicos
- **Plan C**: HTTP REST con localhost:port

## Referencias
- [Windows Named Pipes](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipes)
- [MQL4 DLL Calls](https://docs.mql4.com/basis/function/call_dll)
- [RFC-001](../RFC-001-architecture.md#21-diagrama-general)

