# Plan de implementación i8ab — sl-tp-offset-strategy

## 0. Resumen
- Objetivo: habilitar offsets configurables para SL/TP y su telemetría asociada, reduciendo rechazos `INVALID_STOPS` mediante un fallback determinista en Core.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#2-contexto-y-motivación]
- Alcance: migración `account_strategy_risk_policy`, ampliación de `RiskPolicy`/repositorio, cálculo de offsets en el router, telemetría (logs/metrics/spans) y reintento automático con offsets en 0 tras `ERROR_CODE_INVALID_STOPS`.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#4-alcance][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales]
- Exclusiones: no se modifica Agent, pipes ni los EAs; `ModifyOrder` queda fuera hasta i8b y no se tocan flags/ETCD nuevos.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#42-fuera-de-alcance]

## 1. Plan de commits por pasos
- Paso 1 → schema + dominio
  - Paths: `echo/deploy/postgres/migrations`, `echo/sdk/domain/...`, `echo/core/internal/repository`.
  - Cambios: nueva migración `i8ab_risk_policy_offsets.sql`, campos `StopLossOffsetPips/TakeProfitOffsetPips` (y `RiskTier`) en `domain.RiskPolicy`, lectura/parsing en el repo Postgres, defaults BWC (0).
  - Riesgo: bajo; sólo agrega columnas con default y parsing defensivo.
  - Rollback: revertir migración (down script) y recompilar módulo si fuera necesario.
- Paso 2 → router + fallback
  - Paths: `echo/core/internal/router.go`, `echo/core/internal/risk_policy_service.go` (si requiere exponer metadata), `echo/sdk/telemetry/semconv` (nuevos atributos si aplica).
  - Cambios: helpers para pip size/segment, cálculo de offsets, span `core.stop_offset.compute`, logs `stop_offset_*`, métricas, y reintento `ExecuteOrder` con offsets 0 + clamp mínimo; almacenamiento de contexto para correlacionar fallback.
  - Riesgo: medio; toca hot-path de órdenes y manejo de concurrencia del router.
  - Rollback: feature flag no disponible; la reversión sería mediante revert del commit y limpieza de context maps.
- Paso 3 → métricas + pruebas
  - Paths: `echo/sdk/telemetry/metricbundle/echo.go`, `echo/core/internal/router*_test.go`, `echo/core/internal/risk_policy_service_test.go`.
  - Cambios: nuevos instrumentos `stop_offset_*`, métodos `Record*`, casos table-driven para offsets/fallback, y validación de parsing.
  - Riesgo: medio-bajo; requiere mantener compatibilidad con métricas existentes y cumplir cobertura.
  - Rollback: revertir commit y borrar métricas/ tests añadidos.

## 2. Cambios de código y artefactos
- Paquetes tocados:
  - `echo/sdk/domain`: nuevo estado en `RiskPolicy`, helpers para risk tier/offset defaults.
  - `echo/core/internal/repository`: SELECT ampliado y parsing seguro cuando columnas no existen (entornos legacy).
  - `echo/core/internal/router`: cálculo de offsets en pips, contexto por `command_id`, fallback al segundo intento, telemetría integral.
  - `echo/sdk/telemetry/metricbundle`: creación y registro de contadores/histogramas `stop_offset_*`.
- Migraciones DB: `echo/deploy/postgres/migrations/i8ab_risk_policy_offsets.sql` (up/down con default 0 y trigger existente).[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#62-modelo-de-datos-y-migración]
- Flags/Config: sin ETCD nuevo; offsets se gestionan exclusivamente vía Postgres (PK cuenta×estrategia).[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#63-configuración-flags-y-parámetros]
- Contratos: `RiskPolicy` amplía struct pero mantiene compatibilidad binaria; `account_strategy_risk_policy` sigue usando la PK actual, por lo que no hay cambios en APIs públicas.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#6-contratos-datos-y-persistencia]

## 3. Instrumentación
- Métricas:
  - `echo.core.stop_offset_applied_total{type,segment,result}` para SL/TP (counter).
  - `echo.core.stop_offset_distance_pips{type,segment}` (histogram con buckets RFC).
  - `echo.core.stop_offset_edge_rejections_total{reason,segment}` para clamps por StopLevel.
  - `echo.core.stop_offset_fallback_total{stage,result,segment}` para el degradado.
- Spans:
  - `core.stop_offset.compute` (hijo de `core.router.execute_order`), atributos: cuenta, estrategia, side, offsets configurados, resultado y stop_level_pips.
  - `core.stop_offset.fallback` alrededor del reintento, con `stage=execute`, `result=*`, `retries=1`.
- Logs:
  - Evento `stop_offset_applied`/`stop_offset_skipped` en formato JSON (telemetry client) con `account_id`, `strategy_id`, `side`, offsets pips, distancias master/final y `stop_level_pips`.
  - Log `stop_offset_fallback` con `command_id_original`, `command_id_retry`, `error_code`, `segment` y `result`.

## 4. Prueba local
- Pre-requisitos: Postgres local con migraciones i1 aplicadas; aplicar nueva migración `psql -f deploy/postgres/migrations/i8ab_risk_policy_offsets.sql`.
- Comandos:
  - Build: `go build ./...`
  - Lint: `golangci-lint run ./...`
  - Unit: `go test ./... -count=1 -race -run 'RiskPolicy|Router'`
  - Integración core (si aplica): `go test ./core/... -count=1 -race`
- Datos de ejemplo: insertar política `INSERT INTO echo.account_strategy_risk_policy (...) VALUES (..., sl_offset_pips=25, tp_offset_pips=-10);` y verificar en logs/metrics que los offsets se reflejan.
- Troubleshooting:
  - Si la migración falla por locks, ejecutar `SELECT pg_terminate_backend(pid)` sobre sesiones inactivas.
  - En fallback, verificar que existe `symbol_quote_snapshot`; de lo contrario se usa precio master (log `No quote snapshot`).

## 5. Prueba en CI
- Jobs afectados: `lint`, `unit-core`, `unit-sdk`, y cualquier job que ejecute migraciones (asegurar que la nueva migración está versionada).
- Variables/secretos: mismos que pipeline actual (no se agregan secretos).
- Cobertura: mantener ≥95 % en core y sdk según `go test -cover ./...`.

## 6. Casos borde cubiertos
- Offset positivo y negativo en BUY/SELL, incluyendo símbolos con `digits < 3`.
- Master sin SL/TP ⇒ offsets ignorados y métricas marcan `result=skipped`.
- StopLevel más grande que la distancia resultante ⇒ clamp al mínimo y registro en `edge_rejections`.
- Ausencia de quotes al aplicar offsets ⇒ se loguea y se continúa con valores del master (sin fallback prematuro).
- Broker responde `ERROR_CODE_INVALID_STOPS` ⇒ reintento inmediato con offsets 0 y `minDistance`, segundo fallo ⇒ métrica `result=failed`, error propagado.
- Segmento no configurado en `config` ⇒ se usa `global` para evitar cardinalidad excesiva.

## 7. Matriz de contratos
- `account_strategy_risk_policy`: nuevas columnas INTEGER `sl_offset_pips` y `tp_offset_pips`, default 0; trigger `trg_risk_policy_changed` intacto ⇒ BWC para consumidores existentes.
- `domain.RiskPolicy`: struct añade offsets y `RiskTier`; métodos existentes siguen devolviendo punteros compatibles.
- gRPC (`pb.ExecuteOrder`): sin cambios contractuales; sólo se ajustan valores de `stop_loss`/`take_profit` antes de enviar.
- Fallback usa el mismo flujo `ExecuteOrder`, por lo que no introduce nuevos mensajes ni estados.

## 8. Política de pruebas — Estado final
- Unit: ⏳ Pendiente
- Integración: ⏳ Pendiente
- E2E/Smoke/Regresión: QA (documentar en runbook)

## 9. Checklist final
- Compila ⏳
- Lints ⏳
- Tests unit ⏳
- Tests integración ⏳
- Respeta PR-* ⏳
- Contratos intactos/BWC ⏳
# Plan de implementación i8ab — sl-tp-offset-strategy

## 0. Resumen
- Objetivo: Incorporar offsets configurables en pips para SL/TP aplicados por el Core antes de emitir ExecuteOrder, con fallback ante INVALID_STOPS limitado a un segundo intento sin offsets.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#21-estado-actual][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales]
- Alcance: Migración Postgres, extensión de `RiskPolicy`, lecturas en repositorio, cálculo de offsets en router, telemetría específica (métricas/logs/spans) y fallback con segundo ExecuteOrder sin offsets.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#4-alcance]
- Exclusiones: Sin cambios en Agent/EAs ni emisión de ModifyOrder; rollout global sin feature flags; catástrofe SL (i9) permanece sin tocar.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#42-fuera-de-alcance]

## 1. Plan de commits por pasos
- Paso 1 → Migración offsets Risk Policy  
  - Paths: `echo/deploy/postgres/migrations/i8ab_risk_policy_offsets.sql`  
  - Cambios: Crear migración up/down que agregue `sl_offset_pips`/`tp_offset_pips` con default 0 y mantenga trigger existente, más script smoke (doc).  
  - Riesgo: Bajo; afecta solo schema compartido.  
  - Rollback: ejecutar sección down tras setear columnas a 0.
- Paso 2 → SDK: dominio + metric bundle  
  - Paths: `echo/sdk/domain/risk_policy.go`, `echo/sdk/telemetry/metricbundle/echo.go`, `echo/sdk/domain/trade.go` (struct tag)  
  - Cambios: Añadir campos offsets/segment a `RiskPolicy`, exponer métodos de métricas nuevas (`stop_offset_*`).  
  - Riesgo: Medio por impacto en dependientes; mitigado con gofmt y pruebas unitarias.
- Paso 3 → Core repo service  
  - Paths: `echo/core/internal/repository/postgres.go`, `echo/core/internal/risk_policy_service.go` (solo wiring)  
  - Cambios: Leer nuevas columnas con fallback legacy (`pq` error 42703), parsear `config.risk_tier`, poblar defaults.  
  - Riesgo: SQL roto si fallback mal implementado; cubrir con prueba manual utilizando BD sin migrar.
- Paso 4 → Router offsets + métricas  
  - Paths: `echo/core/internal/router.go`, `echo/core/internal/router_offsets.go` (nuevo helper)  
  - Cambios: Reordenar pipeline para resolver símbolos antes de `TradeIntentToExecuteOrder`, calcular offsets en pips→precio, instrumentar métricas/logs/spans, propagar `segment`, ajustar `CommandContext`.  
  - Riesgo: Alto (hot path); usar helpers puros y pruebas table-driven (`router_offsets_test.go`) para validar cálculos y clamping.
- Paso 5 → Fallback INVALID_STOPS  
  - Paths: `echo/core/internal/router.go` (handleExecutionResult + helpers)  
  - Cambios: Nuevo flujo: stage attempt1 registra requested + reemite ExecuteOrder con offsets 0; attempt2 registra success/failed y corta. Mantener dedupe y métricas.  
  - Riesgo: Condiciones de carrera/duplicados; mitigar almacenando `CommandContext` con intent/lot/segment y pruebas unitarias del helper.  
  - Rollback: Feature guard (config) no pedido; fallback se puede desactivar dejando offsets en 0 (documentar en runbook) si genera ruido.
- Paso 6 → Tests / lint / docs  
  - Paths: `echo/core/internal/router_offsets_test.go`, `echo/sdk/telemetry/metricbundle/echo_test.go` (si aplica)  
  - Cambios: Casos table-driven para `computePipSize`, offsets, fallback metrics; actualización de pruebas existentes.  
  - Riesgo: Bajo; asegura PR-TEST.

## 2. Cambios de código y artefactos
- Paquetes tocados:  
  - `sdk/domain`: ampliar `RiskPolicy` para offsets + segment y exponer en `TransformOptions`.  
  - `sdk/telemetry/metricbundle`: añadir contadores/histogramas `stop_offset_*` y métodos de registro.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#81-metricas]  
  - `core/internal/repository`: lectura de nuevas columnas y parsing `risk_tier`.  
  - `core/internal/router`: cálculo de offsets, instrumentación, fallback.  
  - Nuevos helpers `router_offsets.go` + tests para aislar lógica pura.
- Migraciones DB: `echo/deploy/postgres/migrations/i8ab_risk_policy_offsets.sql` (up/down, default 0, trigger intacto).[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#62-modelo-de-datos-y-migracion]
- Flags/Config: sin claves nuevas; offsets solo en Postgres. Extraer `config.risk_tier` para métricas `segment`, default `global`.  
- Contratos:  
  - `RiskPolicy` mantiene compatibilidad; campos nuevos opcionales con default 0/`global`.  
  - `CommandContext` extiende metadata (intent, strategy, attempt) sin exponer API pública.  
  - gRPC/proto sin cambios → BWC total.

## 3. Instrumentación
- Métricas (`EchoMetrics`):  
  - `echo_core.stop_offset_applied_total{type, result, segment}`  
  - `echo_core.stop_offset_distance_pips{type, segment}`  
  - `echo_core.stop_offset_edge_rejections_total{reason, segment}`  
  - `echo_core.stop_offset_fallback_total{stage=attempt1|attempt2, result, segment}`  
  - Todos leen `segment` desde `config.risk_tier` y usan baja cardinalidad.  
- Spans:  
  - `core.stop_offset.compute` alrededor del cálculo inicial.  
  - `core.stop_offset.fallback` durante reintentos, con atributos `stage`, `result`, `account_id`, `trade_id`.  
  - Errores reportados con `telemetry.RecordError`.
- Logs JSON (`router`): eventos `stop_offset_applied`, `stop_offset_skipped`, `stop_offset_fallback` con payload mínimo `{account_id,strategy_id,side,sl_offset_pips,tp_offset_pips,distance_master_pips,distance_final_pips,stop_level_pips,result,segment}`.

## 4. Prueba local
- Pre-requisitos:  
  - Postgres local con migraciones up to date (`make migrate`).  
  - Seeds de políticas con offsets ±pips para cuentas de prueba.  
  - Telemetry endpoint habilitado (mismo stack de siempre).  
- Comandos:  
  - build: `go build ./...`  
  - lint: `golangci-lint run`  
  - unit: `go test ./... -count=1 -race`  
  - integración: `go test ./core/... -count=1 -race -run 'Integration'` (si existen etiquetas)  
- Datos de ejemplo:  
  - Inserción `account_strategy_risk_policy` con `sl_offset_pips=15`, `tp_offset_pips=-10`, `config='{"risk_tier":"tier_1"}'`.  
  - Simular quotes con StopLevel bajo para forzar clamp.  
  - Fixture `ExecutionResult` manual via test double para `ERROR_CODE_INVALID_STOPS`.
- Troubleshooting:  
  - Si `router` no aplica offsets → verificar pip_size (logs `stop_offset_skipped`), revisar que `digits/point` existan.  
  - Si fallback no dispara → confirmar que error code exacto sea `ERROR_CODE_INVALID_STOPS`.

## 5. Prueba en CI
- Jobs afectados: `lint`, `unit`, `integration` (core).  
  - Asegurar que nueva migración esté incluida en pipelines de DB (scripts `deploy/postgres`).  
- Variables: mismas que pipeline actual (no secrets nuevos).  
- Artefactos: cobertura debe mantenerse ≥95 % en rutas core (agregar pruebas nuevas para helpers).

## 6. Casos borde cubiertos
- Inputs nulos: política sin offsets, sin SL/TP → resultado `skipped`.  
- Timeouts: ausencia de quotes impide ajuste → se loguea y no aplica offset.  
- Reintentos: primer INVALID_STOPS reemite sin offsets; segundo fallo corta con métrica `failed`.  
- Errores de red: si `sendToAgent` falla en fallback, se registra `result=failed` y no se reintenta infinito.  
- Idempotencia: nuevo comando usa `command_id` distinto pero mismo `trade_id`; dedupe se mantiene.  
- Concurrencia: `CommandContext` extendido con locks existentes (`commandContextMu`).  
- Dependencias externas: falta de columnas → fallback legacy query con offsets=0.

## 7. Matriz de contratos
- `RiskPolicy` (SDK): campos opcionales `StopLossOffsetPips`/`TakeProfitOffsetPips`/`Segment`, default 0/`global`; no rompe consumidores existentes.  
- `Metricbundle.EchoMetrics`: API amplía struct + métodos; binarios deben recrear cliente para nuevo bundle (ya sucede al compilar).  
- Router → Agent: `ExecuteOrder` sin cambios (solo valores SL/TP distintos).  
- Persistencia `account_strategy_risk_policy`: columnas nuevas con default 0; triggers y PK se mantienen.  
- QA: fallback no introduce nuevos mensajes; sólo `ExecuteOrder` repetido con `command_id` distinto documentado.

## 8. Política de pruebas — Estado final
- Unit: verde [ ]  
- Integración: verde [ ]  
- E2E/Smoke/Regresión: a cargo de QA (pendiente) — gaps documentados (validar métricas/latencias en ambiente shared).

## 9. Checklist final
- Compila [ ]  
- Lints [ ]  
- Tests unit [ ]  
- Tests integración [ ]  
- Respeta PR-* [ ]  
- Contratos intactos/BWC [ ]  


