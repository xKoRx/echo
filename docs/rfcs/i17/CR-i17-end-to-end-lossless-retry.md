# Code Review i17 — end-to-end-lossless-retry
## Metadatos
- RFC: `docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md`
- Implementación: `docs/rfcs/i17/IMPLEMENTATION.md`
- Revisor: Echo_Revisor_Codigo_v1
- Dictamen: Listo
- Fecha: 2025-11-18

## 1. Resumen Ejecutivo
- **Heartbeats gRPC resueltos (Gap H1):** `applyDeliveryHeartbeatPolicy` fuerza `grpc/keepalive/*` al intervalo `core/delivery/heartbeat_interval_ms` dentro de 1–5 s, el servidor aplica esos valores al crear el listener y el Agent vuelve a calcular `client_keepalive` con el mismo intervalo antes de validar los límites (`core/internal/config.go:715-774`, `core/internal/core.go:360-399`, `agent/internal/config.go:230-372`). `startDeliveryHeartbeatLoop` refresca y difunde la cadencia (`core/internal/core.go:860-899`).
- **Persistencia lossless (Gap H2):** `ScheduleExecuteOrder/CloseOrder` encapsulan `Insert/Update/Assign` bajo `withJournalRetry`, reintentan hasta `max_retries` con backoff y sólo despachan tras éxito; si se agota, el Router corta el fanout y registra métrica/log (`core/internal/delivery_service.go:160-437`, `core/internal/router.go:471-845`).
- **Observabilidad y métricas:** Cada operación emite spans `core.delivery.*`, logs estructurados y `RecordDeliveryRetry/PendingAge`; el Router marca `journal failed` y `DeliveryHeartbeat` añade `ack_timeout/retry_backoff/heartbeat_interval` para los Agents (`core/internal/delivery_service.go:185-337`, `core/internal/router.go:471-487`, `core/internal/core.go:815-847`).
- **CI:** Builds y unit tests verdes; `golangci-lint` sigue fallando por incompatibilidad conocida con Go 1.25 y las pruebas de Named Pipes MT4 permanecen pendientes en Windows, conforme al plan (`docs/rfcs/i17/CI-i17-end-to-end-lossless-retry.md`).

## 2. Matriz de Hallazgos
Sin hallazgos. Se verificó que los dos gaps del reporte fueron resueltos y no se detectaron desviaciones nuevas respecto al RFC.

## 3. Contratos vs RFC
- `DeliveryHeartbeat` propaga `ack_timeout_ms`, `retry_backoff_ms`, `max_retries` y `heartbeat_interval_ms`, y `LoadDeliveryRuntimeConfig` refresca también `reconciler_interval_ms`, cumpliendo §6.3 del RFC (`core/internal/config.go:400-466`, `core/internal/core.go:815-899`).
- Los contratos gRPC mantienen compatibilidad: el Core exige `KeepAlive` 1–5 s y el Agent clamp-ea el cliente al mismo rango antes de abrir el stream (`core/internal/core.go:360-399`, `agent/internal/core_client.go:20-63`).
- `DeliveryService` asegura que ningún comando sale del Core sin registro en `delivery_journal`, respetando los mandatos DoD de ledger + retries (`core/internal/delivery_service.go:160-274`).

## 4. Concurrencia, Errores y Límites
### 4.1 Concurrencia
- `DeliveryService` expone `UpdateConfig` y el reconciliador recrea su `ticker` al recibir intervalos nuevos (`core/internal/delivery_service.go:113-148`, `core/internal/delivery_service.go:452-520`). No se observaron carreras en lecturas/escrituras gracias a los RWLocks existentes.

### 4.2 Manejo de Errores
- Cada operación de journal usa `withJournalRetry`, registra `telemetry.Warn/Error` con causa y, si excede el máximo, devuelve error que el Router trata como bloqueo del fanout (`core/internal/delivery_service.go:198-437`, `core/internal/router.go:471-845`).

### 4.3 Límites y Edge Cases
- `normalizeHeartbeatInterval` aborta si ETCD define valores fuera de 1–5 s, garantizando que tanto el servidor como el cliente respetan el rango solicitado por i17 (§Mandatos) (`core/internal/config.go:739-781`, `agent/internal/config.go:328-372`).
- `markPending` corta en `MaxRetries` dejando `next_retry_at` vacío para forzar intervención manual y evitar silencios cuando Postgres no vuelve (`core/internal/delivery_service.go:296-315`).

## 5. Observabilidad y Performance
- Spans `core.delivery.journal` y `core.delivery.retry` incluyen `command_id`, `ack_timeout_ms` y `backoff_ms`; se complementan con métricas `echo_core.delivery.retry_total` y `echo_core.delivery.pending_age_ms` (`core/internal/delivery_service.go:185-337`). El Router emite `RecordDeliveryRetry(... stage="journal")` al fallar la programación (`core/internal/router.go:471-487`).
- `startDeliveryHeartbeatLoop` reusa un `timer` por intervalo; la sobrecarga es constante y no aparecen estructuras nuevas en hot path.

## 6. Dictamen Final y Checklist
- **Dictamen:** Listo.

Checklist:
- RFC cubierto sin desviaciones: **OK**
- Build compilable: **OK** (`go build ./core/...`, `go build ./agent/...`)
- Tests clave verdes según CI: **OBS** (lint falla por bug `golangci-lint` + smoke MT4 pendiente)
- Telemetría mínima requerida presente: **OK**
- Riesgos críticos abordados: **OK**

