---
title: "RFC-004b: Iteración 3 (Parte 2 y final) — Autoregistro de símbolos desde Slave EA"
version: "1.1"
date: "2025-10-30"
status: "Propuesta para aprobación"
owner: "Arquitectura Echo"
iteration: "3 (parte 2/final)"
depends_on:
  - "docs/rfcs/RFC-004-iteracion-3-catalogo-simbolos.md"
  - "docs/01-arquitectura-y-roadmap.md"
  - "docs/GUIA-ACTIVAR-NUEVOS-SIMBOLOS.md"
---

## Resumen ejecutivo

Esta RFC completa la Iteración 3 con su segunda y última parte, enfocada en el Slave EA (MT4) para que envíe, durante el handshake por Named Pipe, la lista completa de símbolos disponibles por cuenta con sus especificaciones mínimas, habilitando el autoregistro en el Core. El Agent ya convierte este payload en `AccountSymbolsReport` y el Core ya valida, cachea y persiste los mapeos por cuenta.

Objetivos medibles (i3 parte 2):
- 100% de cuentas Slave reportan sus símbolos en el handshake inicial (y en reconexiones).
- Core puede operar solo sobre cuentas/símbolos correctamente mapeados: 0 órdenes enviadas a slaves sin mapeo cuando `unknown_action=reject`.
- Telemetría visible: `AccountSymbolsReport sent` (Agent) y `Symbol mappings upserted` (Core) por cuenta, con conteos consistentes.

No se modifican contratos ni servicios en Go en esta parte; solo se cambia el Slave EA. Se documenta un ajuste menor recomendado en Core para robustez (normalización antes del lookup), ver §7.2.

## Contexto y alineación con el roadmap

Referencias oficiales del roadmap:

```157:162:/home/kor/go/src/github.com/xKoRx/echo/docs/01-arquitectura-y-roadmap.md
Iteración 3 — Catálogo canónico de símbolos y mapeo por cuenta
- Objetivo: estandarizar `canonical_symbol ⇄ broker_symbol` por cuenta.
- Alcance: catálogo canónico en Core; agent/EA reportan mapeo del broker al conectar; validaciones previas al envío.
- Exclusiones: sizing y políticas.
- Criterios de salida: 0 errores por símbolo desconocido; mapeo persistido y trazable.
```

Primera parte de i3 (aprobada) implementó SDK/Agent/Core, persistencia y validaciones. Esta segunda parte entrega el cambio en el Slave EA para que el sistema sea end-to-end: la cuenta se autoregistra con los símbolos que opera, el Core persiste y resuelve mapeos por cuenta, y el Router envía órdenes con el símbolo del broker correcto.

## Principios de diseño (World‑Class, Clean Code, SOLID, Modularidad)

- SDK‑first, Core como orquestador, Agent como bridge; el EA solo reporta datos que conoce (especificaciones del broker). 0 lógica de negocio en el EA.
- Configuración exclusivamente en ETCD (Core); el EA no lee configuración central, solo reporta.
- Observabilidad consistente: logs estructurados en EA; métricas y spans en Agent/Core vía SDK.
- Contratos estables: se utiliza `pb.SymbolMapping` vía `AgentSymbolsReport` ya disponible en `sdk/proto/v1/agent.proto`.

## Estado actual (antes de esta RFC)

- Agent extrae `symbols` del handshake y envía `AccountSymbolsReport` al Core:

```611:697:/home/kor/go/src/github.com/xKoRx/echo/agent/internal/pipe_manager.go
// handleHandshake procesa un handshake del EA (i3).
// Extrae symbols del handshake y envía AccountSymbolsReport al Core si están presentes.
func (h *PipeHandler) handleHandshake(msgMap map[string]interface{}) error { /* ... */ }
```

- Core valida canónicos, cachea y persiste mapeos por cuenta; resuelve en hot‑path en el Router:

```667:735:/home/kor/go/src/github.com/xKoRx/echo/core/internal/core.go
// handleAccountSymbolsReport: valida, upserta en caché y encola persistencia async.
```

```404:431:/home/kor/go/src/github.com/xKoRx/echo/core/internal/router.go
// i3: Traducir símbolo canónico a broker_symbol por cuenta
// ResolveForAccount(ctx, slaveAccountID, canonicalSymbol)
```

- La tabla `echo.account_symbol_map` existe y está migrada (deploy):

```13:40:/home/kor/go/src/github.com/xKoRx/echo/deploy/postgres/migrations/i3_symbols.sql
CREATE TABLE IF NOT EXISTS echo.account_symbol_map (...);
```

- El Slave EA hoy no envía `symbols` en su handshake:

```290:308:/home/kor/go/src/github.com/xKoRx/echo/clients/mt4/slave.mq4
void SendHandshake()
{
   string payload =
      "{"
      +"\"type\":\"handshake\"," /* ... */
      +"\"role\":\"slave\"," /* no incluye symbols */
      +"\"version\":\"0.2.0\""
      +"}"
      +"}";
   PipeWriteLn(payload);
}
```

Conclusión: falta solo modificar el Slave EA para enviar `symbols` en handshake, completando i3.

## Alcance de la RFC

- En alcance:
  - Modificar `clients/mt4/slave.mq4` para incluir `payload.symbols[]` con uno o más `SymbolMapping` por cuenta.
  - El EA NO normaliza; reporta `broker_symbol` tal cual y setea `canonical_symbol` igual a `broker_symbol` (best‑effort). La normalización canónica y validación se realizan en SDK/Core.
  - Logging mínimo en EA: conteo de símbolos reportados.

- Fuera de alcance:
  - Cambios en Agent/Core (ya implementados en la primera parte de i3).
  - Cambios en Master EA (se asume uso de canónicos definidos en ETCD, ver §7.1).

## Especificación del handshake extendido (EA → Agent)

Formato JSON (compat con la guía oficial):

```json
{
  "type": "handshake",
  "timestamp_ms": 1730250000000,
  "payload": {
    "client_id": "slave_123456",
    "account_id": "123456",
    "broker": "Acme Markets Ltd",
    "role": "slave",
    "version": "0.3.0",
    "symbols": [
      {
        "broker_symbol": "XAUUSD.m",
        "canonical_symbol": "XAUUSD.m",
        "digits": 2,
        "point": 0.01,
        "tick_size": 0.01,
        "min_lot": 0.01,
        "max_lot": 100.0,
        "lot_step": 0.01,
        "stop_level": 30,
        "contract_size": 100.0
      }
    ]
  }
}
```

Campos requeridos por símbolo: `broker_symbol`, `canonical_symbol` (puede ser igual a `broker_symbol` si el EA no conoce el canónico), `digits`, `point`, `tick_size`, `min_lot`, `max_lot`, `lot_step`, `stop_level`. Campo opcional: `contract_size`.

Normalización canónica: la realiza exclusivamente el SDK/Core mediante `domain.NormalizeCanonical()` y `ValidateCanonicalSymbol()` usando la whitelist de ETCD. El valor enviado por el EA en `canonical_symbol` es solo best‑effort (puede ser igual a `broker_symbol`).

Nota: El Agent convierte este payload a `AccountSymbolsReport` y le agrega `reported_at_ms` automáticamente.

## Cambios requeridos en `clients/mt4/slave.mq4`

1) Construcción de `symbols[]` a partir de los símbolos disponibles en el broker (dos opciones):
   - Estática (rápida): lista blanca local `symbolsToReport[]` (ej.: `{"XAUUSD","GER30.x"}`) y normalización por sufijos.
   - Dinámica (recomendada): iterar `Market Watch` con `SymbolsTotal(true)`/`SymbolName(i, true)`, filtrar y construir cada entrada con `MarketInfo()`.

2) Sustituir el handshake actual por uno que incluya `symbols`. Mantener el resto del payload (client_id, account_id, broker, role, version) y el log informativo con el conteo.

Puntos de edición (resumen):

```290:308:/home/kor/go/src/github.com/xKoRx/echo/clients/mt4/slave.mq4
void SendHandshake() { /* reemplazar por versión con symbols[] */ }
```

Recomendaciones de implementación (MQL4):
- Usar `MarketInfo(sym, MODE_*)` para `digits`, `point`, `tick_size`, `min_lot`, `max_lot`, `lot_step`, `stop_level`, `contract_size`.
- NO normalizar en el EA: setear `canonical_symbol = sym` (el Core normaliza con SDK).
- JSON seguro: reutilizar `EscapeJSON()` existente para `broker_symbol`.
- Performance: limitar el set reportado a símbolos realmente operados; no enviar listas masivas irrelevantes.

Ejemplo (estático) a integrar en `SendHandshake()` (pseudocódigo MQL4):

```mql
string symbolsToReport[] = {"XAUUSD", "GER30.x"};
string symbolsJson = ""; int count = 0;
for (int i = 0; i < ArraySize(symbolsToReport); i++) {
  string sym = symbolsToReport[i];
  if (!IsSymbolValid(sym)) continue;
  string canonical = sym; // NO normalizar en el EA; el Core normaliza
  // leer specs del broker
  int digits = (int)MarketInfo(sym, MODE_DIGITS);
  double point = MarketInfo(sym, MODE_POINT);
  double tickSize = MarketInfo(sym, MODE_TICKSIZE);
  double minLot = MarketInfo(sym, MODE_MINLOT);
  double maxLot = MarketInfo(sym, MODE_MAXLOT);
  double lotStep = MarketInfo(sym, MODE_LOTSTEP);
  int stopLevel = (int)MarketInfo(sym, MODE_STOPLEVEL);
  double contractSize = MarketInfo(sym, MODE_LOTSIZE);
  // Validaciones mínimas locales
  if(point <= 0 || minLot <= 0 || maxLot <= 0 || lotStep <= 0 || tickSize <= 0) {
     Log("WARN", "Invalid symbol specs", "symbol=" + sym);
     continue;
  }
  if (count > 0) symbolsJson += ",";
  symbolsJson += "{\"canonical_symbol\":\""+canonical+"\"," +
                 "\"broker_symbol\":\""+EscapeJSON(sym)+"\"," +
                 "\"digits\":"+IntegerToString(digits)+"," +
                 "\"point\":"+DoubleToString(point,10)+"," +
                 "\"tick_size\":"+DoubleToString(tickSize,10)+"," +
                 "\"min_lot\":"+DoubleToString(minLot,10)+"," +
                 "\"max_lot\":"+DoubleToString(maxLot,10)+"," +
                 "\"lot_step\":"+DoubleToString(lotStep,10)+"," +
                 "\"stop_level\":"+IntegerToString(stopLevel)+"}";
  if (contractSize > 0) symbolsJson = StringSubstr(symbolsJson,0,StringLen(symbolsJson)-1)+",\"contract_size\":"+DoubleToString(contractSize,10)+"}";
  count++;
}
// ensamblar handshake con symbolsJson y enviar
```

La guía operativa completa y ejemplo dinámico está en `docs/GUIA-ACTIVAR-NUEVOS-SIMBOLOS.md`.

## Telemetría y trazabilidad

- EA: mantener `Log("INFO","Handshake sent","account=... symbols=COUNT")` y (opcional debug) un log por símbolo reportado.
- Agent/Core: ya instrumentados (métricas `echo.symbols.*`, spans de procesamiento y logs).
- Métricas adicionales recomendadas (Agent/Core):
  - `echo.symbols.handshake_size` (histogram, bytes del JSON del handshake procesado).
  - `echo.symbols.reported_count` (histogram, símbolos por handshake).
  - `echo.symbols.handshake_stale` (counter, reportes con `reported_at_ms` ≤ último procesado para la cuenta).
  - `echo.symbols.duplicates` (counter, reportes rechazados por `canonical_symbol` duplicado).

## Compatibilidad y rollout

1) Desplegar primero Core i3 (ya listo) con `core/symbols/unknown_action=warn` (ETCD) para compatibilidad.
2) Desplegar Agents (ya listos) y EAs con esta modificación. Verificar `AccountSymbolsReport sent` y `Symbol mappings upserted`.
3) Activar `reject` cuando 100% de cuentas reporten (cambio de flag en ETCD).

Orden de procesamiento (Agent): el Agent envía `AccountSymbolsReport` inmediatamente tras el handshake (fire‑and‑forget) y NO espera confirmación del Core. Durante el rollout, el Core opera en `unknown_action=warn`. Una vez 100% de cuentas reporten correctamente, activar `reject` en ETCD. Esta estrategia elimina bloqueos por sincronización y evita timeouts artificiales.

Rollback: si >10% de cuentas activas (conectadas al menos 1 vez en las últimas 48h) no reportan símbolos tras 48h del despliegue de EAs actualizados, volver a `unknown_action=warn` y, de ser necesario, revertir EAs a i2 mientras se corrige la causa raíz. El Core en modo `warn` permite rollback sin downtime.

Métrica de referencia (consultas de apoyo):
```sql
-- Cuentas activas con conexión reciente (ejemplo de tabla de conexiones)
SELECT COUNT(DISTINCT account_id)
FROM echo.account_connections
WHERE last_connected_at >= NOW() - INTERVAL '48 hours';

-- Cuentas que reportaron símbolos en las últimas 48h
SELECT COUNT(DISTINCT account_id)
FROM echo.account_symbol_map
WHERE reported_at_ms >= (EXTRACT(EPOCH FROM NOW() - INTERVAL '48 hours') * 1000);
```

## Criterios de aceptación

- [ ] Todas las cuentas Slave envían `symbols` en el handshake (y tras reconexión), el Agent reporta `AccountSymbolsReport` al Core.
- [ ] El Core resuelve `canonical → broker_symbol` por cuenta en `ExecuteOrder` y `CloseOrder`; con `reject` no se envían órdenes sin mapeo.
- [ ] Persistencia en `echo.account_symbol_map` consistente con los reportes (campos y conteos).
- [ ] Métricas `echo.symbols.lookup`, `echo.symbols.reported`, `echo.symbols.validate` activas y coherentes.

## Validación manual (resumen)

1) ETCD: `core/canonical_symbols` contiene los canónicos esperados; `core/symbols/unknown_action=warn` durante el rollout.
2) Logs Agent: `AccountSymbolsReport sent to Core (i3) | account_id=... symbols_count=...`.
3) Logs Core: `Symbol mappings upserted (i3)` y `Account mappings loaded from PostgreSQL (lazy load)` cuando aplique.
4) BD: tabla `echo.account_symbol_map` con filas por `account_id × canonical_symbol` esperadas.
5) Flujo E2E: TradeIntent con canónico → ExecuteOrder al Slave con `broker_symbol` correcto.

## Consideraciones y ajustes menores

### 7.1 Precondición de símbolo canónico en TradeIntent
El Router valida el símbolo canónico del `TradeIntent`. El Master EA puede configurar mapeo canónico mediante input params (ver §6.6) o enviar el `broker_symbol` tal cual; el Core aplica `domain.NormalizeCanonical()` antes de validar contra ETCD como fallback. Esto garantiza compatibilidad sin acoplar el Master EA a ETCD.

### 7.2 Mejora de robustez en Core (recomendada)
Obligatorio: en `createExecuteOrders`, normalizar el símbolo del `TradeIntent` antes del lookup para prevenir misses si el Master aún no está adaptado:

```go
// core/internal/router.go (dentro de createExecuteOrders)
canonicalSymbol, _ := domain.NormalizeCanonical(intent.Symbol)
brokerSymbol, _, found := r.core.symbolResolver.ResolveForAccount(ctx, slaveAccountID, canonicalSymbol)
```

Esto no cambia el contrato ni la validación existente y es backward compatible.

### 7.3 Semántica de invalidación (hard‑replace)
Cada `AccountSymbolsReport` representa la lista completa y autoritativa de símbolos de la cuenta. Procesamiento:
1) Hard‑replace en caché: eliminar de la caché de la cuenta los canónicos que no aparezcan en el nuevo reporte; luego upsert de los nuevos.
2) Hard‑replace en PostgreSQL: ejecutar `DELETE FROM echo.account_symbol_map WHERE account_id=$1 AND canonical_symbol NOT IN (...)` antes del upsert batch; idempotencia por `reported_at_ms`.
3) Idempotencia temporal: si `reported_at_ms` ≤ último procesado para la cuenta, ignorar el reporte (no invalidar ni upsert).

SQL recomendado (transaccional):
```sql
BEGIN;

-- Paso 1: invalidar símbolos que ya no están en el reporte
DELETE FROM echo.account_symbol_map
WHERE account_id = $1
  AND canonical_symbol NOT IN (
    SELECT unnest($2::text[]) -- array canónicos reportados
  );

-- Paso 2: upsert batch de nuevos símbolos
INSERT INTO echo.account_symbol_map (
  account_id, canonical_symbol, broker_symbol,
  digits, point, tick_size, min_lot, max_lot, lot_step, stop_level,
  reported_at_ms, updated_at, contract_size
)
VALUES
  -- construir valores por cada mapping del reporte
  -- ($1, $can, $broker, $digits, $point, $tick_size, $min_lot, $max_lot, $lot_step, $stop_level, $reported_at, NOW(), $contract_size)

ON CONFLICT (account_id, canonical_symbol)
DO UPDATE SET
  broker_symbol   = EXCLUDED.broker_symbol,
  digits          = EXCLUDED.digits,
  point           = EXCLUDED.point,
  tick_size       = EXCLUDED.tick_size,
  min_lot         = EXCLUDED.min_lot,
  max_lot         = EXCLUDED.max_lot,
  lot_step        = EXCLUDED.lot_step,
  stop_level      = EXCLUDED.stop_level,
  contract_size   = NULLIF(EXCLUDED.contract_size, 0.0),
  reported_at_ms  = EXCLUDED.reported_at_ms,
  updated_at      = NOW()
WHERE EXCLUDED.reported_at_ms >= echo.account_symbol_map.reported_at_ms;

COMMIT;
```

## Impacto en componentes

- Slave EA (MT4): modificación en `SendHandshake()` y helpers locales de construcción de `symbols`.
- Agent (Go): sin cambios (ya implementa parse y envío a Core).
- Core (Go): sin cambios obligatorios; mejora recomendada §7.2 (opcional y segura).
- SDK: sin cambios (contratos y validaciones ya presentes).
- Persistencia: sin cambios (migración i3 aplicada).
 - Observabilidad: añadir métricas sugeridas en Agent/Core.

## Checklist de PR (bloqueante)

- [ ] Modificación de `clients/mt4/slave.mq4` para incluir `symbols` en handshake.
- [ ] Versión del EA incrementada y unificada a SemVer (ej.: `0.3.0`). Usar constante `EA_VERSION` en el handshake.
- [ ] Evidencias: captura de logs EA/Agent/Core y SELECT a `echo.account_symbol_map` (antes/después).
- [ ] Documentación actualizada: `docs/ea/*` si aplica, referencia a `GUIA-ACTIVAR-NUEVOS-SIMBOLOS.md`.
- [ ] Despliegue coordinado con flag `core/symbols/unknown_action` (ETCD).
- [ ] Idioma: PR completamente en español, usando la plantilla estándar y versión SemVer.

## Riesgos y mitigaciones

| Riesgo | Mitigación |
|---|---|
| Canónico no coincide con ETCD | Core/SDK normalizan y validan contra ETCD; el EA reporta broker_symbol tal cual. |
| Listas muy grandes de símbolos | Reportar solo símbolos relevantes; validación de tamaño en revisión de PR. |
| Reconexiones frecuentes de EA | El handshake ya se reenvía; Agent/Core son idempotentes vía `reported_at_ms`. |

## Sección nueva — §6.5 Validaciones y observabilidad del handshake

Para garantizar robustez sin límites artificiales:

1) Validación de duplicados (DECISIÓN i3): si el EA reporta el mismo `canonical_symbol` dos veces, el SDK debe **rechazar el reporte completo** con error explícito. El Core debe loggear WARNING y registrar métrica `echo.symbols.duplicates` con `account_id` y `canonical_symbol`. El EA debe reenviar un handshake limpio.
2) Validación de specs en EA: si `MarketInfo()` retorna valores inválidos (≤0), el EA debe saltar el símbolo y loggear WARNING; permitir handshake con 0 símbolos (modo compatibilidad en Core).
3) Idempotencia por timestamp: solo procesar handshakes con `reported_at_ms` > último procesado (evita churn bajo reconexiones).
4) Observabilidad: métricas `echo.symbols.handshake_size`, `echo.symbols.reported_count`, `echo.symbols.handshake_stale`.

Nota sobre `tick_value`: es un valor dinámico que cambia con condiciones de mercado. Por ello, se reporta en `StateSnapshot` (i2+), NO en `AccountSymbolsReport`. El Money Management (i5) lo obtendrá desde el estado actual reportado por el Agent, no desde especificaciones estáticas del handshake.

## Sección nueva — §6.6 Configuración de símbolos en Master EA (input params)

El Master EA no lee ETCD pero puede necesitar enviar canónicos en `TradeIntent`. Solución: input params con mapeo explícito `canonical:broker_symbol`.

Input param recomendado:
```mql
// Master EA
input string SymbolMappings = "NDX:US100.pro,XAUUSD:XAUUSD,DAX:GER30";
```

Parsing y uso:
```mql
string GetCanonicalSymbol(string brokerSymbol) {
    string pairs[];
    StringSplit(SymbolMappings, ',', pairs);
    for(int i = 0; i < ArraySize(pairs); i++) {
        string mapping[];
        int count = StringSplit(pairs[i], ':', mapping);
        if(count == 2 && mapping[1] == brokerSymbol) {
            return mapping[0];
        }
    }
    return brokerSymbol; // fallback: enviar broker_symbol; Core normaliza
}
```

Nota: para `contract_size`, ver política de obligatoriedad en esta sección. Para `tick_value` (dinámico), ver §6.5.

Política de `contract_size` (clarificación i3→i5):
- i3/i4: Persistencia permite `NULL` en `contract_size`. Si el EA envía `0.0`, normalizar a `NULL`. El SDK/Core loggean WARNING cuando falta para símbolos donde `MODE_LOTSIZE`>0.
- i5 (Money Management): `contract_size` es requerido para categorías donde afecta el cálculo (forex/commodities); si falta, el Core rechaza el sizing para ese símbolo/cuenta (fail‑safe) con métrica `echo.sizing.missing_contract_size`.
  - Normalización: PostgreSQL normaliza a `NULL` durante el upsert (ver SQL en §7.3: `NULLIF(EXCLUDED.contract_size, 0.0)`).

## Notas de calidad

- Modularidad: EA reporta; Agent transporta; Core valida/resuelve; SDK concentra contratos y helpers.
- SOLID/Clean Code: responsabilidades separadas por componente; funciones cortas y nombres explícitos.
- Observabilidad: atributos en contexto en Go; logs concisos en EA.
- Configuración: exclusivamente ETCD (Core). El EA no introduce fuentes paralelas de config.

## Apéndice — Referencias

- Código actual del handshake en el Slave EA (a reemplazar):

```290:308:/home/kor/go/src/github.com/xKoRx/echo/clients/mt4/slave.mq4
void SendHandshake() { /* ... */ }
```

- Handler del Agent que ya procesa `symbols` del handshake:

```611:697:/home/kor/go/src/github.com/xKoRx/echo/agent/internal/pipe_manager.go
func (h *PipeHandler) handleHandshake(msgMap map[string]interface{}) error { /* ... */ }
```

- Resolución y routing en Core (uso del mapeo por cuenta):

```404:431:/home/kor/go/src/github.com/xKoRx/echo/core/internal/router.go
// i3: Traducir símbolo canónico a broker_symbol por cuenta
```

---

Fin de RFC.


