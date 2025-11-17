# i8ab — Estado real de la implementación

## Resumen ejecutivo
- `account_strategy_risk_policy` ahora persiste `sl_offset_pips` y `tp_offset_pips` con default `0`, de modo que los offsets se configuran sólo en Postgres y conservan compatibilidad retroactiva.
- La SDK amplía `domain.RiskPolicy` con ambos offsets y el `risk_tier`; el repositorio Postgres detecta en caliente si la migración existe y devuelve 0 en entornos legacy.
- El router calcula distancias en pips mediante `computeStopOffsetTargets`, las clampa contra el StopLevel del símbolo y actualiza el `ExecuteOrder` antes de ejecutar el ajuste estándar `adjustStopsAndTargets`.
- Se exponen logs, spans y métricas `stop_offset_*` dentro de `sdk/telemetry/metricbundle`, reutilizando el segmento (`risk_tier`) como etiqueta de baja cardinalidad.
- Ante `ERROR_CODE_INVALID_STOPS` el Core reintenta en la misma goroutine un segundo `ExecuteOrder` sin offsets, fuerza la distancia mínima y registra el resultado del fallback.

## Componentes clave

### Persistencia y dominio
La migración `i8ab_risk_policy_offsets.sql` añade las columnas con default 0 y comentarios descriptivos:

```5:11:deploy/postgres/migrations/i8ab_risk_policy_offsets.sql
ALTER TABLE echo.account_strategy_risk_policy
    ADD COLUMN IF NOT EXISTS sl_offset_pips INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tp_offset_pips INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN echo.account_strategy_risk_policy.sl_offset_pips IS 'Offset (en pips) aplicado al StopLoss del slave (puede ser negativo)';
COMMENT ON COLUMN echo.account_strategy_risk_policy.tp_offset_pips IS 'Offset (en pips) aplicado al TakeProfit del slave (puede ser negativo)';
```

La SDK incorpora los campos nuevos y expone `RiskTier`, que se usa más adelante para etiquetar métricas:

```34:49:sdk/domain/risk_policy.go
type RiskPolicy struct {
	AccountID  string
	StrategyID string
	Type       RiskPolicyType
	FixedLot   *FixedLotConfig
	FixedRisk  *FixedRiskConfig
	// StopLossOffsetPips representa el offset configurado en pips para SL (puede ser negativo).
	StopLossOffsetPips int32
	// TakeProfitOffsetPips representa el offset configurado en pips para TP (puede ser negativo).
	TakeProfitOffsetPips int32
	// RiskTier identifica el segmento operativo (global, tier_1, tier_2, tier_3).
	RiskTier   string
	Version    int64
	UpdatedAt  time.Time
	ValidUntil *time.Time
}
```

El repositorio Postgres consulta condicionalmente las nuevas columnas, rellena los offsets si existen y normaliza el `risk_tier` desde `config`:

```940:1036:core/internal/repository/postgres.go
supportsOffsets := r.supportsOffsetColumns(ctx)
query := `
		SELECT risk_type, lot_size, config, risk_currency, risk_amount, version, updated_at, valid_until
		FROM echo.account_strategy_risk_policy
		WHERE account_id = $1 AND strategy_id = $2
	`
if supportsOffsets {
	query = `
			SELECT risk_type, lot_size, config, risk_currency, risk_amount,
			       sl_offset_pips, tp_offset_pips, version, updated_at, valid_until
			FROM echo.account_strategy_risk_policy
			WHERE account_id = $1 AND strategy_id = $2
		`
}
…
if supportsOffsets {
	scanErr = row.Scan(&riskType, &lotSize, &configRaw, &riskCurrency, &riskAmount, &slOffset, &tpOffset, &version, &updatedAt, &validUntil)
} else {
	scanErr = row.Scan(&riskType, &lotSize, &configRaw, &riskCurrency, &riskAmount, &version, &updatedAt, &validUntil)
}
…
if slOffset.Valid {
	policy.StopLossOffsetPips = slOffset.Int32
}
if tpOffset.Valid {
	policy.TakeProfitOffsetPips = tpOffset.Int32
}
policy.RiskTier = normalizeRiskTier(extractRiskTierFromConfig(configRaw.String))
```

### Router: cálculo y telemetría
`computeStopOffsetTargets` deriva las distancias master → SL/TP en pips, suma el offset configurado y clampa contra el StopLevel reportado. Los precios objetivo se recalculan según BUY/SELL:

```2001:2057:core/internal/router.go
stats := &stopOffsetStats{
	Segment:              segment,
	ConfiguredSLPips:     policy.StopLossOffsetPips,
	ConfiguredTPPips:     policy.TakeProfitOffsetPips,
	StopLevelPips:        pricing.StopLevelPips,
	PipSize:              pricing.PipSize,
	SLResult:             "skipped",
	TPResult:             "skipped",
}
…
stats.TargetSLDistancePips = stats.MasterSLDistancePips + float64(policy.StopLossOffsetPips)
if stats.TargetSLDistancePips <= float64(minTargetPips) {
	stats.TargetSLDistancePips = float64(minTargetPips)
	stats.SLResult = "clamped"
}
offsetPrice := stats.TargetSLDistancePips * stats.PipSize
if intent.Side == pb.OrderSide_ORDER_SIDE_BUY {
	slPrice = intent.Price - offsetPrice
} else {
	slPrice = intent.Price + offsetPrice
}
```

`applyStopOffsets` abre un span `core.stop_offset.compute`, actualiza el `ExecuteOrder` y devuelve estadísticas usadas por `emitStopOffsetTelemetry` para logs + métricas:

```2092:2127:core/internal/router.go
_, span := r.core.telemetry.StartSpan(ctx, "core.stop_offset.compute")
span.SetAttributes(
	attribute.String("account_id", accountID),
	attribute.String("segment", segment),
	semconv.Echo.OrderSide.String(orderSideToString(intent.Side)),
	attribute.Int64("sl_offset_pips", int64(policy.StopLossOffsetPips)),
	attribute.Int64("tp_offset_pips", int64(policy.TakeProfitOffsetPips)),
)
…
if slTarget != nil {
	order.StopLoss = proto.Float64(roundToDigits(*slTarget, digits))
}
if tpTarget != nil {
	order.TakeProfit = proto.Float64(roundToDigits(*tpTarget, digits))
}
```

Cuando hay snapshot de precios, `adjustStopsAndTargets` recalcula la distancia final contra el entry price real y marca clamps adicionales; si no hay quote, conserva los valores provenientes del master.

### Telemetría y métricas
El bundle `EchoMetrics` agrega contadores/histogramas dedicados y helpers para usarlos desde el router:

```765:812:sdk/telemetry/metricbundle/echo.go
func (m *EchoMetrics) RecordStopOffsetApplied(ctx context.Context, stopType, segment, result string) {
	attrs := []attribute.KeyValue{
		attribute.String("type", stopType),
		attribute.String("segment", segment),
		attribute.String("result", result),
	}
	m.StopOffsetApplied.Add(ctx, 1, metric.WithAttributes(attrs...))
}
…
func (m *EchoMetrics) RecordStopOffsetFallback(ctx context.Context, stage, result, segment string) {
	attrs := []attribute.KeyValue{
		attribute.String("stage", stage),
		attribute.String("result", result),
		attribute.String("segment", segment),
	}
	m.StopOffsetFallback.Add(ctx, 1, metric.WithAttributes(attrs...))
}
```

### Fallback ante `ERROR_CODE_INVALID_STOPS`
`handleExecutionResult` detecta especificamente `ERROR_CODE_INVALID_STOPS` y delega en `retryExecuteOrderWithoutOffsets`, registrando el estado del fallback:

```1455:1465:core/internal/router.go
needsFallback := !result.Success && result.ErrorCode == pb.ErrorCode_ERROR_CODE_INVALID_STOPS
if needsFallback && cmdCtx != nil {
	if !cmdCtx.FallbackAttempted {
		r.recordStopOffsetFallback(ctx, cmdCtx, fallbackStageAttempt1, fallbackResultRequested, commandID, "")
		if r.retryExecuteOrderWithoutOffsets(ctx, cmdCtx, commandID) {
			return
		}
		r.recordStopOffsetFallback(ctx, cmdCtx, fallbackStageAttempt1, fallbackResultFailed, commandID, "")
	} else if cmdCtx.PendingFallback {
		r.recordStopOffsetFallback(ctx, cmdCtx, fallbackStageAttempt2, fallbackResultFailed, cmdCtx.OriginalCommandID, cmdCtx.FallbackCommandID)
	}
}
```

El reintento clona la última orden, fuerza offsets en 0, recalcula los stops con `forceMinDistanceStops` y reemplaza el `command_id` para mantener la deduplicación:

```2210:2307:core/internal/router.go
fallbackOrder := proto.Clone(cmdCtx.LastOrder).(*pb.ExecuteOrder)
newCommandID := utils.GenerateUUIDv7()
…
if cmdCtx.Intent.StopLoss != nil {
	sl := cmdCtx.Intent.GetStopLoss()
	fallbackOrder.StopLoss = proto.Float64(sl)
} else {
	fallbackOrder.StopLoss = nil
}
…
forceMinDistanceStops(fallbackOrder, cmdCtx.Intent, pricing, quote)
_ = r.adjustStopsAndTargets(ctx, fallbackOrder, cmdCtx.Intent, quote, accountInfo, spec, cmdCtx.SlaveAccountID, pricing, nil)
…
r.core.echoMetrics.RecordStopOffsetFallback(ctx, stage, result, cmdCtx.Segment)
```

## Riesgos y brechas frente al RFC
- `supportsOffsetColumns` se evalúa una sola vez por proceso; si la migración se aplica después del arranque, el Core seguirá asumiendo que no existen columnas hasta reiniciar:

```1179:1194:core/internal/repository/postgres.go
r.detectOffsetsOnce.Do(func() {
	query := `
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'echo'
			  AND table_name = 'account_strategy_risk_policy'
			  AND column_name = 'sl_offset_pips'
			LIMIT 1
		`
	var exists int
	if err := r.db.QueryRowContext(ctx, query).Scan(&exists); err == nil && exists == 1 {
		r.offsetColumns = true
	}
})
```

- El fallback sólo se activa cuando el broker responde exactamente `ERROR_CODE_INVALID_STOPS`; otros rechazos equivalentes (p.ej. `ERROR_CODE_INVALID_PRICE`) no gatillan el reintento.
- Si `symbolQuoteService` no tiene snapshot, `adjustStopsAndTargets` sale antes de recalcular distancias finales, por lo que el offset se aplica usando precios del master; no hay métrica de distancia en esos casos:

```1244:1250:core/internal/router.go
if quote == nil {
	r.core.telemetry.Debug(ctx, "No quote snapshot available for stop adjustment",
		attribute.String("account_id", accountID),
		attribute.String("canonical_symbol", intent.Symbol),
	)
	return outcome
}
```

- El RFC menciona utilizar `TransformOptions.ApplyOffsets`, pero en la práctica el router crea las opciones sin poblar `SLOffset/TPOffset` y altera el `ExecuteOrder` a posteriori:

```1112:1119:core/internal/router.go
opts := &domain.TransformOptions{
	LotSize:   lotSize,
	CommandID: commandID,
	ClientID:  fmt.Sprintf("slave_%s", slaveAccountID),
	AccountID: slaveAccountID,
}
order := domain.TradeIntentToExecuteOrder(intent, opts)
```

- La métrica `echo_core.stop_offset_edge_rejections_total` etiqueta todos los clamps con `reason="stop_level"`, por lo que no distingue violaciones por otras causas potenciales.
- La iteración i8b (Modificar SL/TP post-fill mediante `ModifyOrder`) sigue pendiente; hoy el fallback sólo reenvía `ExecuteOrder`.

## Estado recomendado para los documentos
- Marcar **i8a** como `✅`: offsets y fallback están implementados en Core + SDK y disponen de telemetría dedicada.
- Mantener **i8b** en `🚧`: aún no existe workflow `ModifyOrder` post-fill ni clamps posteriores a la ejecución.

