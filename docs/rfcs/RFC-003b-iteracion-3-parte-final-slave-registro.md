---
title: "RFC-004c: Iteración 3 — Registro de símbolos y telemetría básica del Slave EA"
version: "1.2"
date: "2025-11-04"
status: "Listo para implementación"
owner: "Arquitectura Echo"
iteration: "3"
depends_on:
  - "docs/00-contexto-general.md"
  - "docs/01-arquitectura-y-roadmap.md"
  - "docs/rfcs/RFC-001-architecture.md"
  - "docs/GUIA-ACTIVAR-NUEVOS-SIMBOLOS.md"
---

## Resumen ejecutivo

Esta versión del RFC entrega la totalidad de los compromisos de la Iteración 3 descrita en `RFC-001-architecture.md`: el Slave EA reporta el mapeo `canonical_symbol ⇄ broker_symbol` con especificaciones de broker, publica precios coalescidos cada 250 ms, mantiene reconexión automática con el Agent, limpia buffers tras cerrar operaciones y habilita al Core para validar StopLevel antes de enviar órdenes. La solución se mantiene simple, sin agregar funcionalidades planeadas para iteraciones posteriores (versionamiento del protocolo, feedback push, modo degradado avanzado), garantizando un tiempo de implementación acorde al roadmap (≈2 días) y cumpliendo los lineamientos de clean code, modularidad y observabilidad del proyecto.

## Contexto y requerimientos

Iteración 3 según `RFC-001-architecture.md` (sección "Iteración 3 (2 días)") demanda:

- Mapeo símbolos (canónico ⇄ broker) reportado por las EAs al conectar.
- Especificaciones del broker (digits, point, tick_size, lotes, stop_level, contract_size).
- Reporting de precios Bid/Ask coalescido cada 250 ms desde los Slaves.
- Reconexión automática EA↔Agent y Agent↔Core.
- Limpieza de buffers de operaciones tras cerrar órdenes en EA, Agent y Core.
- Core valida símbolos contra ETCD, persiste en PostgreSQL y traduce canonical→broker antes de ejecutar.
- Core valida StopLevel previo al envío de órdenes.

## Alcance

### Dentro de alcance

- Modificaciones a `clients/mt4/slave.mq4` para soportar input `SymbolMappings`, handshake con `symbols[]`, reporting de precios 250 ms, reconexión automática y limpieza básica de buffers.
- Actualizaciones en el Agent (`agent/internal/pipe_manager.go`, `agent/internal/state_snapshot.go`) para traducir el handshake a `AccountSymbolsReport`, coalescer snapshots de precios y reenviar tras reconexión.
- Cambios en el Core (`core/internal/router.go`, `core/internal/symbol_repository.go`, `core/internal/stop_validator.go`) para validar símbolos contra ETCD, persistir `contract_size`, aplicar constraint `account_id + broker_symbol` y validar StopLevel antes de `ExecuteOrder`.
- Migración SQL que agrega `contract_size` y constraint de unicidad por cuenta/broker.
- Observabilidad mínima: logs JSON y métricas que permitan monitorear handshake, snapshots y reconexión.

### Fuera de alcance (iteraciones futuras)

- Versionamiento explícito del handshake (`protocol_version`).
- Feedback push `SymbolRegistrationResult` Core→Agent→EA.
- Hash/ciclo de configuración avanzado o panel gráfico de modo degradado.

## Solución técnica

### 1. Input y parser de `SymbolMappings`

- Nuevo parámetro en el Slave EA:
  ```mql
  input string SymbolMappings = ""; // Ej: "US100.pro:NDX,XAUUSD:XAUUSD,GER30:DAX"
  ```
- Reglas:
  - Longitud total ≤ 1024 caracteres.
  - `broker_symbol`: 1..50 caracteres; debe existir (`IsSymbolValid` o `MarketInfo(symbol, MODE_TRADEALLOWED) > 0`).
  - `canonical_symbol`: trim + mayúsculas; 3..20 caracteres.
  - Se rechazan duplicados tanto de `broker_symbol` como de `canonical_symbol`.
- Comportamiento: si el parser no produce al menos un mapeo válido se registra `ERROR` y el EA continúa sin enviar handshake (Core operará en modo `warn`).

### 2. Handshake `symbols[]`

- `SendHandshake()` se ejecuta:
  - En `OnInit()` cuando el parser tiene éxito.
  - Tras reconexión del pipe.
- Payload JSON (sin `protocol_version`):
  ```json
  {
    "type": "handshake",
    "timestamp_ms": 1730660400000,
    "payload": {
      "client_id": "slave_12345",
      "account_id": "12345",
      "symbols": [
        {
          "canonical_symbol": "XAUUSD",
          "broker_symbol": "XAUUSD.m",
          "digits": 2,
          "point": 0.01,
          "tick_size": 0.01,
          "min_lot": 0.01,
          "max_lot": 100.0,
          "lot_step": 0.01,
          "stop_level": 0,
          "contract_size": 100.0
        }
      ]
    }
  }
  ```
- Construcción: hasta tres intentos con backoff fijo de 1 s cuando `MarketInfo()` retorna valores inválidos. Símbolos que siguen fallando se omiten con `WARN`.

### 3. Reporting de precios cada 250 ms

- El Slave envía snapshots de precios por símbolo operado usando coalescing:
  ```mql
  void OnTick()
  {
     static ulong lastSnapshotMs = 0;
     ulong nowMs = GetTickCount();
     if(nowMs - lastSnapshotMs < 250) return;
     lastSnapshotMs = nowMs;

     double bid = MarketInfo(Symbol(), MODE_BID);
     double ask = MarketInfo(Symbol(), MODE_ASK);

     string payload =
        "{"+
        "\"type\":\"state_snapshot\","+
        "\"timestamp_ms\":" + ULongToStr(nowMs) + ","+
        "\"payload\":{"+
        "\"account_id\":\"" + IntegerToString(AccountNumber()) + "\","+
        "\"symbol\":\"" + EscapeJSON(Symbol()) + "\","+
        "\"bid\":" + DoubleToString(bid, Digits) + ","+
        "\"ask\":" + DoubleToString(ask, Digits) +
        "}}";

     PipeWriteLn(payload);
  }
  ```
- El Agent coalescea snapshots por cuenta/símbolo y los reenvía al Core vía `StateSnapshot` gRPC.

### 4. Reconexión automática

- El EA programa `EventSetTimer(2)` para verificar conexión:
  ```mql
  void OnTimer()
  {
     if(!PipeIsOpen())
     {
        if(ReconnectPipe())
        {
           SendHandshake();
           lastSnapshotMs = 0; // fuerza snapshot inmediato
        }
     }
  }
  ```
- Se reutilizan helpers existentes (`PipeIsOpen`, `ReconnectPipe`). El Agent ya implementa reconexión gRPC; se documenta prueba específica en sección de testing.

### 5. Limpieza de buffers

- Slave EA: al recibir `ORDER_CLOSE` o `EXECUTION_RESULT`, borra estructuras temporales (`ArrayResize` a 0) para deduplicación.
- Agent: después de enviar `ExecutionResult` al Core, limpia mapas asociativos `trade_id → tickets`.
- Core: al cerrar posiciones, elimina entradas en caches (`symbolResolver`, `pendingCommands`).
- Se agregan métricas `echo.buffers.cleared_count` por componente.

### 6. Agent – traducción de handshake y snapshots

- Handshake → `AccountSymbolsReport` (pseudo-Go):
  ```go
  func (h *PipeHandler) handleHandshake(msg pipeMessage) {
      symbols := make([]*pb.SymbolMapping, 0, len(msg.Payload.Symbols))
      for _, s := range msg.Payload.Symbols {
          symbols = append(symbols, &pb.SymbolMapping{
              CanonicalSymbol: s.Canonical,
              BrokerSymbol:    s.Broker,
              Digits:          int32(s.Digits),
              Point:           s.Point,
              TickSize:        s.TickSize,
              MinLot:          s.MinLot,
              MaxLot:          s.MaxLot,
              LotStep:         s.LotStep,
              StopLevel:       int32(s.StopLevel),
              ContractSize:    s.ContractSize,
          })
      }

      report := &pb.AccountSymbolsReport{
          AccountId:    msg.Payload.AccountID,
          Symbols:      symbols,
          ReportedAtMs: time.Now().UnixMilli(),
      }
      h.coreStream.Send(&pb.AgentMessage{
          Payload: &pb.AgentMessage_AccountSymbolsReport{AccountSymbolsReport: report},
      })
  }
  ```
- Snapshots → `StateSnapshot` coalescido (250 ms) para cada cuenta/símbolo.

### 7. Core – validaciones y persistencia

- Validar cada `canonical_symbol` con `core/canonical_symbols` (ETCD). Si no existe se registra error y se ignora el símbolo.
- Persistir en PostgreSQL (`account_symbol_map`) mediante UPSERT por `(account_id, broker_symbol)` actualizando `contract_size` cuando cambie.
- StopLevel: antes de `ExecuteOrder` el Core verifica `MarketSpec.StopLevel` del broker y ajusta SL/TP o rechaza con error `INVALID_STOPS`.
- Limpiar buffers tras `CloseOrder` o cuando se recibe `StateSnapshot` que indique posiciones 0.

### 8. Observabilidad

- Logs JSON (EA/Agent/Core) con campos obligatorios: `timestamp`, `level`, `message`, `file_path`, `line_nro`, `app`, `feature`, `event`, `account_id`, `symbol`, `metadata`.
- Métricas clave:
  - `echo.handshake.sent_count{account_id}`.
  - `echo.handshake.parse_error_count{account_id}`.
  - `echo.snapshot.sent_count{account_id,symbol}` y `coalesce_ms` (p95 ≤ 250 ms).
  - `echo.reconnect.attempts{component}`.
  - `echo.buffers.cleared_count{component}`.
  - `echo.stop_validation.failures{account_id,symbol}`.

## Migraciones

```sql
-- Agregar contract_size (nullable) y constraint por cuenta/broker
ALTER TABLE echo.account_symbol_map
  ADD COLUMN IF NOT EXISTS contract_size NUMERIC(18,8);

ALTER TABLE echo.account_symbol_map
  DROP CONSTRAINT IF EXISTS account_symbol_map_pkey;

ALTER TABLE echo.account_symbol_map
  ADD CONSTRAINT account_symbol_map_pkey
  PRIMARY KEY (account_id, broker_symbol);

CREATE INDEX IF NOT EXISTS idx_account_symbol_map_canonical
  ON echo.account_symbol_map (account_id, canonical_symbol);
```

## Pruebas

1. **Unitarias EA**: parser (formatos válidos/ inválidos, duplicados), helper `EscapeJSON`, reporting 250 ms.
2. **Unitarias Agent/Core**: traducción a proto, UPSERT en DB, validación StopLevel, reconexión gRPC.
3. **Integración**:
   - Configurar `SymbolMappings = "US100.pro:NDX,XAUUSD:XAUUSD"`.
   - Verificar handshake en logs, snapshot 250 ms y registros en DB.
   - Simular desconexión de pipe → reconexión + handshake nuevo.
   - Ejecutar trade con SL/TP fuera de StopLevel; Core debe rechazar y loguear `INVALID_STOPS`.
4. **Negativos**: parser vacío, `MarketInfo()` inválido, símbolo no permitido en ETCD.

## Plan de despliegue

1. Aplicar migraciones SQL en ambiente piloto.
2. Distribuir binarios del EA actualizados y documentación de `SymbolMappings`.
3. Activar en cuentas piloto con `core/symbols/unknown_action=warn`, monitorear 24 h métricas de handshake/snapshot.
4. Cambiar a `unknown_action=reject`, monitorear buffers y reconexiones adicional 48 h.
5. Despliegue masivo.

## Riesgos y mitigaciones

- **Configuraciones incompletas**: se detectan por logs `parse_error` y métricas; checklist operativo actualizado.
- **Reconexión fallida**: alertas por `echo.reconnect.attempts`. Se documenta procedimiento manual.
- **StopLevel desalineado**: fallback a rechazo controlado y observabilidad de `stop_validation.failures`.
- **Carga adicional por snapshots**: coalescing 250 ms mantiene overhead bajo (≤4 mensajes/segundo por símbolo).

## Checklist de implementación

- ✅ Input y parser robusto de `SymbolMappings`.
- ✅ Handshake inicial con especificaciones completas.
- ✅ Snapshots Bid/Ask cada 250 ms.
- ✅ Reconexión automática EA↔Agent.
- ✅ Limpieza de buffers en EA, Agent y Core.
- ✅ Validación StopLevel previa a ejecución.
- ✅ Migraciones SQL aplicadas.
- ✅ Métricas y logs configurados.
- ⏳ Pruebas integrales ejecutadas con evidencia.

## Referencias

- `docs/00-contexto-general.md`
- `docs/01-arquitectura-y-roadmap.md`
- `docs/rfcs/RFC-001-architecture.md`
- `docs/GUIA-ACTIVAR-NUEVOS-SIMBOLOS.md`
- `agent/internal/pipe_manager.go`
- `core/internal/router.go`

