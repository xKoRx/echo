---
title: "RFC-006: Iteración 6 — Sizing con riesgo fijo y stops dinámicos"
version: "1.0"
date: "2025-11-05"
status: "Implementado"
owner: "Arquitectura Echo"
iteration: "6"
depends_on:
  - "docs/00-contexto-general.md"
  - "docs/01-arquitectura-y-roadmap.md"
  - "docs/rfcs/RFC-architecture.md"
  - "docs/rfcs/RFC-004d-iteracion-4-especificaciones-broker.md"
  - "docs/rfcs/RFC-005a-iteracion-5-handshake-versionado.md"
---

## Resumen ejecutivo

La iteración 6 habilita el primer modo de money management profesional en Echo: sizing por riesgo fijo (`FIXED_RISK`) por cuenta × estrategia. El Core calculará el lote óptimo en base al monto de riesgo configurado, la distancia real al stop loss y la microestructura del símbolo (tick size, tick value, contract size). Además, todos los `ExecuteOrder` replicarán los stops/targets trasladados al precio vigente del broker del seguidor usando el último `SymbolQuoteSnapshot`. La solución respeta los principios de modularidad y SOLID: se introduce un `FixedRiskEngine` en el Core, se tipifica el contrato de riesgos en la SDK, y se reutiliza el handshake v2 (ya versiónado en i5) extendiendo las **capabilities** y payloads existentes para transportar `tick_value` y `currency`, con observabilidad end-to-end reforzada. Resultado: cada trade replica el riesgo monetario definido por mesa sin over-shoots, manteniendo latencias bajas y trazabilidad total.

## Contexto y motivación

- **Estado actual (post-i5):** el Core sólo conoce políticas `FIXED_LOT`. El `Router` usa el valor `lot_size` almacenado en `account_strategy_risk_policy` y lo clampa a los límites del broker (`core/internal/router.go`). No existe lógica para dimensionar posiciones según la distancia al stop ni para validar si el stop existe.
- **Gaps identificados:**
  - El mismo valor de lote genera riesgos completamente distintos según distancia SL/TP, haciendo imposible operar múltiples estrategias con distinta volatilidad.
  - No se conoce `tick_value` ni la divisa de la cuenta, por lo que no se puede transformar distancia de precio en dinero.
  - El ajuste de SL/TP ya usa el quote actual (`adjustStopsAndTargets`) pero el sizing no lo incorpora; si el precio del slave se movió, el riesgo efectivo cambia.
  - Falta telemetría de exposición: no sabemos cuánto riesgo monetario enviamos en cada orden ni si el cálculo se desvió por clamps.
- **Objetivo de i6:** garantizar que cada orden enviada por Echo respete un riesgo monetario fijo definido por la mesa para cada pareja cuenta × estrategia, rechazando cualquier trade que no cumpla con los datos mínimos (SL, specs, quotes vigentes).

## Objetivos medibles (Iteración 6)

- 100 % de `ExecuteOrder` emitidos con política `FIXED_RISK` deben registrar `expected_loss` dentro de ±2 % del monto configurado (telemetría `echo.core.risk.expected_loss` con label `risk_currency`).
- 0 órdenes con política `FIXED_RISK` se envían sin `StopLoss` válido; si falta SL el trade se rechaza con `ERROR_CODE_RISK_POLICY_INVALID` y métrica `risk_policy_rejected_total`.
- Latencia adicional del cálculo de riesgo ≤ 2 ms p95 por orden (medida con span `core.risk.calculate`).
- Cobertura ≥ 90 % en el paquete nuevo `core/internal/riskengine` y ≥ 85 % en los cambios de SDK/Core/Agent.

## Alcance

### Dentro de alcance

- Nuevo tipo de política `FIXED_RISK` con contratos y validaciones en SDK.
- Extender las especificaciones reportadas por los EAs con `tick_value` y `account_currency` para convertir distancia de precio en dinero.
- Motor de sizing (`FixedRiskEngine`) en el Core que calcula el lote, valida datos y actualiza métricas.
- Ajuste de SL/TP usando el precio actual del slave (ya existe) pero garantizando coherencia con el lote calculado.
- Migración de esquema y datos para soportar configuraciones tipadas por JSON en `account_strategy_risk_policy`.
- Configuración centralizada en ETCD para tolerancias: edad máxima del quote, tolerancia de error de riesgo y fallback ante falta de datos.
- Observabilidad específica (logs, spans, métricas) para riesgo fijo.
- Herramientas de seed/CLI para registrar y auditar políticas `FIXED_RISK`.

### Fuera de alcance

- Estrategias adicionales (Kelly, ATR, portfolio risk); se documenta como evolución futura.
- Re-apertura/re-entries automáticos tras rechazo; seguirá `omit with comment`.
- Modificación post-fill por StopLevel (planificado en i8b).
- Gestión de stops catastróficos y ventanas operativas (i9+).

## Arquitectura de solución

### Visión general

1. El Master EA envía `TradeIntent` con SL/TP y `strategy_id` (ya disponible).
2. El Slave EA reporta `SymbolSpecReport` enriquecido con `tick_value` y envía quotes cada 250 ms.
3. El Agent valida el payload usando handshake v2 + capabilities (`spec_report/tickvalue`) y forwardea al Core.
4. El `Router` consulta `RiskPolicyService`. Si la política es `FIXED_RISK`, delega el sizing a `FixedRiskEngine`.
5. `FixedRiskEngine` obtiene spec + quote vigentes, calcula la distancia real, convierte a dinero y deriva el lote óptimo. Si falta algún dato se rechaza el trade.
6. El lote calculado pasa por `VolumeGuard` (clamp a min/max/step). Se recalcula el riesgo real; si excede la tolerancia configurada, se rechaza.
7. El `ExecuteOrder` resultante se emite con lote ajustado y SL/TP traducidos al precio actual del slave.
8. Toda la ruta queda instrumentada con métricas y spans específicos.

### SDK (`github.com/xKoRx/echo/sdk`)

- **Proto**
  - `proto/v1/common.proto`: añadir `ERROR_CODE_RISK_POLICY_INVALID = 1003;` y `ERROR_CODE_STOP_REQUIRED = 1004;`.
  - `proto/v1/trade.proto`:
    - `AccountInfo` agrega `string currency = 8;` (deposit currency reportada por el EA).
  - `proto/v1/agent.proto`:
    - `SymbolGeneral` añade `double tick_value = 14;` (valor de un tick en la divisa de la cuenta).
    - Declarar la capability `features += "spec_report/tickvalue"` para agentes que envían `tick_value`, manteniendo `protocol_version = 2`.
- **Domain**
  - `domain/risk_policy.go`: nuevo `RiskPolicyTypeFixedRisk` y estructura `FixedRiskConfig` con campos obligatorios `Amount` y `Currency`, más opcional `MaxLotOverride` (techo específico por política cuando la mesa lo requiera).
  - `domain/risk_policy_validator.go` (nuevo): valida esquema JSON y coerciona mayúsculas en divisas (`^[A-Z]{3}$`).
  - `domain/risk_calculator.go` (nuevo): helper puro `func CalculateLotByRisk(distancePoints, tickValue, riskAmount float64) (lot float64, err error)` que encapsula la fórmula:

    ```go
    riskPerLot := distancePoints * tickValue
    if riskPerLot <= 0 {
        return 0, domain.NewError(domain.ErrInvalidSpec, "risk per lot is zero")
    }
    return riskAmount / riskPerLot, nil
    ```

  - `domain/validation.go`: asegurar `tick_value > 0` cuando la capability `spec_report/tickvalue` esté presente.
- **RiskPolicyRepository**
  - Mapear columna `config JSONB` a structs tipadas, manteniendo compatibilidad con `FIXED_LOT`.
- **Telemetry**
  - `metricbundle/echo.go`: nuevas métricas `RecordFixedRiskCalculation(ctx context.Context, result string, attrs ...)`, `RecordFixedRiskExposure(ctx context.Context, amount float64, attrs ...)` (requiere `risk_currency`) y `RecordRiskPolicyRejected(ctx context.Context, reason string, attrs ...)`.
  - `semconv/echo.go`: atributos `RiskAmount`, `RiskCurrency`, `RiskDecision`, `RiskRejectReason`.

### Master / Slave EA (MQL4/MQL5)

- **Master EA**
  - `trade_intent` debe incluir siempre `stop_loss`; si la estrategia opera sin SL, se documenta como no soportado para `FIXED_RISK`.
- **Slave EA**
  - Handshake JSON (`type="handshake"`) mantiene `protocol_version = 2`; agrega en `features` la capability `spec_report/tickvalue` y `quotes/250ms` para anunciar soporte.
  - `SymbolSpecReport` incluirá `general.tick_value = MarketInfo(symbol, MODE_TICKVALUE)` y `general.margin_currency = AccountCurrency()`.
  - Actualizar logs estructurados para registrar `tick_value` y `currency` por símbolo.
  - El EA seguirá aplicando órdenes a mercado; SL/TP provienen del Core.
  - Hardening multi-activo: `OrderSend`/`OrderClose` consultan `MarketInfo(symbol, MODE_ASK/BID)` del símbolo recibido en el comando y abortan con `price_unavailable` cuando la cotización no está disponible o pertenece a otro instrumento.

### Agent (`echo/agent`)

- Validar `tick_value > 0` en `handleSymbolSpecReport`. Si falta, rechazar el símbolo con `SymbolRegistrationIssueCode_SPEC_STALE`.
- Propagar `account_info.currency` cuando reciba `StateSnapshot`.
- Requerir capability `spec_report/tickvalue`; si falta, el Agent marcá la cuenta con warning y el Core rechazará `FIXED_RISK` por falta de datos.
- Nueva métrica `echo.agent.specs.filtered_total{reason="missing_tick_value"}`.
- Adjuntar atributo `risk_policy` en logs cuando forwardea `TradeIntent`.

### Core (`echo/core`)

#### Paquete `riskengine` (nuevo)

- Estructura principal:

  ```go
  type FixedRiskEngine struct {
      specs    *SymbolSpecService
      quotes   *SymbolQuoteService
      accounts *AccountStateService
      cfg      RiskEngineConfig
      tel      *telemetry.Client
      metrics  *metricbundle.EchoMetrics
  }
  ```

- API pública:

  ```go
  func (e *FixedRiskEngine) ComputeLot(
      ctx context.Context,
      accountID, strategyID, canonical string,
      intent *pb.TradeIntent,
      policy *domain.FixedRiskConfig,
  ) (lot float64, expectedLoss float64, decision Decision, err error)
  ```

- Pasos internos:
  1. Validar inputs: `policy.Amount > 0`, `intent.StopLoss != nil`, `quotes` y `specs` disponibles y no obsoletos.
  2. Recuperar `accountCurrency` desde `AccountStateService`; si falta, usar `cfg.DefaultCurrency` sólo cuando `EnableCurrencyFallback=true` (se registra warning). Si `accountCurrency` difiere de `policy.Currency`, rechazar con `ERROR_CODE_RISK_POLICY_INVALID`.
  3. Obtener `quote` más reciente y verificar `age <= cfg.MaxQuoteAge`.
  4. Calcular `distancePrice := |intent.Price - intent.StopLoss|`.
  5. Normalizar distancia a puntos: `distancePoints := distancePrice / tickSize` (cae back a `point` si `tickSize` no está disponible). Si `distancePoints < cfg.MinDistancePoints` ⇒ `DecisionReject` (imposible dimensionar riesgo).
  6. Calcular lote objetivo vía `CalculateLotByRisk` utilizando `distancePoints` y `tickValue`.
  7. Aplicar override `policy.MaxLotOverride` si existe.
  8. Invocar `volumeGuard.Execute(...)` con el lote objetivo para obtener `lotAdj` y la decisión (Clamp/Reject/PassThrough).
  9. Recalcular pérdida esperada con `lotAdj` (`expectedLoss`).
 10. Validar desviación: si `expectedLoss` excede `policy.Amount * (1 + cfg.MaxRiskDrift)` ⇒ `DecisionReject`.
 11. Registrar métricas (`success`, `reject_reason`, `risk_currency`) y retornar.

- `RiskEngineConfig` proviene de ETCD (ver sección siguiente) e incluye explícitamente `MinDistancePoints` (float64) además de `MaxQuoteAge`, `MaxRiskDrift`, `DefaultCurrency` y `EnableCurrencyFallback`.

#### Integración con Router

- `Router.createExecuteOrders` se actualiza:
  - Lee `policy.Type`. Para `FIXED_LOT` se mantiene comportamiento actual.
  - Para `FIXED_RISK` invoca `FixedRiskEngine`. Según `decision`:
    - `DecisionReject`: log warn, `RecordRiskPolicyRejected(ctx, reason, attrs...)` (alimentando `risk_policy_rejected_total`), `continue`.
    - `DecisionDefer`: log info y colocar en `pending` (reservado para i7, actualmente no se usa).
    - `DecisionProceed`: usar el lote y `expectedLoss` retornados (ya evaluados por el guardián).
  - Propagar `expectedLoss` y `risk_currency` vía atributos en telemetría.

#### AccountStateService (nuevo light)

- Cachea `AccountInfo` por cuenta (balance, equity, currency) para que el motor de riesgo conozca la divisa base.
- Se alimenta desde `handleStateSnapshot` (Agent → Core).

#### Error handling

- Cuando falta SL ⇒ `riskengine` retorna `decision=Reject`, `err=ErrStopRequired`, se publica `ERROR_CODE_STOP_REQUIRED`.
- Cuando falta spec o quote ⇒ `ERROR_CODE_SPEC_MISSING` (ya existente) con issue `risk_policy_missing_data`.

### Persistencia (PostgreSQL)

- Migración `deploy/postgres/migrations/i6_risk_policy_fixed_risk.sql` (fase **expand/migrate**):
  1. `ALTER TABLE echo.account_strategy_risk_policy ADD COLUMN config JSONB NOT NULL DEFAULT '{}'::jsonb;`
  2. Backfill `config` con `{"lot_size": lot_size}` para políticas existentes y actualizar triggers para publicar cambios tanto en columnas legacy como en JSON.
  3. `ALTER TABLE ... ADD COLUMN risk_currency TEXT, ADD COLUMN risk_amount DOUBLE PRECISION;`
  4. `ALTER TABLE ... ADD CONSTRAINT chk_risk_policy_config CHECK (risk_type != 'FIXED_RISK' OR (config ? 'amount' AND config ? 'currency'));`
  5. Trigger `notify_risk_policy_changed` se mantiene (ya dispara invalidación).

Una vez todo el stack opere leyendo `config` (fase **contract** posterior, controlada por feature flag), se ejecutará una migración separada `i6b_drop_lot_size.sql` para eliminar la columna `lot_size` y campos legacy, garantizando rollback seguro.

- Repositorio `postgresRiskPolicyRepo.Get` parsea `config`:

  ```go
  var raw json.RawMessage
  row.Scan(&riskType, &raw, ...)
  switch riskType {
  case domain.RiskPolicyTypeFixedRisk:
      var cfg domain.FixedRiskConfig
      json.Unmarshal(raw, &cfg)
      policy.FixedRisk = &cfg
  }
  ```

### Configuración ETCD

Nuevas llaves bajo `/echo/core/risk/`:

| Clave | Tipo | Default | Descripción |
|-------|------|---------|-------------|
| `quote_max_age_ms` | entero | 750 | Edad máxima permitida para la última cotización usada en el cálculo de riesgo. |
| `min_distance_points` | float | 5 | Distancia mínima (en puntos) para aceptar un SL. |
| `max_risk_drift_pct` | float | 0.02 | Desviación máxima tolerada entre el riesgo esperado y el configurado tras clamps. |
| `reject_on_missing_tick_value` | bool | true | Si `true`, rechaza políticas `FIXED_RISK` cuando no haya tick value. |
| `default_currency` | string | "USD" | Divisa por defecto para validar políticas si el EA aún no reporta currency (opcional transición). |
| `enable_currency_fallback` | bool | false | Si `true`, habilita el uso de `default_currency` cuando aún no llega `account_currency`; genera logs/métricas con `fallback=true`. |

Semilla (`tools/seed_etcd`) actualizará estos valores en environments `dev`, `qa`, `prod`.

### Observabilidad

- **Spans**
  - `core.risk.calculate` (hijo de `core.create_execute_orders`). Atributos: `account_id`, `strategy_id`, `canonical_symbol`, `risk_amount`, `risk_currency`, `decision`.
- **Métricas nuevas**
  - `echo.core.risk.fixed_risk_calculation_total{decision="success|reject|fallback"}`.
  - `echo.core.risk.expected_loss` (histograma) etiquetado con `risk_currency`, `account_id` y `strategy_id` para comparar vs `policy.Amount` en la divisa adecuada.
  - `echo.core.risk.distance_points` (histograma) para monitorear volatilidad replicada.
  - `echo.core.risk.policy_rejected_total{reason, account_id, strategy_id}` para auditar rechazos de políticas.
  - `echo.agent.specs.filtered_total{reason="missing_tick_value"}` (Agent).
- **Logs estructurados**
  - Nivel `Info`: lote calculado, pérdida esperada y tolerancia.
  - Nivel `Warn`: rechazos (indicar `reason`, `missing_stop`, `stale_quote`, `tick_value_missing`).
  - Todos los logs usan `telemetry.Client` (nada de `fmt`).

### Compatibilidad y rollout gradual

- Core sólo permitirá `FIXED_RISK` si:
  1. Handshake declara `protocol_version = 2` (o superior) e incluye la capability `spec_report/tickvalue` junto con `quotes/250ms`.
  2. Existe `SymbolSpec` con `tick_value` y `SymbolQuoteSnapshot` vigente.
  3. Política `FIXED_RISK` registrada en Postgres con currency igual a la reportada por la cuenta.
- Si una cuenta aún opera con EAs legacy, la política debe continuar como `FIXED_LOT` (o se rechaza la orden).
- CLI (`tools/seed_etcd` y `tools/setup`) incluirá comandos:
  - `echo-core-cli risk-policy set-fixed-risk --account <id> --strategy <id> --amount 100 --currency USD`
  - `echo-core-cli risk-policy inspect --account <id>`

## Flujo detallado (pseudocódigo)

1. `Router` recibe `TradeIntent`.
2. Consulta `RiskPolicyService.Get(account, strategy)`.
3. Si `policy.Type == FIXED_RISK` ⇒ `lot, expectedLoss, decision, err := fixedRiskEngine.ComputeLot(...)`.
4. `ComputeLot` valida SL, currency de la cuenta, specs vigentes, quote fresco y calcula el lote objetivo.
5. El motor invoca `volumeGuard.Execute` y retorna lote ajustado + pérdida esperada.
6. Si la decisión es `Reject`, el Router registra métrica/log y finaliza; si es `Proceed`, utiliza el lote retornado.
7. `adjustStopsAndTargets` usa el mismo `quote` para desplazar SL/TP.
8. Se envía orden al Agent y se registran métricas/logs (`expectedLoss`, `risk_currency`).
9. Si cualquier paso falla, se rechaza con código específico y se actualiza `dedupe` a `REJECTED`.

## Ejemplo numérico

- Configuración: `account_strategy_risk_policy` define `FIXED_RISK {amount=100, currency=USD}`.
- Trade del master:
  - `Price = 4000.00`, `StopLoss = 3950.00`, `TakeProfit = 4210.00` (distancia SL = 50.00).
- Especificación del slave:
  - `tick_size = 0.01`, `tick_value = 1.0` USD, `lot_step = 0.01`.
- Quote actual del slave: `Ask = 4010.00` (BUY), `Bid = 4009.80`.
- Cálculo en Core:
  - `distance_price = |4000 - 3950| = 50.00`.
  - `distance_points = 50.00 / 0.01 = 5000`.
  - `riskPerLot = 5000 (distance_points) * 1.0 (tick_value) = 5000` USD por lote.
- Con `cfg.MinDistancePoints = 5`, la validación se cumple porque `distance_points = 5000` ≫ `5`.
  - `lot_target = 100 / 5000 = 0.02`.
  - `VolumeGuard` verifica min/max/step ⇒ resultado 0.02.
  - `expectedLoss = 0.02 * 5000 = 100`. Cumple tolerancia.
  - SL traducido: `4010 - 50 = 3960`; TP traducido: `4010 + 210 = 4220`.
  - Se emite `ExecuteOrder` (`lot_size=0.02`, `stop_loss=3960`, `take_profit=4220`).

## Testing

- **SDK**
  - Tests table-driven para `CalculateLotByRisk` (distancias, slippage, edge cases). Cobertura ≥ 95 %.
  - Validaciones de `FixedRiskConfig` (amount ≤ 0, currency inválida).
- **Core**
  - Tests unitarios de `FixedRiskEngine` (casos: success, falta SL, quote viejo, tick_value = 0, clamps que exceden tolerancia).
  - Tests de integración en `Router` con mocks de `RiskPolicyService`, `SymbolSpecService`, `SymbolQuoteService`.
  - Verificar que `dedupe` se marca `REJECTED` en rechazos.
- **Agent**
  - Tests de parsing de `SymbolSpecReport` con capability `spec_report/tickvalue` dentro de handshake v2.
- **E2E smoke**
  - Escenario: 1 master, 2 slaves con distintos `tick_value`. Afirmar que riesgo se mantiene ±2 % usando datos reales.

## Plan de despliegue

1. **Preparación**: ejecutar en staging la migración de expansión (`i6_risk_policy_fixed_risk.sql`).
2. Actualizar SDK (incluyendo generación de protos) ⇒ relevar a Agent/Core/EAs.
3. Desplegar nuevos EAs (handshake v2) anunciando capability `spec_report/tickvalue` y publicando `tick_value`.
4. Desplegar Agent (valida `tick_value`).
5. Desplegar Core con `FixedRiskEngine` apagado por feature flag (`core/risk/enable_fixed_risk=false`).
6. Cargar políticas `FIXED_RISK` en staging usando CLI; validar telemetría.
7. Encender flag (`enable_fixed_risk=true`) por cuentas piloto.
8. Monitorear métricas 24 h: `expected_loss`, `decision=reject`. Ajustar tolerancias si es necesario.
9. Repetir en producción siguiendo mismo orden.
10. Una vez todos los binarios operen sólo con `config`, ejecutar la migración de contracción (`i6b_drop_lot_size.sql`).

## Checklist de salida

- ⏳ Documentar guía operativa para registrar políticas `FIXED_RISK` (runbook).
- ⏳ Actualizar dashboards con las nuevas métricas (`echo.core.risk.*`).
- ⏳ Evidencia de pruebas (unitarias, integración y smoke) anexada al PR.
- ⏳ Validar que todas las cuentas piloto exponen la capability `spec_report/tickvalue` antes de activar `enable_fixed_risk`.

## Riesgos y mitigaciones

| Riesgo | Impacto | Mitigación |
|--------|---------|------------|
| Quotes desactualizados → riesgo subestimado | Alto | TTL configurable (`quote_max_age_ms`), rechazo automático y alerta `spec_quote_stale`. |
| Brokers sin `tick_value` consistente | Medio | Requerir capability `spec_report/tickvalue`; fallback temporal a `FIXED_LOT` con alerta. |
| Overshoot por clamps a `min_lot` | Medio | Validar desviación con `max_risk_drift_pct`; documentar ajuste manual del monto de riesgo. |

## Referencias

- `docs/00-contexto-general.md`
- `docs/01-arquitectura-y-roadmap.md`
- `docs/rfcs/RFC-architecture.md`
- `docs/rfcs/RFC-004d-iteracion-4-especificaciones-broker.md`
- `docs/rfcs/RFC-005a-iteracion-5-handshake-versionado.md`


## Notas de implementación (2025-11-06)

- **Motor FixedRisk Engine**: se incorporó `core/internal/riskengine` con cálculo de lotes por riesgo, control de drift (`echo.core.risk.fixed_risk_calculation_total`, `echo.core.risk.expected_loss`, `echo.core.risk.distance_points`) y spans `core.risk.calculate`.
- **Servicios auxiliares**: se añadió `AccountStateService` para cachear `StateSnapshot` con currency de cuenta y alimentar el motor.
- **Router y políticas**: `createExecuteOrders` ahora soporta `FIXED_RISK`, documenta motivos de rechazo y mantiene `FIXED_LOT` como fallback.
- **SDK**: nuevos contratos (`FixedRiskConfig`, `CalculateLotByRisk`, `JSONToStateSnapshot`) y validaciones con `SymbolSpecValidationOptions`.
- **Agent**: los EAs deben anunciar `spec_report/tickvalue`; el handler filtra specs sin `tick_value` y reenvía snapshots de estado.
- **Persistencia y migraciones**: se agregó `deploy/postgres/migrations/i6_risk_policy_fixed_risk.sql` para columnas `config`, `risk_currency` y `risk_amount` en `account_strategy_risk_policy`.
- **Configuración ETCD**: se incluyeron nuevas llaves (`core/risk/quote_max_age_ms`, `min_distance_points`, `max_risk_drift_pct`, `default_currency`, `enable_currency_fallback`, `reject_on_missing_tick_value`) y se actualizaron los seeds en `sdk/etcd/echo_seed_test.go`.
- **Builds y pruebas**: `go test` en módulos `sdk`, `core`, `agent` y ejecución de `./build_all.sh` para generar binarios actualizados.

## Resumen operativo y checklist de diagnóstico (2025-11-06)

1. **Componentes nuevos**
   - `FixedRiskEngine` calcula el lote objetivo (`lot_target = riesgo / (distancia_en_puntos × tick_value)`), valida SL obligatorio y verifica `tick_value`, quotes frescos y currency consistente. Expone métricas y logs `INFO` con el motivo de la decisión.
   - `AccountStateService` persiste en memoria el último `StateSnapshot` por cuenta para resolver la divisa reportada por el EA.
   - `RiskPolicyRepository` soporta `config` JSONB y columnas `risk_currency`/`risk_amount` para `FIXED_RISK`.

2. **Flujo de datos**
   - Agent exige handshake con capability `spec_report/tickvalue` y descarta specs sin `tick_value`.
   - Cada `StateSnapshot` actualiza la caché de cuentas (currency/balance) y queda disponible para el motor.
   - El router selecciona la política (`FIXED_LOT` o `FIXED_RISK`), registra el motivo en telemetría y decide si emitir la orden.

3. **Configuración en ETCD** *(namespace `/echo/core/risk/`)*
   - `quote_max_age_ms`, `min_distance_points`, `max_risk_drift_pct`, `default_currency`, `enable_currency_fallback`, `reject_on_missing_tick_value`.
   - Seeds actualizados en `sdk/etcd/echo_seed_test.go`.

4. **SQL de migración**
   - `deploy/postgres/migrations/i6_risk_policy_fixed_risk.sql` agrega `config JSONB`, `risk_currency` y `risk_amount`.

5. **Ideas para diagnosticar “no inserta nuevas órdenes”**
   - Revisar métricas `echo.core.risk.policy_rejected_total` y `echo.core.risk.fixed_risk_calculation_total{result="reject"}`.
   - Confirmar que el EA esclavo anuncia `spec_report/tickvalue` y que los specs incluyen `tick_value > 0`.
   - Validar que el `TradeIntent` venga con `stop_loss`; el motor rechaza sin SL (`ERROR_CODE_STOP_REQUIRED`).
   - Verificar `StateSnapshot`: la cuenta debe reportar `currency` o habilitar `core/risk/enable_currency_fallback=true`.
   - Quotes: `quote_max_age_ms` demasiado bajo puede marcar `quote_stale`. Revisar logs `INFO` del motor y la métrica `echo.core.risk.distance_points`.
   - Consistencia de políticas: `account_strategy_risk_policy` debe tener `risk_type='FIXED_RISK'` y `config` con `amount`/`currency`.

6. **Siguientes pasos sugeridos**
   - Monitorear logs `INFO` recién añadidos en `FixedRiskEngine` (motivo de rechazo, lote calculado, pérdida esperada).
   - Si el flujo sigue bloqueado, capturar muestras de métricas + logs y ejecutar `SELECT` del policy + specs/quotes para validar datos de entrada.


