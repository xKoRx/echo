**Métricas sugeridas (nombres, tipo, etiquetas):**
- `orders_relay_latency_ms` (histogram) — labels: `symbol`, `account_id`, `strategy`  
- `orders_relay_fail_total` (counter) — labels: `reason`  
- `offset_applied_total` (counter) — labels: `type=sl|tp`, `source=global|strategy|account`  
- `offset_violation_total` (counter) — labels: `constraint`  
- `sizing_compute_latency_ms` (histogram) — labels: `instrument`, `account_id`
- `retry_attempt_total` (counter) — labels: `op`, `status`

**Tracing**:
- Span `sizing.compute` con attrs: `instrument`, `risk_policy_id`, `offset_tp_bps`, `offset_sl_bps`.
- Span `order.dispatch` con `broker`, `account_id`, `latency_ms`.

