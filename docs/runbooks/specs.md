# Runbook — Guardián de volumen lanza `spec_stale`

1. **Leer edad reportada**
   - El log indica `spec_age_ms`. Divide por 1000 para pasar a segundos.
   - Si la edad es enorme (e.g. `1.7e12` ms → ~55 años), se trata de un envejecimiento artificial.

2. **Revisar configuración actual**
   - `core/specs/max_age_ms`: límite para considerar “stale”. Default 10_000 ms.
   - `core/specs/alert_threshold_ms`: sólo alerta, no rechaza.

3. **Traer timestamp real**
   - En SQL: `SELECT account_id, canonical_symbol, reported_at_ms FROM echo.account_symbol_spec ORDER BY updated_at DESC LIMIT 10;`
   - Verifica que `reported_at_ms` crezca; si no, el Agent no está enviando reportes nuevos.

4. **Soluciones**
   - Si la edad es absurda, override temporal: `core/specs/max_age_ms = 180000` (3 minutos) y reinicia Core/Agent.
   - Si no hay reportes frescos, revisar Agent/Slave EA (que exista `symbol_spec_report`).
   - Si los reportes existen pero el Core no actualiza, revisar trigger o caché: `SymbolSpecService`.

5. **Restaurar configuración**
   - Una vez normalizado el flujo de specs, vuelve `max_age_ms` al valor original para evitar degradar controles.

