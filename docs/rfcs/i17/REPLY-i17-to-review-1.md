# REPLY i17 → Review 1

| Hallazgo | Acción | PR-* | Sección corregida / enlace |
|----------|--------|------|----------------------------|
| H1 – GAP-DEV ledger sin especificación | Cambiar | PR-ROB / PR-MOD | `§6.2 Modelo de datos` ahora fija BoltDB como store único, define bucket `acks`, JSON on-disk y política de fsync/retención para `agent_ack_ledger`. |
| H2 – GAP-DEV backoff indefinido | Cambiar | PR-ROB / PR-RES | `§6.3 Configuración` detalla `retry_backoff_ms` como array JSON `[50..25600]`, regla para intentos 11–100 y validaciones de entrada compartidas por Core/Agent/EAs. |
| H3 – PR-BWC sin plan de convivencia | Cambiar | PR-BWC | `§10.2 Backward compatibility` describe negociación `supports_lossless_delivery`, modo compatibilidad temporal, métricas `compat_mode_total` y ventana operativa de 24 h para migrar agentes/EA legacy. |
| H4 – QA sin Given-When-Then | Cambiar | PR-ROB / PR-OBS | `§9.1 Casos E2E` incluye tabla GWT (Given-When-Then) para cada escenario con métricas/logs esperados para QA. |

No se mantienen observaciones MEN, por lo que todos los BLOQ y MAY quedaron cerrados en la iteración.
