---
rfc_id: "RFC-008ab"
title: "Iteración i8ab — SL/TP offset strategy"
version: "1.0"
status: "draft"
owner_arch: "Arquitectura Echo"
owner_dev: "Echo Core"
owner_qa: "Echo QA"
date: "2025-11-15"
iteration: "i8ab"
type: "feature"
depends_on:
  - "docs/00-contexto-general.md"
  - "docs/01-arquitectura-y-roadmap.md"
  - "docs/rfcs/RFC-architecture.md"
  - "vibe-coding/prompts/common-principles.md"
related_rfcs:
  - "docs/rfcs/[DEPRECATED] RFC-i8(a+b)-sl-tp-offset-stop-level.md"
tags:
  - "core"
  - "sdk"
  - "agent"
---

# RFC-008ab: Iteración i8ab — SL/TP offset strategy

---

## 1. Resumen ejecutivo

- El router actualmente sólo recalcula SL/TP usando el quote reciente y el StopLevel reportado por el broker, sin permitir offsets configurables por estrategia.[echo/core/internal/router.go#adjuststopsandtargets]
- El roadmap de Echo exige que la iteración i8(a+b) agregue offsets SL/TP y lógica StopLevel-aware para reducir rechazos, requisito aún pendiente en V1.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]
- `account_strategy_risk_policy` es la fuente única de políticas por cuenta×estrategia; la tabla todavía no almacena offsets, impidiendo controlar la distancia Master→Slave al copiar SL/TP.[echo/deploy/postgres/migrations/i4_risk_policy.sql#account_strategy_risk_policy]
- Este RFC define un MVP para configurar offsets en pips, aplicarlos en el Core antes de enviar `ExecuteOrder` y exponer telemetría dedicada, manteniendo el SLO intra-host <100 ms p95.[echo/docs/00-contexto-general.md#que-hace-unico-a-echo-vision-de-clase-mundial]

---

## 2. Contexto y motivación

### 2.1 Estado actual

- El Core convierte `TradeIntent` en `ExecuteOrder` reutilizando los niveles SL/TP del master y sólo les aplica ajustes derivados de quotes y StopLevel si existe espec vigente; no existe forma de sumar o restar pips definidos por estrategia.[echo/core/internal/router.go#adjuststopsandtargets]
- `TransformOptions` soporta offsets (`SLOffset`, `TPOffset`, `ApplyOffsets`), pero ningún módulo asigna valores distintos de cero ni activa la bandera, por lo que los niveles enviados al Agent son idénticos a los del master.[echo/sdk/domain/transformers.go#tradeintenttoexecuteorder]
- El repositorio de políticas lee `account_strategy_risk_policy` para obtener `FIXED_LOT` o `FIXED_RISK`, sin campos adicionales más allá de `lot_size`, `config` y metadatos, por lo que no hay almacenamiento estructurado para offsets.[echo/core/internal/repository/postgres.go#postgresRiskPolicyRepo.Get]

### 2.2 Problema / gaps

- Las diferencias de precio y StopLevel entre brokers generan rechazos por `INVALID_STOPS` o distancias desalineadas con la tolerancia de cada estrategia, afectando la trazabilidad y la tasa de copias exitosas.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]
- V1 compromete SL/TP opcionales con offset y validación de StopLevel (iteración i8), pero no existe ningún contrato vigente que permita configurar o medir estos offsets.[echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1]
- Sin telemetría dedicada es imposible auditar cuánto offset se aplicó ni correlacionar rechazos con configuraciones específicas, debilitando los principios PR-ROB y PR-OBS.[echo/vibe-coding/prompts/common-principles.md#pr-obs]

### 2.3 Objetivo de la iteración

Implementar un MVP que:
1. Permita configurar offsets SL/TP en pips por cuenta×estrategia dentro de `account_strategy_risk_policy`.
2. Convierta esos offsets a precios en el Core considerando BUY/SELL y StopLevel antes de enviar `ExecuteOrder`.
3. Exponga observabilidad (logs, métricas, spans) para validar la efectividad de los offsets.

Con ello se cumple el alcance previsto para i8(a+b) sin alterar otros módulos ni introducir flags adicionales.[echo/docs/rfcs/RFC-architecture.md#5-2-logica-v1-segun-prd]

---

## 3. Objetivos medibles (Definition of Done)

- 0 rechazos adicionales por `INVALID_STOPS` cuando la distancia configurada sea alcanzable; se monitorea la métrica `echo_core.stop_offset_edge_rejections_total` y debe permanecer <0.3 % de las órdenes procesadas.[echo/docs/00-contexto-general.md#metricas-clave-y-slo]
- Fallback efectivo documentado: cuando el broker devuelve `ERROR_CODE_INVALID_STOPS`, `echo_core.stop_offset_fallback_total{result=success}` debe alcanzar ≥95 % de los casos en testing, garantizando la degradación exigida para i8.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]
- Offset aplicado registrado en logs estructurados para 100 % de las órdenes con SL/TP, incluyendo distancia original y final.[echo/docs/rfcs/RFC-architecture.md#7-5-observabilidad]
- Latencia añadida por el cálculo de offsets <2 ms p95 dentro del span `core.stop_offset.compute`, garantizando que el pipeline del router mantenga el SLO intra-host <100 ms.[echo/docs/00-contexto-general.md#que-hace-unico-a-echo-vision-de-clase-mundial]
- Riesgo de backfill: todas las filas existentes en `account_strategy_risk_policy` deben contener `0` como offset tras la migración; verificado mediante script de smoke test.

---

## 4. Alcance

### 4.1 Dentro de alcance

- Migración PostgreSQL `i8ab_risk_policy_offsets.sql` para agregar `sl_offset_pips` y `tp_offset_pips` (INTEGER, default 0) en `account_strategy_risk_policy`.
- Extensión de `domain.RiskPolicy`, repositorio Postgres y `RiskPolicyService` para exponer los nuevos campos sin afectar caché ni invalidaciones.
- Ajustes en `Router` y `TransformOptions` para convertir offsets en pips a precios, considerando BUY/SELL y StopLevel.
- Observabilidad dedicada (logs JSON, métricas, spans) que describa offsets aplicados o clampados.

### 4.2 Fuera de alcance

- Reintentos inteligentes o colas de modificación post-fill; continúan como iteraciones futuras.[echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1]
- Cambios en Agent o EAs; el contrato `ExecuteOrder` ya soporta SL/TP estándar.[echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto]
- Feature flags o rollout gradual: estamos en entorno de testing y el feature se habilita globalmente.

---

## 5. Arquitectura de solución

### 5.1 Visión general

1. `RiskPolicyService` lee `account_strategy_risk_policy`, cachea offsets y los entrega junto al resto de la política.
2. El router calcula la distancia master→SL/TP en pips, suma el offset configurado, convierte el resultado a precio usando el `point`/`digits` del símbolo y aplica clamps por StopLevel.
3. `TransformOptions` activa `ApplyOffsets` para que `TradeIntentToExecuteOrder` serialice los nuevos niveles antes de enviar `ExecuteOrder` por gRPC.[echo/docs/rfcs/RFC-architecture.md#5-1-flujo-de-datos]

### 5.2 Componentes afectados

| Componente | Tipo de cambio | BWC | Notas |
|------------|----------------|-----|-------|
| `echo/deploy/postgres/migrations` | Nueva migración `i8ab_risk_policy_offsets.sql` | Sí | Default 0 mantiene comportamiento previo.[echo/deploy/postgres/migrations/i4_risk_policy.sql#account_strategy_risk_policy] |
| `echo/core/internal/repository/postgres.go` | Lectura de offsets en `postgresRiskPolicyRepo` | Sí | Devuelve 0 si DB no define columnas (entornos legacy). |
| `echo/core/internal/risk_policy_service.go` | Cache y métricas incluyen offsets | Sí | Listener mantiene invalidaciones por cuenta×estrategia. |
| `echo/core/internal/router.go` | Nueva etapa `applyOffsetsInPips` antes de `adjustStopsAndTargets` | Sí | Si offset=0 no hay cambios. |
| `echo/sdk/domain/risk_policy.go` | `RiskPolicy` amplía struct | Sí | Campos opcionales, no afectan consumidores actuales. |
| `echo/sdk/domain/transformers.go` | `TradeIntentToExecuteOrder` recibe offsets aplicables | Sí | Reutiliza API existente. |

### 5.3 Flujos principales

1. **Carga de política**  
   - Core solicita política al servicio, que la obtiene desde Postgres con `sl_offset_pips` y `tp_offset_pips` en la misma consulta que `risk_type`.[echo/core/internal/repository/postgres.go#postgresRiskPolicyRepo.Get]  
   - Los offsets se cachean por `account::strategy` y generan métricas `risk_policy_lookup{result=hit|miss}` como hasta ahora.

2. **Construcción de órdenes**  
   - Router calcula la distancia original en pips: `distance_pips = (master_price - stop_price) / pip_size` para BUY (negativa para SELL) y la convierte en valor absoluto.  
   - Offset configurado (positivo o negativo) se suma algebraicamente: `final_distance_pips = distance_pips + sl_offset_pips`.  
   - El resultado se multiplica por `pip_size` para obtener distancia en precio; se clamp a `minDistance` derivado de `stop_level` y `point` antes de llamar a `computeStopPrice`.[echo/core/internal/router.go#adjuststopsandtargets]
   - `TransformOptions` recibe `ApplyOffsets=true`, `SLOffset` / `TPOffset` en precio para que `TradeIntentToExecuteOrder` seteé los valores finales enviados al Agent.[echo/sdk/domain/transformers.go#tradeintenttoexecuteorder]

3. **Observabilidad**  
   - Cada orden produce logs JSON y métricas: se registran offset configurado, distancia original/enviada y resultado `applied|clamped|skipped`.

4. **Fallback ante `INVALID_STOPS`**  
   - Si el broker responde `ERROR_CODE_INVALID_STOPS` luego de aplicar clamps, el router ejecuta un degradado determinista en la misma goroutine del `command_id`:[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]
     1. Marca el primer rechazo con `result=fallback_requested`, incrementa `echo_core.stop_offset_fallback_total{stage="attempt1",result="requested"}` y registra atributos `fallback_stage=attempt1` en el span.  
     2. Reemite un `ExecuteOrder` con el mismo `trade_id` pero **nuevo** `command_id` para preservar dedupe; los offsets se fuerzan a 0 y la distancia se fija al mínimo permitido (`minDistance`), garantizando cumplimiento del StopLevel.  
     3. Si el segundo `ExecuteOrder` se llena, el fallback termina (no se emiten `ModifyOrder`); se incrementa `echo_core.stop_offset_fallback_total{stage="attempt2",result="success"}` y se etiqueta `fallback_result=success` en el span.  
     4. Tras dos intentos fallidos, se propaga `ERROR_CODE_INVALID_STOPS`, se marca `result="failed"` (`stage="attempt2"`) y se genera alerta operativa.  
   - Todo el flujo queda cubierto por logs (`stop_offset_fallback`), métricas agregadas y spans `core.stop_offset.fallback`, habilitando QA para verificar el KPI de rechazos.

### 5.4 Reglas de negocio y casos borde

- Configuración en pips (no points):  
  - `pip_size` se define como `point × pip_multiplier`, donde `pip_multiplier = 10` si `digits >= 3`, de lo contrario `1`.  
  - Para símbolos con `tick_size` distinto y `digits < 3`, se usa `max(point, tick_size)` como base.
- BUY vs SELL:  
  - BUY: SL se aleja restando distancia positiva; offsets positivos aumentan la distancia (SL más lejos), negativos la reducen.  
  - SELL: SL se aleja sumando distancia; offsets positivos alejan, negativos acercan.  
  - TP aplica la lógica inversa (BUY: suma; SELL: resta).
- StopLevel: si `final_distance_price < minDistance`, se eleva al mínimo permitido y se etiqueta `result=clamped`.  
- Falta de SL/TP en el master → no se genera offset (se conserva SL catastrófico si aplica).  
- Valores nulos o columnas ausentes → se interpretan como 0 para no bloquear entornos heredados.
- Fallback: sólo se activa cuando el broker devuelve `INVALID_STOPS` tras clamp; el segundo intento envía offsets en 0 y la distancia mínima permitida. Si el broker vuelve a rechazar, se registra fallo definitivo; los `ModifyOrder` post-fill quedan para iteraciones futuras.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]

---

## 6. Contratos, datos y persistencia

### 6.1 Mensajes / contratos

- `domain.RiskPolicy` añade campos:  
  ```go
  type RiskPolicy struct {
      ...
      StopLossOffsetPips int32
      TakeProfitOffsetPips int32
  }
  ```  
  Los valores son signed int32, almacenados en pips.  
- `RiskPolicyRepository.Get` debe mapear NULL→0 y exponer errores si la fila existe pero el tipo de política es incompatible.  
- `RiskPolicyService` agrega getters auxiliares para recuperar offsets manteniendo la interfaz pública (no se amplía la interfaz, sólo la estructura devuelta).  
- `TransformOptions` ya incluye `SLOffset`, `TPOffset`, `ApplyOffsets`; el Core los poblará con distancias en precio tras aplicar offsets en pips.[echo/sdk/domain/transformers.go#tradeintenttoexecuteorder]

### 6.2 Modelo de datos y migración

- Nueva migración `deploy/postgres/migrations/i8ab_risk_policy_offsets.sql`:

| Columna | Tipo | Default | Semántica |
|---------|------|---------|-----------|
| `sl_offset_pips` | INTEGER NOT NULL | 0 | Offset configurado para StopLoss en pips (puede ser negativo). |
| `tp_offset_pips` | INTEGER NOT NULL | 0 | Offset configurado para TakeProfit en pips. |

- La PK `(account_id, strategy_id)` permanece; no se requieren índices nuevos.  
- El trigger `trg_risk_policy_changed` continúa notificando cambios, por lo que no hay trabajo adicional de invalidación.[echo/deploy/postgres/migrations/i4_risk_policy.sql#account_strategy_risk_policy]
- Down migration elimina ambas columnas para entornos que necesiten rollback (no se espera en testing).

### 6.3 Configuración, flags y parámetros

- Queda estrictamente prohibido crear claves en ETCD para `sl_offset_pips`/`tp_offset_pips`; la única fuente válida es PostgreSQL `account_strategy_risk_policy` (PK `account_id`, `strategy_id`).[echo/docs/rfcs/RFC-architecture.md#2-principios-arquitectonicos]
- Offsets se gestionan mediante scripts SQL/CLI que escriben en `account_strategy_risk_policy`; se documentará en runbooks.
- No hay feature flags ni toggles: el feature se considera parte del baseline de testing como solicitó el owner.

---

## 7. Principios de diseño y trade-offs

- **PR-MOD**: se mantiene `account_strategy_risk_policy` como única fuente para parametrizar cuentas×estrategias, evitando configuraciones duplicadas.[echo/vibe-coding/prompts/common-principles.md#pr-mod]
- **PR-ROB**: offsets positivos ayudan a absorber StopLevel altos; los clamps documentados previenen violaciones cuando los brokers cambian reglas sin aviso.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]
- **PR-OBS**: métricas y spans específicos entregan visibilidad del impacto del feature.[echo/vibe-coding/prompts/common-principles.md#pr-obs]
- **Trade-off**: se introduce lógica adicional en el hot path del router (pequeño costo CPU), aceptado porque evita rechazos que hoy obligan a intervención manual. No se agrega flag para simplificar el MVP, siguiendo la instrucción del owner.

---

## 8. Observabilidad (logs, métricas, trazas)

### 8.1 Métricas

- `echo_core.stop_offset_applied_total` (counter)  
  - labels: `type=sl|tp`, `result=applied|clamped|skipped`, `segment`.  
  - `segment` representa el “tier operacional” de la cuenta (global, tier_1, tier_2, tier_3) y se obtiene del campo `config.risk_tier` de `account_strategy_risk_policy`. Si la política no define `risk_tier`, se asigna `segment=global`. El mismo valor se inyecta en los atributos comunes del contexto para mantener correlación con logs y spans.
  - Incrementa cuando se procesa cada nivel; los IDs detallados se relegan a logs/spans para evitar cardinalidad excesiva.[echo/docs/03-respuesta-a-correcciones.md#riesgos-remanentes-y-mitigacion]
- `echo_core.stop_offset_distance_pips` (histogram)  
  - buckets: 0,1,2,5,10,20,50,100 pips; labels: `type`, `segment` (misma fuente).  
  - Mide distancia final tras offset para análisis agregado.
- `echo_core.stop_offset_edge_rejections_total` (counter)  
  - labels: `reason=stop_level|min_distance`, `segment` (misma fuente).  
  - Rastrea clamps obligatorios.
- `echo_core.stop_offset_fallback_total` (counter)  
  - labels: `stage=attempt1|attempt2`, `result=requested|success|failed`, `segment` (misma fuente).  
  - Permite auditar la degradación descrita en §5.3 y respaldar el KPI ≥95 % de éxito sin depender de `ModifyOrder`.

### 8.2 Logs estructurados

- Logger `router` emite eventos `stop_offset_applied` y `stop_offset_skipped` en JSON `{app, iter, comp, op, corr_id, span_id, msg}` siguiendo la convención del SDK.[echo/docs/rfcs/RFC-architecture.md#7-5-observabilidad]
- Campos adicionales: `account_id`, `strategy_id`, `side`, `sl_offset_pips`, `tp_offset_pips`, `distance_master_pips`, `distance_final_pips`, `stop_level_pips`.

### 8.3 Trazas y spans

- Nuevo span `core.stop_offset.compute` hijo de `core.router.execute_order`, atributos: `account_id`, `strategy_id`, `side`, `offset_sl_pips`, `offset_tp_pips`, `result`.  
- Si se produce un clamp no esperado o desbordes, se llama `telemetry.RecordError` dentro del span.  
- Métricas y logs reutilizan los atributos comunes en contexto, manteniendo correlación completa.
- Cuando se activa el fallback se crea sub-span `core.stop_offset.fallback` con atributos `stage`, `result`, `retries`, vinculando los eventos del punto 5.3 para facilitar el tracing integral.

---

## 9. Plan de pruebas (Dev y QA)

Todos los escenarios se preparan configurando `sl_offset_pips` y `tp_offset_pips` directamente en PostgreSQL dentro de `account_strategy_risk_policy` (PK `account_id` + `strategy_id`). La parametrización es estrictamente por cuenta×estrategia y queda terminantemente prohibido crear claves equivalentes en ETCD.

### 9.1 Casos E2E

| ID | Descripción | Precondiciones | Resultado esperado |
|----|-------------|----------------|--------------------|
| E2E-01 | SL offset positivo aleja StopLoss en BUY | `sl_offset_pips=20`, StopLevel=10 pips | Orden enviada con SL desplazado +20 pips, result=`applied`. |
| E2E-02 | SL offset negativo acerca StopLoss en SELL | `sl_offset_pips=-15`, StopLevel=5 pips | SL se acerca 15 pips sin violar StopLevel, result=`applied`. |
| E2E-03 | Offset provoca violación y se clampa | `sl_offset_pips=-30`, StopLevel=25 pips | Core ajusta al mínimo permitido, result=`clamped`, métrica incrementada. |
| E2E-04 | Sólo TP configurado | `tp_offset_pips=25`, sin SL | Se ajusta únicamente TP, log `stop_offset_skipped` para SL. |
| E2E-05 | Offsets en 0 (BWC) | default 0 | Comportamiento idéntico al actual, sin métricas adicionales. |
| E2E-06 | Fallback ante `INVALID_STOPS` persistente | Simular respuesta del broker con `ERROR_CODE_INVALID_STOPS` tras clamp | Router reemite orden con offset 0, segunda ejecución exitosa y `stop_offset_fallback_total{stage="attempt2",result="success"}` incrementa. |

### 9.2 Pruebas del Dev

- Unit tests table-driven para `computeOffsetDistance` y nueva lógica del router (BUY/SELL × signo offset × StopLevel).  
- Tests de repositorio para verificar lectura/escritura de offsets (mock DB).  
- Smoke test de migración: `SELECT COUNT(*) WHERE sl_offset_pips <> 0` en entorno limpio debe retornar 0 tras aplicar migración.
- Tests específicos para el fallback: simulación de `ExecutionResult` con `ERROR_CODE_INVALID_STOPS` verificando que el router reemite la orden con offset 0 y registra métricas/spans correspondientes.

### 9.3 Pruebas de QA

- Validar en ambiente de testing con cuentas sintéticas que:  
  - Métricas reflejan offsets aplicados.  
  - Logs contienen distancia original/final.  
- Forzar un `INVALID_STOPS` para comprobar que el fallback completa en ≤2 intentos y que la métrica `stop_offset_fallback_total{result="success"}` se incrementa.  
- Pruebas de regresión: replicar una orden con offset 0 y comparar latencias vs baseline (debe mantenerse dentro del SLO).  
- No se requiere prueba de carga adicional porque el feature no introduce IO nuevo.

### 9.4 Datos de prueba

- Seeds SQL para insertar políticas con offsets positivos/negativos.  
- Quotes simulados que permitan verificar clamps (StopLevel variable).  
- Scripts para limpiar offsets (setear 0) tras cada ejecución.  
- Fixture de respuestas `ExecutionResult` con `ERROR_CODE_INVALID_STOPS` para automatizar pruebas del fallback.

---

## 10. Plan de rollout, BWC y operación

- **Despliegue**: aplicar migración `i8ab_risk_policy_offsets.sql`, redeploy del Core, cargar offsets necesarios vía SQL; no se requieren cambios en Agent ni EAs.  
- **BWC**: valores default 0 garantizan igualdad con el comportamiento previo, permitiendo habilitar offsets por estrategia de manera inmediata una vez escritos.  
- **Rollback**: en testing se puede revertir la migración si fuese necesario; antes, setear offsets en 0 para evitar discrepancias.  
- **Operación**: agregar panel en Grafana con histogramas y counters mencionados; runbook debe documentar cómo escribir offsets y verificar métricas.

---

## 11. Riesgos, supuestos y preguntas abiertas

### 11.1 Riesgos

| ID | Descripción | Impacto | Mitigación |
|----|-------------|---------|------------|
| R1 | Offset negativo extremo acerca SL por debajo del StopLevel | MAY | Clamp automático + métrica `edge_rejections` + alerta. |
| R2 | Conversión pips→precio errónea por símbolos exóticos | MAY | Uso de `point`/`tick_size` y pruebas unitarias con distintos `digits`. |
| R3 | Migración no aplicada en alguna base | MEN | Script de verificación incluido en pipeline de testing. |

### 11.2 Supuestos

- Todos los entornos de testing ejecutan migraciones en orden antes de levantar el Core.  
- Los EAs no necesitan modificaciones porque sólo reciben niveles finales ya ajustados.  
- No se requiere feature flag ni rollout parcial (solicitud explícita del owner).

### 11.3 Preguntas abiertas / NEED-INFO

- Ninguna; los requisitos del owner responden al QPACK (offset en pips, sin límites, independientes, sin FF ni rollout parcial).

---

## 12. Trabajo futuro

- Exponer tooling (CLI o UI) para editar offsets sin SQL manual.  
- Integrar dashboards específicos cuando el producto salga de testing.  
- Evaluar combinación con política de SL catastrófico (i9) para escenarios sin SL del master.

---

## 13. Referencias

- `docs/00-contexto-general.md`.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]
- `docs/01-arquitectura-y-roadmap.md`.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]
- `docs/rfcs/RFC-architecture.md`.[echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1]
- `docs/rfcs/[DEPRECATED] RFC-i8(a+b)-sl-tp-offset-stop-level.md`.[echo/docs/rfcs/[DEPRECATED] RFC-i8(a+b)-sl-tp-offset-stop-level.md#configuracion-de-offsets-sl-tp]
- `vibe-coding/prompts/common-principles.md`.[echo/vibe-coding/prompts/common-principles.md#pr-obs]

---

## 14. Matriz PR-*

| PR-* | Evidencia | Impacto |
|------|-----------|---------|
| PR-ROB | Offsets más fallback evitan rechazos permanentes y cumplen i8.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales] | MAY |
| PR-MOD | Config centralizada en `account_strategy_risk_policy`.[echo/deploy/postgres/migrations/i4_risk_policy.sql#account_strategy_risk_policy] | MAY |
| PR-ESC | Lógica se encapsula en router sin dependencias cruzadas.[echo/core/internal/router.go#adjuststopsandtargets] | MEN |
| PR-CLN | Uso de una sola fuente de verdad y funciones pequeñas en router/service.[echo/core/internal/repository/postgres.go#postgresRiskPolicyRepo.Get] | MEN |
| PR-SOLID | DIP mantenido via interfaces `RiskPolicyRepository/Service`.[echo/sdk/domain/risk_policy.go#RiskPolicy] | MEN |
| PR-KISS | Sin flags ni flujos paralelos; offsets aplicados en un solo lugar.[echo/sdk/domain/transformers.go#tradeintenttoexecuteorder] | MEN |
| PR-OBS | Métricas/logs/spans dedicados al feature.[echo/docs/rfcs/RFC-architecture.md#7-5-observabilidad] | MAY |
| PR-BWC | Default 0 asegura compatibilidad total.[echo/deploy/postgres/migrations/i4_risk_policy.sql#account_strategy_risk_policy] | MAY |
| PR-IDEMP | Router sigue usando `command_id`/dedupe sin cambios.[echo/docs/rfcs/RFC-architecture.md#5-1-flujo-de-datos] | MEN |
| PR-RES | Clamp automático ante StopLevel mantiene resiliencia.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion] | MEN |
| PR-SEC | No se exponen datos nuevos ni secretos; se usa Postgres existente.[echo/docs/rfcs/RFC-architecture.md#3-2-componentes-principales] | INFO |
| PR-PERF | Lógica ligera para offsets mantiene latencia <2 ms adicional.[echo/docs/00-contexto-general.md#que-hace-unico-a-echo-vision-de-clase-mundial] | MEN |

---

## 15. Refs cargadas

- echo/docs/00-contexto-general.md — "---"
- echo/docs/01-arquitectura-y-roadmap.md — "---"
- echo/docs/rfcs/RFC-architecture.md — "---"
- echo/vibe-coding/prompts/common-principles.md — "**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."
- echo/docs/templates/rfc.md — "---"
- echo/docs/rfcs/[DEPRECATED] RFC-i8(a+b)-sl-tp-offset-stop-level.md — "# RFC i8(a+b) — sl-tp-offset-stop-level"

---

