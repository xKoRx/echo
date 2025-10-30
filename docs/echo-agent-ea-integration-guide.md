# Guía de Implementación: Integración Robusta Agent ↔ MasterEA / SlaveEA
**Fecha:** 2025-10-29  
**Estado:** Lista para implementar  
**Ámbito:** MT4/MT5 + Agent (Windows Named Pipe)

---

## Objetivo
Garantizar reconexión automática, liveness comprobable y entrega idempotente de mensajes entre el Agent y las EAs Master/Slave. El diseño evita bloqueos en eventos MT4/MT5 y asegura que el Agent acepte reconexiones ilimitadas.

---

## Resumen ejecutivo
- **Framing:** JSON delimitado por `\n` en UTF‑8.
- **Liveness:** `ping`/`pong` con correlación por `id` y `PONG_TIMEOUT_MS`.
- **Reconexión:** bucles externos con backoff exponencial + jitter, cierre explícito de handles.
- **Idempotencia:** `command_id` y de‑dupe O(1) en EAs; “última escritura gana” en el Core.
- **No bloqueo:** sin `Sleep()` en eventos; lecturas con cupo por tick.
- **Observabilidad:** logs y métricas de reconexión, latencias y errores de I/O.

---

## Especificación de protocolo
### Mensajes
Todos los mensajes son objetos JSON con campos mínimos: `type`, `timestamp_ms`, `payload`.
- **handshake**
  - `type:"handshake"`
  - `payload:{ client_id, account_id, broker, role:"master|slave", symbol, version }`
  - Enviado **después de cada conexión** exitosa.
- **ping / pong**
  - `ping`: `{"type":"ping","id":"<uuid>","timestamp_ms":<t>,"role":"master|slave"}`
  - `pong`: `{"type":"pong","id":"<uuid>","timestamp_ms":<t_agent>,"echo_ms":<t_ping>}`
  - Correlación por `id`. Un solo ping en vuelo por lado.
- **Negocio Master→Agent**
  - `trade_intent`: apertura detectada en Master.
  - `trade_close`: cierre detectado en Master.
- **Negocio Agent→Slave**
  - `execute_order`, `close_order`.
- **Acks Slave→Agent**
  - `execution_result`, `close_result` con `command_id` y timestamps t0..t7.

### Reglas
- Cada línea = un mensaje completo. Terminar con `\n`.
- Longitud máxima aconsejada por línea: ≤ 64 KiB.
- Campos de tiempo en milisegundos desde arranque o epoch, consistente por flujo.
- Toda reconexión se considera **nuevo** ciclo: reenviar `handshake`.

---

## Named Pipe: controles y parámetros
### Server (Agent)
- **Modo:** full‑duplex, mensaje, bloqueo seguro.
- **Flags recomendados en creación:**
  - `PIPE_ACCESS_DUPLEX`
  - `PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT`
- **Instancias:**
  - Secuencial: una instancia por pipe lógico, reconexiones **secuenciales**.
  - Paralelo opcional: varias instancias si se desea concurrencia (no requerido aquí).
- **Aceptación:** `Accept / ConnectNamedPipe` en bucle externo.
- **Cierre:**
  - Al detectar `EOF` o error de I/O: cerrar `currentConn`, mantener `listener`.
  - Asegurar `currentConn=nil` tras `Close` para evitar fugas.
- **Buffers:** 8–64 KiB por dirección. Ajustar si hay ráfagas.
- **Flush:** forzar `FlushFileBuffers` o `writer.Flush()` tras `pong` y respuestas críticas.
- **Codificación:** UTF‑8. No mezclar UTF‑16.
- **Límites:** aplicar tamaño máximo de línea; descartar mensajes parciales con log de error.
- **Seguridad:** DACL que permita al usuario de MT4/MT5 conectar. Evitar permisos amplios.

### Client (EAs)
- **Apertura:** `GENERIC_READ|GENERIC_WRITE`, modo mensaje.
- **Lectura:** no bloqueante desde la perspectiva de la EA.
  - Implementar `ReadPipeLine` con `PeekNamedPipe` o tiempo de espera corto.
- **Vida del handle:** cerrar **siempre** ante error de escritura, silencio prolongado o timeout de `pong`.
- **Jitter:** introducir aleatoriedad pequeña en backoff para evitar avalanchas coordinadas.

### Framing y parsing
- Corte estricto por `\n`. Strip de `\r\n`.
- Validar `type`. Ignorar silenciosamente tipos desconocidos.
- Evitar parsers pesados en `OnTimer`. JSON simple o librería ligera en Slave; librería robusta en Agent.

---

## Agent (Go): cambios y blueprint
### 1) Bucle externo de reconexión
- Envolver `WaitForConnection()` + loop de lectura en `for{}`.
- Al salir por `EOF` o error:
  - `notifyAccountDisconnected(reason)`
  - `server.Close()` de la conexión actual
  - backoff exponencial con jitter `[100ms..5s]`
  - `continue` para volver a `WaitForConnection()`

### 2) Ping → Pong
- En el dispatcher:
  ```go
  case "ping":
      writer.WriteLine(fmt.Sprintf(`{"type":"pong","id":"%s","timestamp_ms":%d,"echo_ms":%d}`,
           msg.Payload.ID, nowMS(), msg.TimestampMS))
      writer.Flush()
  ```

### 3) Lectura
- Respetar límites de tamaño por línea. Rechazar >64 KiB.
- Si `Deserialize` falla: log de nivel WARN y continuar.

### 4) Notificaciones
- `notifyAccountConnected()` al conectar.
- `notifyAccountDisconnected()` al cerrar/EOF.

### 5) Métricas
- `echo.pipe.reconnection.attempts{pipe,role,account}`
- `echo.pipe.reconnection.success{pipe,role,account}`
- `echo.pipe.connection.duration_seconds{pipe,role}`
- `echo.pipe.ping.rtt_ms{pipe,role}`

### 6) Logging
- Intento de reconexión con delay actual.
- Éxito/fallo y motivo de desconexión.
- Mensajes descartados por tamaño o JSON inválido.

---

## MasterEA (MT4/MT5): guía de implementación
### Objetivo
Detectar aperturas/cierres locales, reportarlos al Agent y mantener liveness con `ping/pong`. Sin bloquear la UI ni el hilo de trading.

### Cambios clave
1. **Lectura del pipe.** Añadir `ReadPipeLine(handle, buf, n)` en la DLL y wrapper `ReadPipeLineString` en la EA.
2. **Ping/pong.**
   - `PING_INTERVAL_MS=5000`, `PONG_TIMEOUT_MS=3000`.
   - Un ping en vuelo. Guardar `id` y `t_sent`. Borrar al recibir `pong` con mismo `id`.
   - Si timeout: cerrar handle y reconectar con backoff y jitter.
3. **Backoff no bloqueante.**
   - Quitar `Sleep()`.
   - `AttemptReconnect()` basado en `GetTickCount()` y `g_ReconnectDelayMs = min(5s, 2x + jitter)`.
4. **Cupo de lectura por tick.**
   - `MAX_READ_PER_TICK = 64` para evitar hambruna.
5. **Handshake tras cada conexión.**
   - Reenviar siempre.
6. **Tracking de tickets.**
   - Arrays paralelos `ticket[]` ↔ `trade_id[]`.
   - No generar `trade_id` nuevo en `trade_close`.
7. **Logs.**
   - `FileFlush` tras `FileWrite` si `LogToFile`.

### Pseudo‑código de `OnTimer`
```mq4
void OnTimer(){
  EnsurePipeConnected();
  if(IsPipeConnected()){
    for(int i=0;i<MAX_READ_PER_TICK;i++){
      string line = ReadPipeLineString(g_PipeHandle);
      if(line=="") break;
      if(line contains "\"type\":\"pong\"") resolvePing(line);
      else handleOther(line); // opcional
    }
    if(pingDue()) sendPing();
    if(pongTimedOut()) AttemptReconnect();
  }
  CheckForNewOpenOrders();
  CheckForClosedOrders();
}
```

### Consideraciones MT5
- Opción de **Service** para salud en paralelo. La EA de trading se separa del I/O.
- Comunicación EA↔Service via `GlobalVariable*` o archivos comunes si se necesita.

---

## SlaveEA (MT4/MT5): guía de implementación
### Objetivo
Ejecutar órdenes provenientes del Agent y confirmar resultados. Mantener liveness con `ping/pong` y watchdog de silencio como última línea.

### Cambios clave
1. **Estado de salud.**
   - `g_LastRxMs`, `g_LastTxMs`, `g_PingIdPending`, `g_PingSentMs`.
2. **Wrapper de escritura.**
   - `PipeWriteLn()` que cierra y marca desconexión si `WritePipeW` falla.
3. **Heartbeat y watchdog.**
   - `SendPing()` cada 5 s.
   - Si no llega `pong` en 3 s → reconectar.
   - Watchdog adicional: `MAX_SILENCE_MS=15000` por si el Agent no responde pongs.
4. **Lectura limitada por tick.**
   - `MAX_READ_PER_TICK=64`.
5. **Idempotencia O(1).**
   - Buffer circular para `command_id` con tamaño fijo.
6. **Higiene de trading.**
   - `ClampLot()` a `MINLOT/MAXLOT/LOTSTEP`.
   - Validar símbolo antes de ejecutar.
   - Mapear errores a texto (`ERR_*`).

### Pseudo‑código de `OnTimer`
```mq4
void OnTimer(){
  if(!PipeConnected()){ SafeReconnect(); return; }
  for(int i=0;i<MAX_READ_PER_TICK;i++){
    string line = ReadPipeLineString(g_Pipe);
    if(line=="") break;
    if(!TryHandlePong(line)) HandleCommand(line);
  }
  if(heartbeatDue()) SendPing();
  if(silenceExceeded()) { PipeMarkDisconnected("rx_silence"); SafeReconnect(); }
}
```

---

## Manejo de errores y estados
### Estados de la conexión (EAs)
- `DISCONNECTED` → `CONNECTING` → `CONNECTED` → `DEGRADED` (opcional) → `DISCONNECTED`.
- Transiciones disparadas por: éxito en `ConnectPipe`, fallo en `WritePipeW`, timeout de `pong`, watchdog de silencio.

### Errores típicos y acción
- **`WritePipeW<=0`**: cerrar handle y reconectar.
- **`ReadPipeLine==0` repetido**: si supera `MAX_SILENCE_MS`, reconectar.
- **JSON inválido**: log y descartar.
- **Símbolo inválido**: responder con error y no ejecutar.

---

## Valores recomendados (constantes)
```
PING_INTERVAL_MS     = 5000
PONG_TIMEOUT_MS      = 3000
MAX_SILENCE_MS       = 15000
RECONNECT_MIN_MS      = 100
RECONNECT_MAX_MS      = 5000
RECONNECT_JITTER_MS    = 250
MAX_READ_PER_TICK       = 64
READ_BUFFER_BYTES    = 8192..65536
SLIPPAGE_POINTS            = 3
```

---

## Pruebas recomendadas
### Funcionales
- Desconexión del Agent durante tráfico.
- Reconexión rápida del EA (<100 ms) y lenta (>5 s).
- Ping perdido, pong presente → no reconectar.
- Pong perdido → reconectar.
- Mensaje >64 KiB → descartado con log.
- JSON inválido → descartado con log.

### Performance
- 1000 mensajes por minuto por 10 minutos. Sin hambruna en `OnTimer`.
- Latencia `ping` p50/p95/p99.

### Resiliencia
- Ciclos de abrir/cerrar Agent 100 veces. Sin fugas de handle.
- Cortes durante `OrderSend` y `OrderClose` con reporte coherente.

---

## Checklist de despliegue
- [ ] Agent con bucle de reconexión y handler `pong`.
- [ ] DLL con `ReadPipeLine` no bloqueante o con timeout breve.
- [ ] MasterEA con lectura, ping/pong y backoff sin `Sleep()`.
- [ ] SlaveEA con ping/pong, watchdog y de‑dupe O(1).
- [ ] Logs rotativos y `FileFlush` en modo DEBUG.
- [ ] Métricas y alertas de reconexión y timeout.

---

## Anexos
### A. Esquemas JSON
```json
// ping
{"type":"ping","id":"<uuid>","timestamp_ms":123456,"role":"master"}

// pong
{"type":"pong","id":"<uuid>","timestamp_ms":123460,"echo_ms":123456}

// handshake
{"type":"handshake","timestamp_ms":123470,
 "payload":{"client_id":"master_123","account_id":"123","broker":"ABC","role":"master","symbol":"XAUUSD","version":"0.1.0"}}

// execute_order (Agent→Slave)
{"type":"execute_order","timestamp_ms":123500,
 "payload":{"command_id":"c-001","trade_id":"t-001","symbol":"XAUUSD","order_side":"BUY","lot_size":0.10,"magic_number":111111,
            "timestamps":{"t0_master_ea_ms":1,"t1_agent_recv_ms":2,"t2_core_recv_ms":3,"t3_core_send_ms":4,"t4_agent_recv_ms":5}}}

// execution_result (Slave→Agent)
{"type":"execution_result","timestamp_ms":123520,
 "payload":{"command_id":"c-001","trade_id":"t-001","client_id":"slave_456","success":true,"ticket":12345,
            "error_code":"ERR_NO_ERROR","error_message":"","executed_price":2400.10,
            "timestamps":{"t0_master_ea_ms":1,"t1_agent_recv_ms":2,"t2_core_recv_ms":3,"t3_core_send_ms":4,"t4_agent_recv_ms":5,"t5_slave_ea_recv_ms":6,"t6_order_send_ms":7,"t7_order_filled_ms":8}}}
```

### B. Mapa de decisiones
- Si el Agent cae: EAs reconectan con backoff + jitter.
- Si el Agent responde `pong`: no reconectar.
- Si el Agent no responde `pong` y hay silencio: reciclar handle.
- Tras reconectar: reenviar `handshake`, resetear backoff.

---

## Conclusión
El sistema se vuelve tolerante a fallos transitorios y recupera sesión sin intervención manual. El Agent escucha reconexiones indefinidas. Las EAs mantienen salud por aplicación y protegen su hilo con operaciones no bloqueantes.
