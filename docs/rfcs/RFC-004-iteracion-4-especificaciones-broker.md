---
title: "RFC-004d: Iteración 4 — Especificaciones de broker y clamps de volumen"
version: "1.1"
date: "2025-11-04"
status: "Implementado"
owner: "Arquitectura Echo"
iteration: "4"
depends_on:
  - "docs/00-contexto-general.md"
  - "docs/01-arquitectura-y-roadmap.md"
  - "docs/rfcs/RFC-004-iteracion-3-catalogo-simbolos.md"
  - "docs/rfcs/RFC-004c-iteracion-3-parte-final-slave-registro.md"
---

## Resumen ejecutivo

Iteración 4 consolida la ingesta de especificaciones del broker y las convierte en guardas operativas dentro del Core. El objetivo es que cada `ExecuteOrder` cumpla con `min_lot`, `max_lot`, `lot_step` y `stop_level` antes de tocar el broker, eliminando rechazos por volumen inválido y reduciendo la latencia derivada de reintentos. La solución introduce un guardián de volumen desacoplado, amplía la telemetría para detectar specs obsoletas y mantiene el pipeline modular entre Slave EA → Agent → Core → SDK. Además deja lista la infraestructura para políticas de riesgo por cuenta × estrategia, habilitando iteraciones posteriores de Money Management (riesgo fijo, cartera diversificada, etc.) sin refactors masivos. Resultado: copiador world-class con tolerancia cero a tamaños inválidos y con base sólida para escalar gerenciamiento de riesgo.

## Contexto y motivación

Iteración 3 ya habilitó el reporte y la persistencia de especificaciones; sin embargo, el Core continúa utilizando un `lot_size` fijo configurado en `core.config.DefaultLotSize`, sin validar si respeta las cotas del broker ni aprovechando que el sizing es realmente por cuenta × estrategia:

```392:401:core/internal/router.go
// Opciones de transformación con target slave
opts := &domain.TransformOptions{
	LotSize:   r.core.config.DefaultLotSize, // TODO i0: hardcoded 0.10
	CommandID: commandID,
	ClientID:  fmt.Sprintf("slave_%s", slaveAccountID), // Issue #C5
	AccountID: slaveAccountID,                          // Issue #C5
}
```

Esto genera rechazos `INVALID_VOLUME` cuando el default cae fuera del rango permitido o no es múltiplo de `lot_step`, y obliga a hardcodear lotes por cuenta en vez de respetar la configuración de riesgo. Hoy ya contamos con un servicio que valida y almacena especificaciones:

```40:116:core/internal/symbol_spec_service.go
func (s *SymbolSpecService) Upsert(ctx context.Context, report *pb.SymbolSpecReport) error {
	if err := domain.ValidateSymbolSpecReport(report, s.whitelist); err != nil {
		s.telemetry.Warn(ctx, "SymbolSpecReport validation failed",
			attribute.String("account_id", report.GetAccountId()),
			attribute.String("error", err.Error()),
			attribute.Int("symbols_count", len(report.GetSymbols())),
		)
		return err
	}
	// ... existing code ...
	if s.repo != nil {
		if err := s.repo.UpsertSpecifications(ctx, accountID, report.Symbols, reportedAt); err != nil {
			s.telemetry.Warn(ctx, "Failed to persist symbol specifications",
				attribute.String("account_id", accountID),
				attribute.String("error", err.Error()),
			)
			return err
		}
	}
	return nil
}
```

La tabla `account_symbol_spec` ya almacena el payload completo:

```3:11:deploy/postgres/migrations/i3_symbol_specs_quotes.sql
CREATE TABLE IF NOT EXISTS echo.account_symbol_spec (
	account_id        TEXT    NOT NULL,
	canonical_symbol  TEXT    NOT NULL,
	broker_symbol     TEXT    NOT NULL,
	payload           JSONB   NOT NULL,
	reported_at_ms    BIGINT  NOT NULL,
	updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (account_id, canonical_symbol)
);
```

El gap es que toda esa información no se usa aún para decidir el tamaño de las órdenes ni para bloquear especificaciones antiguas; tampoco existe un registro canónico de políticas de riesgo por cuenta × estrategia. Esta iteración zanja ambos diferenciales y deja la cancha servida para los motores de MM, alineando el flujo con los principios de robustez, SOLID y telemetría exhaustiva.

## Objetivos medibles (Iteración 4)

- 0 rechazos `ERR_INVALID_VOLUME` o `ERR_INVALID_STOPS` generados por órdenes emitidas desde el Core en ambientes piloto (ventana ≥ 48 h).
- 100 % de `ExecuteOrder` emitidos con lotes que cumplen `min_volume ≤ lot ≤ max_volume` y `lot % volume_step = 0`.
- Alertar en <2 s cuando una cuenta opera con especificaciones ausentes o obsoletas (reportes vencidos > TTL configurado).
- Mantener latencia E2E p95 ≤ 500 ms; el guardián de volumen no puede añadir más de 2 ms en promedio por orden.

## Alcance

### Dentro de alcance
- Guardián de volumen desacoplado en el Core que clampa o rechaza órdenes según specs vigentes.
- Telemetría de ciclo completo: recepción de specs, clamps realizados, rechazos, edad de los reportes.
- Validación temprana en Agent/SDK para negar specs incompletas y proteger la caché.
- Configuración declarativa (ETCD) de políticas de fallback y TTL de especificaciones.
- Registro duradero de políticas de riesgo por cuenta × estrategia (`risk_type`, parámetros) con cache local en Core.
- Interfaces de dominio (`RiskPolicyService`) que el Core consume ya en esta iteración; si la política `FixedLot` no existe se rechaza la orden con `ERR_RISK_POLICY_MISSING`.
- Optimización de persistencia e índices de `account_symbol_spec`.

### Fuera de alcance
- Cálculo de riesgo fijo (Iteración 6) y Money Management avanzado.
- Offsets o modificación post-fill de SL/TP (Iteraciones 7a/7b).
- Cambios en el handshake del Master EA o en protocolos distintos a MT4/MT5.
- Normalización de códigos de error (Iteración 11).
- Ajustes en concurrencia/backpressure del Router (Iteración 12).

## Arquitectura de solución

### SDK (`github.com/xKoRx/echo/sdk`)
- Nuevo helper `domain.ClampLotSize(spec *pb.VolumeSpec, lot float64) (float64, error)` que valida contra `min/max/step`, retorna tamaño clamped y error semántico.
- Estructura `domain.VolumeGuardPolicy` con bandera `OnMissingSpec` (único valor permitido `"reject"`) y `MaxSpecAge`.
- Nuevas métricas de SDK `telemetry` para exponer clamps y rechazos (`EchoMetrics.RecordVolumeClamp`).
- Nuevos contratos `domain.RiskPolicy`, `domain.RiskPolicyRepository` y `domain.RiskPolicyService` que abstraen la obtención de políticas por cuenta × estrategia. Para i4 sólo se admite `RiskTypeFIXEDLOT`, representado por `FixedLotConfig{LotSize float64}`; el SDK valida el esquema al deserializar. `RiskPolicyService` expone `Get(ctx, accountID, strategyID)` y cachea en memoria con TTL configurable.

### Slave EA (`clients/mt4/slave.mq4`)
- Validar y loggear cada lectura de `MODE_MINLOT`, `MODE_MAXLOT`, `MODE_LOTSTEP` y `MODE_STOPLEVEL`. Si el broker entrega 0 o valores negativos, se excluye el símbolo y se emite `WARN`.
- Adjuntar `server_time_ms` derivado de `TimeCurrent()` y transmitir `terminal_boot_id` (UUID generado al iniciar el EA) para medir staleness de forma determinística.
- Añadir atributo `feature=SlaveSpecs` y `event=report` en los logs JSON, manteniendo el formato obligatorio (timestamp RFC3339, level, message, file_path, line_nro, app, feature, event, trace_id opcional, metadata).

### Agent (`agent/internal`)
- Reutilizar `handleSymbolSpecReport` y extenderlo con:
  - Validación pre-gRPC via `domain.ValidateSymbolSpecification`.
  - Detección de reportes repetidos (`reported_at_ms` ≤ último valor cacheado localmente) para filtrar ruido.
  - Atributo `spec_age_ms` en logs para facilitar alertas.
- Publicar métricas: `echo.agent.specs.forwarded_total`, `echo.agent.specs.filtered_stale_total`.
- Añadir passthrough para `trade_intent` con `strategy_id` normalizado (si el master aún no lo envía, se usa fallback `default` sólo para trazas y se exige política registrada antes de operar), preparando el consumo del `RiskPolicyService`.

### Core (`core/internal`)
- Nuevo paquete `volumeguard` con interfaz `Guard.Execute(ctx, accountID, canonical, requestedLot) (lot float64, decision Decision, err error)` donde `Decision` ∈ {`Clamp`, `Reject`, `PassThrough`}.
- `Router.createExecuteOrders` invoca el guardián antes de construir `TransformOptions`. El guardián usa:
  - `SymbolSpecService.GetSpecification(accountID, canonical)` para snapshot actual.
  - Política ante spec faltante: siempre `Reject` con `ERR_SPEC_MISSING`; no existen clamps sin datos confiables.
  - Métricas `echo.core.volume_guard_decision_total{decision}`, `echo.core.volume_guard_spec_age_ms`.
- `RiskPolicyService` (nuevo) obtiene políticas desde persistencia (PostgreSQL) y expone API thread-safe con cache local (LRU + TTL 5s + invalidación inmediata vía `LISTEN/NOTIFY`). Para iteración 4 devuelve políticas `FIXED_LOT` (valor `FixedLotConfig.LotSize`); si no existe registro, rechaza la orden con `ERR_RISK_POLICY_MISSING` y genera métrica `risk_policy_missing`. Toda cuenta habilitada para copiar debe registrar su política antes de operar.
- `Router` incorpora `riskPolicy := riskPolicyService.Get(accountID, strategyID)` al armar las opciones de transformación. Por ahora sólo registra la política en telemetría y usa su `lot_size` nominal como input del guardián, dejando lista la integración para riesgo fijo real en i6.
- `SymbolSpecService` agrega `GetVolumeSpec` y método `IsStale(accountID, canonical, maxAge)` para detectar specs vencidas.
- Ajuste de `SymbolQuoteService` para que `StopLevel` se valide sólo con spec vigente; si está fuera de TTL se rechaza la orden con `ERR_SPEC_MISSING`.
- Integración en telemetría: spans `core.volume_guard` anidados dentro de `core.create_execute_orders`.

### Configuración (ETCD)
Nuevas claves (todas bajo `core/specs/`):
- `core/specs/missing_policy` (`reject` obligatorio; cualquier otro valor provoca error de arranque).
- `core/specs/default_lot` (reemplaza `core/default_lot_size`; este último se eliminará en i5 mediante migración documentada).
- `core/specs/max_age_ms` (valor sugerido: 10 000).
- `core/specs/alert_threshold_ms`
- `core/risk/missing_policy` (`reject`; determina qué hacer si falta política de riesgo; valores distintos fallan el boot).
- `core/risk/cache_ttl_ms` (TTL máximo para la caché del `RiskPolicyService`; default 5000 ms).

## Deployment

1. Aplicar migraciones `i4_symbol_spec_guard.sql` en staging (verificar migraciones previas).
2. Aplicar migración `i4_risk_policy.sql` y poblar con políticas `FIXED_LOT` actuales (map account × strategy → lot_size) para todas las cuentas habilitadas.
3. Actualizar configuración en ETCD (`core/specs/*`, `core/risk/*`) con `missing_policy=reject`, `risk/missing_policy=reject` y TTL recomendado.
4. Desplegar SDK → Agent → Core respetando orden (SDK primero para evitar incompatibilidades).
5. Ejecutar suite de smoke tests con cuentas piloto (trades BUY/SELL en símbolos con `lot_step` distintos y estrategias múltiples).
6. Monitorear métricas y logs por 24 h; si no hay rechazos inesperados ni `risk_policy_missing`, promover a ambientes productivos.

## Checklist

- ✅ Guardián de volumen implementado y cableado en `Router`.
- ✅ Configuración de ETCD documentada y cargada en `core/internal/config.go`.
- ✅ Métricas, logs y spans habilitados para specs, decisiones de volumen y políticas de riesgo.
- ✅ Migraciones aplicadas e índices verificados (`account_symbol_spec`, `account_strategy_risk_policy`).
- ⏳ Documentación actualizada para EA, Agent y Core.
- ⏳ Evidencia de pruebas unitarias e integrales adjunta antes del rollout.
- Recomendación: inicializar atributos comunes (`AppendCommonAttrs`) en `core/cmd/echo-core/main.go` dentro de `bootstrapTelemetry` para evitar repetir metadata en clamps y decisiones de riesgo.

## Notas de implementación (i4)

- Se incorporó el paquete `core/internal/volumeguard` con decisiones `clamp/reject/pass_through`, telemetría estandarizada (`echo.core.volume_guard_*`) y clamps basados en `domain.ClampLotSize`.
- El `Router` ahora obtiene políticas `FIXED_LOT` vía el nuevo `RiskPolicyService`, rechaza órdenes sin política vigente (`ERROR_CODE_RISK_POLICY_MISSING`) y propaga `strategy_id` hasta los Slaves.
- `RiskPolicyService` cachea resultados con TTL, escucha `LISTEN/NOTIFY` desde Postgres y expone métricas `echo.core.risk_policy_lookup_total`.
- `SymbolSpecService` expone `GetVolumeSpec`, edad de reportes y helpers `IsStale/SpecAge` usados por el guardián y la lógica de stops.
- El Agent filtra `SymbolSpecReport` duplicados/obsoletos, valida contra la whitelist y publica métricas `echo.agent.specs.forwarded_total` y `echo.agent.specs.filtered_total`.
- `sdk/telemetry` amplió el bundle Echo con contadores/histogramas nuevos y los semánticos necesarios (`decision`, `policy_type`, etc.).
- `sdk/domain/transformers` ahora preserva `strategy_id` end-to-end; `domain/validation` se limpió para evitar redundancias.
- `core/internal/config.go` consume claves ETCD `core/specs/*` y `core/risk/*`, validando defaults y fallback legacy.
- Nuevas migraciones: `i4_symbol_spec_guard.sql` agrega índices sobre specs y `i4_risk_policy.sql` crea la tabla `account_strategy_risk_policy` con trigger de invalidación.
- El seed de ETCD (`TestSeedEchoConfig_Development`) carga las nuevas llaves para specs, políticas de riesgo y canonical symbols.
- El Master EA ahora adjunta `strategy_id` en cada `trade_intent` (convención `magic_<magic_number>`), manteniendo Core y Agent agnósticos a la derivación de estrategias.