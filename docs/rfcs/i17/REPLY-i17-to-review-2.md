# REPLY i17 → Review 2

| Hallazgo | Acción | PR-* | Sección corregida / enlace |
|----------|--------|------|----------------------------|
| H5 – GAP-DEV flag `supports_lossless_delivery` indefinido | Cambiar | PR-BWC / PR-MOD | `§6.1 Mensajes / contratos` describe nuevos mensajes `AgentHello`/`CoreHello`, campo `supports_lossless_delivery`, flujo de negociación y efecto sobre el modo compatibilidad. `§10.2` se apoya ahora en ese contrato formal. |
| H6 – GAP-DEV sin config para master_retry | Cambiar | PR-ROB / PR-RES | `§6.3 Configuración` explicita que los Master EA reutilizan `/echo/core/delivery/retry_backoff_ms` (propagado vía AgentHello/DeliveryHeartbeat) y documenta cómo toman cambios. |
| H7 – Métrica compat_mode_total ausente | Cambiar | PR-OBS | `§8.1 Métricas` incluye la fila `echo_core.delivery.compat_mode_total` (counter, labels `agent_id`, `protocol_version`). |

Todos los hallazgos se resolvieron actualizando el RFC; no se requieren justificaciones de “sin cambio”.
