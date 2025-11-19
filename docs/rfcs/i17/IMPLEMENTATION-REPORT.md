# IMPLEMENTATION REPORT — i17 end-to-end-lossless-retry

## 1. Resumen de gaps cerrados
- **H1 — Heartbeats gRPC 1–5 s:** `core/internal/config.go` sincroniza los valores `grpc/keepalive/*` con `/echo/core/delivery/heartbeat_interval_ms` (clamp 1–5 s) y `agent/internal/config.go` adopta el mismo intervalo antes de dials gRPC. Así eliminamos los defaults legacy 60 s/20 s.
- **H2 — Pérdida de órdenes al fallar el journal:** `core/internal/delivery_service.go` encapsula `Insert/UpdateStatus/AssignAgent/MarkAcked` bajo `withJournalRetry` (backoff ≤ `max_retries`) y `core/internal/router.go` deja de ignorar errores de `Schedule*`, generando `core_delivery_failed` + `echo_core.delivery.retry_total{delivery_stage="journal"}` cuando no se puede persistir.

## 2. Cambios clave
- Core: enforcement del heartbeat compartido, `startDeliveryHeartbeatLoop` continúa refrescando `/echo/core/delivery/*` y ahora el server gRPC usa los mismos límites. `DeliveryService` reintenta operaciones contra Postgres y propaga fallos irreversibles.
- Agent: `LoadConfig` lee `/echo/core/delivery/heartbeat_interval_ms` para ajustar `grpc/client_keepalive` sin depender de claves legacy.
- Router: al agotar reintentos del journal se detiene el fanout del trade/cierre afectado, emite log JSON estructurado y deja la investigación a NOC (sin pérdidas silenciosas).
- Observabilidad: reutilizamos `echo_core.delivery.retry_total` con `delivery_stage="journal"` y añadimos logs `core_delivery_failed` para alertar de Postgres caído.

## 3. Pruebas locales
| Tipo | Estado | Notas |
|------|--------|-------|
| build core/agent | ok | `go build ./core/...`, `go build ./agent/...` |
| unit core | ok | `cd core && go test ./...` |
| unit agent | ok | `cd agent && go test ./...` |
| lint | fail (conocido) | `/home/kor/go/bin/golangci-lint run` → `typechecking error: pattern ./...: directory prefix . does not contain modules listed in go.work` (golangci-lint aún no soporta el layout multi-módulo + Go 1.25) |
| integración EA/Pipes | pendiente | requiere entorno Windows con Named Pipes reales (QA) |

## 4. Riesgos pendientes / follow-up
- Sigue pendiente validar end-to-end con Pipes reales (QA Windows).
- `withJournalRetry` depende de `max_retries`; si Postgres cae durante horas bloqueará el worker, pero es preferible vs pérdida de órdenes. Documentado en runbook.
- Linter oficial deberá ejecutarse en CI (versión soportada) por la incompatibilidad local reportada.

