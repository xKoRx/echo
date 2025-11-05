---
title: "RFC-005a: Iteración 5 — Handshake versionado y feedback determinístico"
version: "1.0"
date: "2025-11-05"
status: "En desarrollo"
owner: "Arquitectura Echo"
iteration: "5"
depends_on:
  - "docs/00-contexto-general.md"
  - "docs/01-arquitectura-y-roadmap.md"
  - "docs/rfcs/RFC-architecture.md"
  - "docs/rfcs/RFC-004d-iteracion-4-especificaciones-broker.md"
---

## Resumen ejecutivo

La iteración 5 robustece el handshake EA ↔ Agent ↔ Core para operar como una mesa de trading profesional sin slippage operativo. Introducimos `protocol_version` obligatorio, negociación dinámica de capacidades y la respuesta `SymbolRegistrationResult` en sentido Core→Agent→EA. Así eliminamos la ambigüedad sobre qué símbolos quedaron habilitados, detectamos cuentas desalineadas antes del primer tick y bloqueamos versiones incompatibles sin comprometer la latencia. Con esto subimos el listón de confiabilidad antes de abordar el sizing de riesgo fijo (i6), manteniendo la arquitectura modular, SOLID y con observabilidad end-to-end.

## Contexto y motivación

- **Estado actual**: El handshake enviado por los EA (master/slave) no declara versión. El Agent acepta cualquier payload con `type="handshake"` y el Core asume compatibilidad implícita. Si una cuenta reporta símbolos fuera del catálogo o con `SymbolSpecReport` incompleto, recién lo detectamos al momento de enviar el primer `ExecuteOrder`, generando rechazos tardíos (`ERR_SPEC_MISSING`, `ERR_RISK_POLICY_MISSING`) y ruido operativo.
- **Dolor detectado**:
  - Desalineaciones de configuración sin feedback inmediato a los operadores.
  - Imposibilidad de coordinar upgrades graduales entre EAs, Agent y Core porque no existe negociación explícita de capacidades.
  - Falta de trazabilidad: no almacenamos qué símbolos quedaron aprobados tras cada handshake ni la versión del EA.
- **Meta**: Tener un flujo handshake/feedback tan claro como una mesa de control de riesgo: sabemos qué versión se conecta, qué símbolos están operativos, cuáles fueron rechazados y por qué, todo antes de enviar la primera orden.

## Objetivos medibles (Iteración 5)

- 100 % de handshakes entrantes declaran `protocol_version` y `client_semver`; versiones fuera del rango configurado se rechazan antes de permitir `ExecuteOrder`.
- 100 % de cuentas reciben un `SymbolRegistrationResult` en < 150 ms tras el handshake, con estatus por símbolo (`ACCEPTED | WARNING | REJECTED`).
- Persistir y exponer en telemetría los resultados de registro (`echo.core.handshake.status_total{status}`) para al menos 30 días.
- Mantener la latencia E2E p95 ≤ 500 ms; la validación extra en Agent/Core no puede añadir más de 5 ms p95 por handshake ni bloquear el flujo de órdenes aceptadas.
- Cobertura ≥ 90 % en los paquetes nuevos (`core/internal/protocol`, `sdk/domain/handshake`) y ≥ 85 % en el resto de los cambios.

## Alcance

### Dentro de alcance

- Versionamiento del handshake EA→Agent (`protocol_version`, `client_semver`, `capabilities`, `features`).
- Negociación de rango de protocolo en Agent y Core con configuración centralizada vía ETCD.
- Respuesta `SymbolRegistrationResult` Core→Agent→EA con evaluación por símbolo, almacenamiento en Postgres y telemetría.
- Bloqueo preventivo de cuentas/símbolos rechazados: el Core no emite `ExecuteOrder` ni `CloseOrder` hacia cuentas sin `SymbolRegistrationResult` exitoso.
- Observabilidad completa (logs JSON, métricas, spans) para handshake v2 y feedback.
- Documentación y guías para actualizar EAs, Agent y Core coordinadamente.

### Fuera de alcance

- Cambios en tolerancias de trade o sizing (cobertura de i6+).
- Nuevos campos en `SymbolSpecReport` o lógica de Money Management.
- Paneles gráficos; sólo telemetría y logs programáticos.
- Soporte a protocolos adicionales (REST, WebSocket) o despliegue SaaS.

## Arquitectura de solución

### SDK (`github.com/xKoRx/echo/sdk`)

1. **Proto (`sdk/proto/v1/agent.proto`)**

   ```proto
   enum SymbolRegistrationStatus {
     SYMBOL_REGISTRATION_STATUS_UNSPECIFIED = 0;
     SYMBOL_REGISTRATION_STATUS_ACCEPTED = 1;
     SYMBOL_REGISTRATION_STATUS_WARNING = 2;
     SYMBOL_REGISTRATION_STATUS_REJECTED = 3;
   }

   enum SymbolRegistrationIssueCode {
     SYMBOL_REGISTRATION_ISSUE_CODE_UNSPECIFIED = 0;
     SYMBOL_REGISTRATION_ISSUE_CODE_CANONICAL_NOT_ALLOWED = 1;
     SYMBOL_REGISTRATION_ISSUE_CODE_SPEC_STALE = 2;
     SYMBOL_REGISTRATION_ISSUE_CODE_RISK_POLICY_MISSING = 3;
     SYMBOL_REGISTRATION_ISSUE_CODE_PROTOCOL_VERSION_UNSUPPORTED = 4;
     SYMBOL_REGISTRATION_ISSUE_CODE_FEATURE_MISSING = 5;
   }

   message SymbolRegistrationIssue {
     SymbolRegistrationIssueCode code = 1;
     string message = 2;              // descripción legible
     map<string, string> metadata = 3; // datos adicionales (ej: canonical_symbol, etc.)
   }

   message SymbolRegistrationEntry {
     string canonical_symbol = 1;
     string broker_symbol = 2;
     SymbolRegistrationStatus status = 3;
     repeated SymbolRegistrationIssue warnings = 4;
     repeated SymbolRegistrationIssue errors = 5;
     int64 spec_age_ms = 6;
     int64 evaluated_at_ms = 7;
   }

   message SymbolRegistrationResult {
     string account_id = 1;
     string pipe_role = 2;        // "master" | "slave"
     int32 protocol_version = 3;
     string agent_id = 4;
     string core_version = 5;
     SymbolRegistrationStatus status = 6; // global (peor caso)
     repeated SymbolRegistrationIssue errors = 7; // errores generales (p.ej., version mismatch)
     repeated SymbolRegistrationIssue warnings = 8; // advertencias globales (p.ej., feature opcional ausente)
     repeated SymbolRegistrationEntry symbols = 9;
     int64 evaluated_at_ms = 10;
     string evaluation_id = 11;   // UUIDv7 para correlación
   }

   message HandshakeMetadata {
     int32 protocol_version = 1;
     string client_semver = 2;
     repeated string required_features = 3;
     repeated string optional_features = 4;
     HandshakeCapabilities capabilities = 5;
   }

   message HandshakeCapabilities {
     repeated string features = 1; // ej: "spec_report/v1", "quotes/250ms"
     repeated string metrics = 2;  // ej: "exposure_v1"
   }
   ```

   - Añadir `SymbolRegistrationResult` al `oneof payload` de `CoreMessage` (campo 15 libre).
   - Registrar los nuevos enums `SymbolRegistrationStatus` y `SymbolRegistrationIssueCode` reutilizables en SDK y Core.
   - Extender `AccountSymbolsReport` con un nuevo campo `HandshakeMetadata metadata = 4;` (reservando número libre) para transportar versión, semver y capacidades negociadas.

2. **Paquete nuevo `sdk/domain/handshake`**

   - `const ProtocolVersionV2 = 2` y helpers `Supports(features []string, feature string) bool`.
   - Función `NormalizeHandshakePayload(raw map[string]interface{}) (*Handshake, error)` con validaciones estrictas.
   - Estructuras `Handshake`, `CapabilitySet`, `VersionRange` compartidas por Agent y Core.
   - Enumerado centralizado `IssueCode` alineado con `SymbolRegistrationIssueCode` para evitar strings sueltos.

3. **Telemetría**

   - Nuevos contadores `RecordHandshakeVersion(ctx, version int, status string)` y `RecordSymbolRegistration(ctx, status string, canonical string)` expuestos en `metricbundle.EchoMetrics`.
   - Atributos obligatorios: `account_id`, `pipe_role`, `protocol_version`, `client_semver`.

### Core (`core/internal`)

1. **Nuevo paquete `protocol`**

   - Responsabilidad: cargar desde ETCD (`core/protocol/min_version`, `core/protocol/max_version`, `core/protocol/blocked_versions[]`), validar handshakes y emitir errores estructurados.
   - API pública: `ValidateHandshake(ctx, handshake *handshake.Handshake) (*Evaluation, error)`.
   - `Evaluation` incluye lista de símbolos con resultado junto con errores y warnings globales.
   - Reglas:
     - Versiones fuera de rango → `SymbolRegistrationStatus_REJECTED` global con issue `protocol_version_unsupported`.
     - Versiones dentro del rango pero con features faltantes obligatorias (`spec_report/v1`) → `WARNING` global.
     - `client_semver` debe cumplir patrón SemVer (`MAJOR.MINOR.PATCH`).

2. **Handler de `AccountSymbolsReport`**

   - Después de persistir el reporte, invocar `protocol.ValidateHandshake` consumiendo `report.Metadata`.
   - Reconciliar contra catálogo y políticas:
     - Si `canonical_symbol` no está en catálogo → entry `REJECTED` con issue `canonical_not_allowed`.
     - Si spec vencida (`SymbolSpecService.IsStale`) → issue `spec_stale` (warning).
     - Si falta `RiskPolicy` → issue `risk_policy_missing` (error). Se reaprovecha `RiskPolicyService` de i4.
   - Generar `SymbolRegistrationResult` y enviarlo vía `coreStreamer.Send(&pb.CoreMessage{Payload: &pb.CoreMessage_SymbolRegistrationResult{...}})`.
   - Persistir el resultado global y los detalles por símbolo (ver sección DB) usando `evaluation_id` compartido.
   - Registrar decisión en telemetría (log INFO/ERROR + span `core.handshake.evaluate`).
   - Actualizar caches internas: sólo `status=REJECTED` bloquea el enrutamiento; `WARNING` mantiene el flujo activo con observabilidad reforzada.

3. **Router y guardas**

   - Al recibir `TradeIntent`, verificar `handshakeRegistry.Get(accountID)`:
     - `ACCEPTED` → continuar.
     - `WARNING` → continuar pero agregar atributos de riesgo (`handshake_status=warning`) y emitir métrica `echo.core.handshake.warning_total`.
     - `REJECTED` → rechazar con `ERROR_CODE_SPEC_MISSING` y issue `handshake_not_ready`.
   - `handshakeRegistry` se alimenta del `SymbolRegistrationResult` persistido y del canal de `NOTIFY`.

4. **Observabilidad en Core**

   - Logs JSON `feature=Handshake`, `event=Evaluate`, `status=ACCEPTED|WARNING|REJECTED`.
   - Métricas nuevas: `echo.core.handshake.version_total{version}`, `echo.core.handshake.status_total{status}`, `echo.core.handshake.symbol_issues_total{issue}`.
   - Spans: `core.handshake.evaluate`, `core.handshake.persist`.

### Agent (`agent/internal`)

1. **Parsing handshake v2**

   - Extender `handleHandshake` para buscar `payload.protocol_version`, `payload.client_semver`, `payload.capabilities`.
   - Si falta `protocol_version`, interpretar como v1 y registrar WARN `status=LEGACY`; aplicar política desde ETCD (`agent/protocol/allow_legacy`).
   - Usar `sdk/domain/handshake.NormalizeHandshakePayload` para convertir a struct y propagar `HandshakeMetadata` en `AccountSymbolsReport.Metadata`.

2. **Reenvío de feedback**

   - Extender `handleCoreMessage` para casos `*pb.CoreMessage_SymbolRegistrationResult`.
   - Serializar a JSON lineal y enviarlo al EA por el pipe actual:
     ```json
     {
       "type": "symbol_registration_result",
       "timestamp_ms": 1730851200456,
       "payload": { ...SymbolRegistrationResult... }
     }
     ```
   - Logs `feature=Handshake`, `event=FeedbackForwarded`, `status`.
   - Métricas `echo.agent.handshake.forwarded_total{status}`.

3. **Bloqueos tempranos**

   - Antes de reenviar `ExecuteOrder` al EA, verificar `handshakeRegistry` local (sincronizado con Core).
     - `ACCEPTED`: flujo normal.
     - `WARNING`: permitir y adjuntar atributos adicionales (`handshake_status=warning`, `warning_codes=[...]`) para observabilidad.
     - `REJECTED`: log ERROR, descartar mensaje y emitir métrica `echo.agent.handshake.blocked_total`.

4. **Configuración**

   - Nuevo bloque `protocol` en `agent/internal/config.go` (`MinVersion`, `MaxVersion`, `AllowLegacy`), cargado desde ETCD y validado contra Core.

### Slave EA (`clients/mt4/slave.mq4`)

1. **Handshake v2**

   - `SendHandshake()` envía payload:
     ```json
     {
       "type":"handshake",
       "timestamp_ms":1730851200000,
       "payload":{
         "role":"slave",
         "account_id":"123456",
         "protocol_version":2,
         "client_semver":"1.5.0",
         "terminal_build":1415,
         "capabilities":{"features":["spec_report/v1","quotes/250ms"],"metrics":["exposure_v1"]},
         "symbols":[...]
       }
     }
     ```
   - `client_semver` se obtiene de constantes del EA.
   - `protocol_version` definido en macro `#define PROTOCOL_VERSION 2`.

2. **Consumo de feedback**

   - Nuevo handler `HandleSymbolRegistrationResult(string jsonLine)` que:
     - Parsear `payload.status`.
     - Loggear resultados (`INFO` si `ACCEPTED`, `WARN` si `WARNING`, `ERROR` si `REJECTED`).
     - Marcar símbolos rechazados en arrays locales (`g_SymbolEnabled[i] = false`) para no aceptar órdenes.
     - Si estado global `REJECTED`, detener operaciones (`DisableTrading()`), escribir log `ERROR` y mostrar alerta en terminal (MessageBox opcional configuración `DebugAlerts`).
   - Telemetría: `LogJSON` con `feature=Handshake`, `event=Result`, `status`.

3. **Observabilidad**

   - Incluir atributos: `protocol_version`, `client_semver`, `evaluation_id`.
   - Métrica local `echo.slave.handshake.status{status}` (enviada vía telemetry SDK C bridge).

### Master EA (`clients/mt4/master.mq4`)

- Mismas adiciones de handshake v2 (`role":"master"`).
- Feedback: si algún símbolo queda `REJECTED`, se loguea y se evita emitir intents para ese símbolo.
- Publicar telemetría `echo.master.handshake.status{status}`.

### Base de datos (PostgreSQL)

```sql
CREATE TABLE IF NOT EXISTS echo.account_symbol_registration_eval (
  evaluation_id      UUID        PRIMARY KEY,
  account_id         TEXT        NOT NULL,
  pipe_role          TEXT        NOT NULL,
  status             TEXT        NOT NULL,
  protocol_version   INT         NOT NULL,
  client_semver      TEXT        NOT NULL,
  global_errors      JSONB       NOT NULL DEFAULT '[]'::jsonb,
  global_warnings    JSONB       NOT NULL DEFAULT '[]'::jsonb,
  evaluated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS echo.account_symbol_registration (
  evaluation_id      UUID        NOT NULL REFERENCES echo.account_symbol_registration_eval(evaluation_id) ON DELETE CASCADE,
  canonical_symbol   TEXT        NOT NULL,
  broker_symbol      TEXT        NOT NULL,
  status             TEXT        NOT NULL,
  warnings           JSONB       NOT NULL DEFAULT '[]'::jsonb,
  errors             JSONB       NOT NULL DEFAULT '[]'::jsonb,
  spec_age_ms        BIGINT      NOT NULL,
  evaluated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (evaluation_id, canonical_symbol)
);

CREATE INDEX IF NOT EXISTS idx_account_symbol_registration_account
  ON echo.account_symbol_registration_eval (account_id, evaluated_at DESC);
```

- Trigger `NOTIFY echo_handshake_result` para invalidar caches en Agent/Core cuando se inserten nuevos registros.
- Listener `core/internal/handshake/listener.go` consume el canal, actualiza `handshakeRegistry` y expone métrica `echo.core.handshake.notify_miss_total`.
- Tabla se purga mediante job semanal (retener 30 días) usando `DELETE FROM ... WHERE evaluated_at < NOW()-INTERVAL '30 days'` en ambas tablas dentro de transacción.

### Configuración (ETCD)

```
/echo/
  /core/
    /protocol/
      min_version          → default 1 (permitir despliegue gradual)
      max_version          → default 2
      blocked_versions     → lista opcional (ej: [0])
      required_features    → ej: ["spec_report/v1","quotes/250ms"]
  /agent/
    /protocol/
      allow_legacy         → true durante rollout, false post-cutover
```

- `core/internal/config.go` y `agent/internal/config.go` validan coherencia (min ≤ max, blocked ∉ [min,max]).
- Nueva clave `/core/protocol/retry_interval_ms` para controlar la frecuencia de re-evaluaciones automáticas (ver sección siguiente).

### Re-evaluación automática

- `RiskPolicyService` y `SymbolSpecService` publican eventos en `core/internal/events` cada vez que detectan cambios (watch ETCD o `LISTEN/NOTIFY`).
- Nuevo worker `core/internal/handshake/reconciler` escucha estos eventos y programa re-evaluaciones por cuenta:
  - Consolidación en cola `workqueue` con de-dup para evitar tormentas.
  - Re-evaluación dispara `EvaluateHandshake` reutilizando el último `AccountSymbolsReport` persistido.
  - Resultados actualizados generan nuevo `SymbolRegistrationResult`, insertan registros en tablas y envían feedback al Agent.
- Comando operativo `cmd/echo-core-cli handshake evaluate --account <id>` permite triggers manuales (útil para pipelines de operaciones).

### Observabilidad

- **Logs JSON**: `app=echo-core`, `feature=Handshake`, `event=Evaluate|FeedbackForwarded|ResultConsumed`, `status`, `protocol_version`, `client_semver`, `account_id`.
- **Métricas OTEL**:
  - `echo.handshake.version_gauge{component,version}`
  - `echo.handshake.feedback_latency_ms` (histograma, p95 < 150 ms).
  - `echo.handshake.rejected_total{issue}` y `echo.handshake.warning_total{issue,scope="global|symbol"}`.
- **Trazas**: spans encadenados `core.handshake.evaluate` → `core.handshake.persist` → `core.handshake.feedback.send`; Agent crea `agent.handshake.forward`, EA registra `ea.handshake.consume`.

## Flujo actualizado

1. EA (master/slave) envía handshake v2.
2. Agent parsea, valida versión mínima; si ok, genera `AccountSymbolsReport` + metadata y envía al Core.
3. Core persiste símbolos, valida specs/políticas y construye `SymbolRegistrationResult`.
4. Core actualiza caches, graba resultado en Postgres y envía `SymbolRegistrationResult` por gRPC.
5. Agent recibe el mensaje, lo loggea, lo escribe en el pipe y actualiza su cache de `handshakeRegistry`.
6. EA consume el feedback, habilita o bloquea símbolos localmente y loggea el estado. Si `REJECTED`, no procesa intents/commands.
7. Métricas y spans se emiten en cada hop.

## Compatibilidad y rollout escalonado

- **Fase 1**: Desplegar SDK + Agent + Core con soporte v2, manteniendo `min_version=1`, `allow_legacy=true`. EAs antiguos siguen operando, pero reciben WARN `status=LEGACY`.
- **Fase 2**: Actualizar EAs master/slave con handshake v2.
- **Fase 3**: Ajustar ETCD a `min_version=2`, `allow_legacy=false`. Cuentas que sigan enviando v1 quedan rechazadas con feedback explícito.
- **Rollback**: revertir config `min_version=1`, reinstalar binarios previos si es necesario. La tabla `account_symbol_registration` conserva historial para auditorías.

## Plan de pruebas

- Unitarias SDK: validación de SemVer, features obligatorias, construcción de proto.
- Unitarias Core: `protocol.ValidateHandshake`, `handshakeRegistry`, persistencia en Postgres mockeada.
- Unitarias Agent: parsing handshake v2 y serialización de feedback.
- Unitarias EA: parser MQL4 para `symbol_registration_result`, deshabilitación de símbolos.
- Integración end-to-end:
  - Caso feliz (todas las validaciones OK).
  - Canonical desconocido → `REJECTED`.
  - Spec vencida → `WARNING`.
  - Version mismatch → `REJECTED` global.
- Reconciliación automática: simular alta de política de riesgo después de rechazo y validar que la re-evaluación reactive la cuenta.
- Global warnings: simular `FEATURE_MISSING` y verificar que se persiste en `global_warnings`, métrica `warning_total` aumenta y Agent mantiene la cuenta operativa.
- Smoke tests post-deploy: handshake simultáneo de 10 cuentas, ver métricas en Prometheus/Jaeger.

## Checklist de implementación

- ✅ SDK actualizado (proto, dominio, telemetría).
- ✅ Core genera y persiste `SymbolRegistrationResult`.
- ✅ Agent negocia versión y reenvía feedback.
- ✅ Master/Slave EA soportan handshake v2 y consumen feedback.
- ✅ Migraciones aplicadas (`account_symbol_registration`).
- ✅ Configuración ETCD cargada (`core/protocol/*`, `agent/protocol/*`).
- ⏳ Pruebas integrales ejecutadas con evidencia adjunta.
- ⏳ Monitoreo 24 h tras rollout y ajuste de `min_version` a 2.
- ⏳ Listener y reconciliador desplegados con alertas verdes.

## Riesgos y mitigaciones

| Riesgo | Impacto | Mitigación |
|--------|---------|------------|
| EAs legacy sin actualizar | Bloqueo de trading en cuentas antiguas | Mantener `min_version=1` hasta completar rollout, alertas en dashboards. |
| Feedback no llega al EA | Falta de evidencia para operadores | Retries en Agent (3 intentos con backoff exponencial). Alertar `echo.agent.handshake.forward_error_total`. |
| Esquema DB creciendo sin control | Storage excesivo | Job semanal para purgar evaluaciones >30 días (`DELETE WHERE evaluated_at < NOW()-30d`). |
| Divergencia de configuración ETCD | Rechazos falsos | Validación cruzada en arranque (Agent compara con `core.protocol` vía RPC `Ping` extendido). |

## Referencias

- `docs/00-contexto-general.md`
- `docs/01-arquitectura-y-roadmap.md`
- `docs/rfcs/RFC-architecture.md`
- `docs/rfcs/RFC-004d-iteracion-4-especificaciones-broker.md`
- `agent/internal/pipe_manager.go`
- `core/internal/config.go`
- `sdk/proto/v1/agent.proto`


