---
title: "Validación Fase 2 — Slave EA Mínimo (i0)"
rfc: "RFC-002"
version: "1.0"
date: "2025-10-25"
status: "✅ APROBADO"
reviewer: "Aranea Labs - QA Team"
---

# Validación de Fase 2: Slave EA Mínimo (Iteración 0)

## 📋 Resumen Ejecutivo

**Estado**: ✅ **APROBADO**

El Slave EA implementado en `/clients/mt4/slave.mq4` cumple **completamente** con los requisitos de la Fase 2 del RFC-002 (Iteración 0).

**Archivo validado**: `echo/clients/mt4/slave.mq4`  
**Líneas de código**: 484  
**Lenguaje**: MQL4  
**Versión**: 0.1.0

---

## ✅ Criterios de Aceptación de Fase 2 (según RFC-002)

### Entregables

| # | Entregable | Estado | Notas |
|---|------------|--------|-------|
| 1 | EA MQL4 que conecta a Named Pipe del Agent | ✅ COMPLETO | Líneas 197-210 |
| 2 | Envía mensaje de handshake con metadata | ✅ COMPLETO | Líneas 177-195 |
| 3 | Recibe comando ExecuteOrder | ✅ COMPLETO | Líneas 308-369 |
| 4 | Llama OrderSend y reporta ExecutionResult | ✅ COMPLETO | Línea 353 + 367-368 |
| 5 | Envía telemetría básica (timestamps) | ✅ COMPLETO | Timestamps t0-t7 completos |

### Criterios Técnicos

| # | Criterio | Estado | Evidencia |
|---|----------|--------|-----------|
| 1 | Conecta a pipe `\\.\pipe\echo_slave_<account_id>` | ✅ CUMPLE | Línea 199 |
| 2 | Ejecuta market order en cuenta demo | ✅ CUMPLE | Línea 353: `OrderSend(...)` |
| 3 | Reporta ticket al Agent | ✅ CUMPLE | Línea 367: `SendExecutionResult(...)` |
| 4 | Logs estructurados en Expert tab | ✅ CUMPLE | Líneas 37-55 |
| 5 | Usa DLL echo_pipe.dll correctamente | ✅ CUMPLE | Ver sección 5 |

---

## 📝 Detalles de Validación

### 1. ✅ Conexión a Named Pipe

**Archivo**: `slave.mq4:197-210`

**Validaciones**:
- ✅ Import del DLL correcto (línea 17-22)
- ✅ Usa `long` para handles (evita truncamiento x64)
- ✅ Nombre del pipe correcto: `PipeBaseName + AccountNumber()`
- ✅ Validación de conexión con `PipeConnected()`
- ✅ Reconexión automática con throttling (5s)

**Código clave**:
```mql4
void ConnectToPipe()
{
   string pipe = PipeBaseName + IntegerToString(AccountNumber());
   g_Pipe = ConnectPipe(pipe);
   if(PipeConnected())
   {
      Log("INFO","Pipe connected","pipe="+pipe);
      SendHandshake();
   }
   else
   {
      Log("ERROR","Pipe connection failed","pipe="+pipe);
   }
}
```

---

### 2. ✅ Handshake con Metadata Completa

**Archivo**: `slave.mq4:177-195`

**Validaciones**:
- ✅ Todos los campos requeridos presentes:
  - `type`: "handshake"
  - `timestamp_ms`: GetTickCount()
  - `payload.client_id`: "slave_<account_id>"
  - `payload.account_id`: "<account_id>"
  - `payload.broker`: AccountCompany() escapado
  - `payload.role`: "slave"
  - `payload.symbol`: "XAUUSD"
  - `payload.version`: "0.1.0"
- ✅ JSON line-delimited (termina con `\n`)
- ✅ Usa `WritePipeW` (conversión UTF-16 → UTF-8)
- ✅ Log estructurado del resultado

**Formato JSON**:
```json
{
  "type":"handshake",
  "timestamp_ms":12345,
  "payload":{
    "client_id":"slave_67890",
    "account_id":"67890",
    "broker":"IC Markets",
    "role":"slave",
    "symbol":"XAUUSD",
    "version":"0.1.0"
  }
}
```

---

### 3. ✅ Recepción y Ejecución de ExecuteOrder

**Archivo**: `slave.mq4:308-369`

**Validaciones**:
- ✅ Captura timestamp `t5` inmediatamente (línea 310)
- ✅ Valida símbolo (debe ser XAUUSD) (líneas 312-330)
- ✅ Parsea todos los campos requeridos:
  - `command_id`, `trade_id`, `order_side`, `lot_size`, `magic_number`
- ✅ Extrae timestamps previos (t0-t4) del payload (líneas 338-346)
- ✅ Determina operación (OP_BUY/OP_SELL) correctamente (línea 348)
- ✅ Refresca precios con `RefreshRates()` (línea 349)
- ✅ Captura `t6` antes de OrderSend (línea 352)
- ✅ Ejecuta `OrderSend` con MagicNumber correcto (línea 353)
- ✅ Captura `t7` después de OrderSend (línea 354)
- ✅ Obtiene precio de ejecución real (líneas 358-360)
- ✅ Logs estructurados (INFO en éxito, ERROR en fallo)

**Parámetros de OrderSend**:
```mql4
int ticket = OrderSend(
    TradeSymbol,       // XAUUSD hardcoded
    op,                // OP_BUY o OP_SELL
    lotSize,           // Del Core (0.10 en i0)
    price,             // Ask o Bid según operación
    SLIPPAGE_POINTS,   // 3 points
    0,                 // SL (sin SL en i0)
    0,                 // TP (sin TP en i0)
    "Echo Slave",      // Comment
    magicNumber,       // MagicNumber del Master (CRÍTICO)
    0,                 // Expiration (sin límite)
    clrGreen           // Color
);
```

---

### 4. ✅ Envío de ExecutionResult con Timestamps

**Archivo**: `slave.mq4:241-272`

**Validaciones**:
- ✅ Todos los campos requeridos:
  - `type`: "execution_result"
  - `command_id`, `trade_id`, `client_id`
  - `success`: bool
  - `ticket`: MT4 ticket
  - `error_code`: mapeado a string (ERR_NO_ERROR, etc.)
  - `error_message`: escapado
  - `executed_price`: precio real de ejecución
- ✅ **Timestamps completos (t0-t7)**:
  - `t0_master_ea_ms`: Master EA genera intent
  - `t1_agent_recv_ms`: Agent recibe de pipe
  - `t2_core_recv_ms`: Core recibe de stream
  - `t3_core_send_ms`: Core envía ExecuteOrder
  - `t4_agent_recv_ms`: Agent recibe ExecuteOrder
  - `t5_slave_ea_recv_ms`: Slave EA recibe comando
  - `t6_order_send_ms`: Slave EA llama OrderSend
  - `t7_order_filled_ms`: OrderSend retorna
- ✅ JSON line-delimited (termina con `\n`)
- ✅ Usa `WritePipeW`
- ✅ Log estructurado del resultado

**Formato JSON**:
```json
{
  "type":"execution_result",
  "timestamp_ms":12345,
  "payload":{
    "command_id":"01HKQV8Y...",
    "trade_id":"01HKQV8Y...",
    "client_id":"slave_67890",
    "success":true,
    "ticket":111222,
    "error_code":"ERR_NO_ERROR",
    "error_message":"",
    "executed_price":2045.52,
    "timestamps":{
      "t0_master_ea_ms":1698345601000,
      "t1_agent_recv_ms":1698345601010,
      "t2_core_recv_ms":1698345601020,
      "t3_core_send_ms":1698345601030,
      "t4_agent_recv_ms":1698345601040,
      "t5_slave_ea_recv_ms":1698345601050,
      "t6_order_send_ms":1698345601100,
      "t7_order_filled_ms":1698345601140
    }
  }
}
```

---

### 5. ✅ Uso Correcto del DLL echo_pipe

**Archivo**: `slave.mq4:16-22, 74-86, 197-210, 457-471`

**Validaciones**:
- ✅ Import correcto:
  ```mql4
  #import "echo_pipe.dll"
     long  ConnectPipe(string pipeName);           // Usa 'long'
     int   WritePipeW(long handle, string data);   // UTF-16 → UTF-8
     int   ReadPipeLine(long handle, uchar &buffer[], int bufferSize);
     void  ClosePipe(long handle);
  #import
  ```
- ✅ Usa `long` para handles (NO `int`, previene truncamiento en x64)
- ✅ Usa `WritePipeW` (con W) para escritura
- ✅ Usa `uchar` buffer en `ReadPipeLine` (mejor para UTF-8)
- ✅ Helper `ReadPipeLineString` para conversión a string
- ✅ Llama `ClosePipe` en `OnDeinit` (previene leaks)
- ✅ Polling no bloqueante en `OnTimer` (1 segundo)

**Comparación con ejemplo oficial**:

| Aspecto | Ejemplo Oficial | Slave EA | ✓ |
|---------|----------------|----------|---|
| Handle type | `long` | `long` | ✅ |
| Write function | `WritePipeW` | `WritePipeW` | ✅ |
| Buffer type | `char[]` | `uchar[]` | ✅ (mejor) |
| Close en OnDeinit | Sí | Sí | ✅ |
| Polling | OnTimer | OnTimer | ✅ |

---

### 6. ✅ Logs Estructurados

**Archivo**: `slave.mq4:37-55`

**Validaciones**:
- ✅ Formato correcto: `[LEVEL] timestamp_ms | event | details`
- ✅ Niveles soportados: `INFO`, `ERROR`, `WARN`, `DEBUG`
- ✅ Timestamp: `GetTickCount()`
- ✅ Siempre imprime a `Expert` tab
- ✅ Opcionalmente escribe a archivo: `echo_slave_<account_id>.log`
- ✅ Escape JSON en strings con `EscapeJSON()`

**Ejemplo de logs**:
```
[INFO] 1234567 | Pipe connected | pipe=\\.\pipe\echo_slave_67890
[INFO] 1234568 | Handshake sent | bytes=234
[INFO] 1234650 | Command received | type=execute_order,command_id=01HKQV8Y...
[INFO] 1234700 | Order executed | command_id=01HKQV8Y...,ticket=111222,price=2045.52
[INFO] 1234701 | ExecutionResult sent | command_id=01HKQV8Y...,success=true
```

---

### 7. ✅ Handler CloseOrder (Bonus, no requerido en i0)

**Archivo**: `slave.mq4:371-417`

**Validaciones**:
- ✅ Parsea todos los campos: `command_id`, `close_id`, `symbol`, `ticket`, `magic_number`
- ✅ Valida símbolo
- ✅ Busca orden por `ticket` o por `magic_number + symbol`
- ✅ Obtiene precio de cierre correcto (Bid para BUY, Ask para SELL)
- ✅ Llama `OrderClose`
- ✅ Envía `CloseResult`
- ✅ Logs estructurados

---

### 8. ✅ Mapeo de Error Codes

**Archivo**: `slave.mq4:222-239`

**Validaciones**:
- ✅ Mapea códigos MT4 a strings legibles
- ✅ Coincide con `ErrorCode` enum del proto (RFC-002)
- ✅ Incluye todos los códigos importantes:
  - `ERR_NO_ERROR` (0)
  - `ERR_INVALID_PRICE` (129)
  - `ERR_INVALID_STOPS` (130)
  - `ERR_MARKET_CLOSED` (132)
  - `ERR_TRADE_DISABLED` (133)
  - `ERR_NOT_ENOUGH_MONEY` (134)
  - `ERR_PRICE_CHANGED` (135)
  - `ERR_OFF_QUOTES` (136)
  - `ERR_BROKER_BUSY` (137)
  - `ERR_REQUOTE` (138)
  - `ERR_TIMEOUT` (141)
- ✅ `ERR_UNKNOWN` como fallback

---

## 🧪 Tests Sugeridos (Manual)

### Test 1: Conexión al Pipe
1. Arrancar Agent con logging DEBUG
2. Cargar Slave EA en chart XAUUSD (cuenta demo)
3. Verificar en Expert tab: "Pipe connected"
4. Verificar en logs del Agent: "Handshake received from slave_<id>"

**Criterio de éxito**: Conexión exitosa sin errores

---

### Test 2: Ejecución de Orden Market
1. Con Slave EA conectado
2. Desde Core, enviar ExecuteOrder (BUY, 0.10 lot, XAUUSD)
3. Verificar en Expert tab: "Order executed", ticket visible
4. Verificar en MT4: orden abierta con MagicNumber correcto
5. Verificar en logs del Core: "ExecutionResult received, success=true"

**Criterio de éxito**: Orden ejecutada y reportada con timestamps completos

---

### Test 3: Manejo de Errores
1. Desactivar trading en MT4 (Tools → Options → Expert Advisors → desmarcar "Allow live trading")
2. Enviar ExecuteOrder desde Core
3. Verificar en Expert tab: "OrderSend failed, error=133"
4. Verificar ExecutionResult: `success=false`, `error_code="ERR_TRADE_DISABLED"`

**Criterio de éxito**: Error capturado y reportado correctamente

---

### Test 4: Cierre de Posición
1. Ejecutar orden BUY (desde test 2)
2. Enviar CloseOrder desde Core
3. Verificar en Expert tab: "Order closed"
4. Verificar en MT4: posición cerrada
5. Verificar CloseResult: `success=true`

**Criterio de éxito**: Posición cerrada correctamente

---

### Test 5: Reconexión tras Caída del Pipe
1. Con Slave EA conectado
2. Detener Agent (kill process)
3. Verificar en Expert tab: "ReadPipe failed", "Attempting pipe reconnection"
4. Reiniciar Agent
5. Esperar 5-10 segundos
6. Verificar en Expert tab: "Pipe connected", "Handshake sent"

**Criterio de éxito**: Reconexión automática exitosa

---

## 📊 Métricas de Calidad del Código

| Métrica | Valor | Estado |
|---------|-------|--------|
| Líneas de código | 484 | ✅ |
| Funciones | 17 | ✅ |
| Complejidad ciclomática (est.) | Media | ✅ |
| Parsing JSON manual | Sí (sin libs externas) | ✅ Correcto para MT4 |
| Manejo de errores | Completo | ✅ |
| Logs estructurados | Sí | ✅ |
| Documentación inline | Suficiente | ✅ |

---

## ⚠️ Limitaciones Conocidas (Esperadas en i0)

Las siguientes limitaciones son **esperadas** y están documentadas en RFC-002:

1. **Símbolo único**: Solo XAUUSD hardcoded (línea 11)
2. **Sin SL/TP**: OrderSend con SL=0, TP=0 (línea 353)
3. **Sin reintentos**: Si OrderSend falla, reporta error y termina
4. **Sin persistencia**: Estado se pierde al reiniciar EA
5. **Parsing JSON manual**: Sin librería externa (aceptable para MT4)

Estas limitaciones se resolverán en **Iteración 1+** según roadmap.

---

## 🎯 Conclusión

### ✅ Estado Final: **APROBADO**

El Slave EA cumple **100%** de los requisitos de la Fase 2 del RFC-002 (Iteración 0).

**Fortalezas**:
- ✅ Uso correcto del DLL echo_pipe
- ✅ Timestamps completos (t0-t7) para métricas E2E
- ✅ Logs estructurados y detallados
- ✅ Manejo robusto de errores
- ✅ Reconexión automática
- ✅ MagicNumber replicado correctamente (CRÍTICO para trazabilidad)
- ✅ JSON parsing manual pero correcto
- ✅ Handler de CloseOrder implementado (bonus)

**Recomendaciones para i1+**:
1. Agregar soporte multi-símbolo con validación dinámica
2. Implementar SL/TP con offset y tolerancia
3. Agregar reintentos con backoff exponencial
4. Considerar librería JSON externa (ej: mqljson) para parsing robusto
5. Implementar heartbeat al Agent

---

## 📝 Aprobación

**Revisado por**: Aranea Labs - QA Team  
**Fecha**: 2025-10-25  
**Estado**: ✅ **APROBADO PARA FASE 2**

**Siguiente paso**: Proceder con **Fase 3: Agent Mínimo** (8-12h)

---

**Referencias**:
- [RFC-002: Plan de Implementación i0](RFC-002-iteration-0-implementation.md)
- [RFC-001: Arquitectura General](RFC-001-architecture.md)
- [DLL echo_pipe.cpp](../../pipe/echo_pipe.cpp)
- [Slave EA](../../clients/mt4/slave.mq4)





