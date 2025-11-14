**Esquema de errores (código, http, mensaje, remedio, retriable):**
- `E-SIZ-001` 400 `policy_missing` — Revisar `account_strategy_risk_policy`. retriable=false
- `E-SIZ-002` 422 `offset_out_of_bounds` — Ajustar offsets a límites permitidos. retriable=false
- `E-ORD-001` 503 `broker_unreachable` — Retry con backoff. retriable=true
- `E-ORD-002` 502 `broker_timeout` — Retry idempotente. retriable=true

