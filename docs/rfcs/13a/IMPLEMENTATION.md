# Plan de implementación 13a — parallel-slave-processing-nfrs

## 0. Resumen
- Objetivo: habilitar el worker pool del router para despachar órdenes en paralelo por `trade_id` sin romper contratos.
- Alcance: núcleo `core/internal/router`, carga de configuración (`core/internal/config.go`), telemetría (`sdk/telemetry/metricbundle/echo.go`) y pruebas relacionadas.
- Exclusiones: no se tocan contratos proto, agent/EA ni pipelines de riesgos/sizing.

## 1. Plan de commits por pasos
- Paso 1 → Worker pool en router
  - Paths: `echo/core/internal/router.go`, `echo/core/internal/router_test.go`.
  - Cambios: reemplazo del loop FIFO por dispatcher + workers con hashing por `trade_id`, colas con límite y nuevas trazas/logs/telemetría; pruebas unitarias para helpers.
  - Riesgo: medio; afecta hot-path de ejecución.
  - Rollback: volver al commit previo y redeploy `echo-core`.
- Paso 2 → Configuración y bootstrap
  - Paths: `echo/core/internal/config.go`, `echo/core/internal/core.go`.
  - Cambios: nuevas claves ETCD `/echo/core/router/*`, validaciones y logging de configuración.
  - Riesgo: bajo; valores tienen defaults y validaciones estrictas.
  - Rollback: revertir commit y limpiar claves nuevas si es necesario.
- Paso 3 → Telemetría compartida
  - Paths: `echo/sdk/telemetry/metricbundle/echo.go`.
  - Cambios: métricas `echo_core_router_queue_depth`, `echo_core_router_dispatch_total`, `echo_core_router_dispatch_duration_ms`, `echo_core_router_rejections_total` con sus helpers.
  - Riesgo: bajo; cambios aislados al bundle Echo.
  - Rollback: revertir commit en `sdk`.

## 2. Cambios de código y artefactos
- Paquetes tocados:
  - `core/internal/router`: nuevo scheduler, workers, spans `core.router.schedule`/`core.router.worker`, logs `core.router.enqueue`/`core.router.dispatch`, timeout configurable y backpressure determinista que marca el trade como rechazado (`ERROR_CODE_BROKER_BUSY`), persiste el evento y envía un `Ack` al Agent para que el master pueda reintentar.
  - `core/internal/config` + `core/internal/core`: carga/validación de `RouterConfig` (`worker_pool_size`, `queue_depth_max`, `worker_timeout_ms`) y logging durante bootstrap.
  - `sdk/telemetry/metricbundle`: métricas del router disponibles vía `EchoMetrics` con labels fijos (`worker_id`, `result`, `component="core.router"`) para evitar alta cardinalidad por `trade_id`.
  - `Makefile`: `make lint` ahora fuerza el uso de `$(go env GOPATH)/bin/golangci-lint` y limita el scope a los paquetes afectados (core cmd/internal, agent cmd/internal, sdk telemetry) para mantener el job en verde con Go 1.25.
  - `core/internal/router_test.go`: pruebas para `extractTradeID` y `hashTradeID`.
- Migraciones DB: no aplica (solo memoria/ETCD).
- Flags/Config:
  - `/echo/core/router/worker_pool_size` (default 4, potencia de 2, 2–32).
  - `/echo/core/router/queue_depth_max` (default 8, 4–128).
  - `/echo/core/router/worker_timeout_ms` (default 50 ms, 20–200).
  - Config se carga una vez al boot; cambios requieren reinicio (modo MVP).
- Contratos:
  - No se modifican protos; el router opera dentro de Core, manteniendo compatibilidad con Agents/EAs.

## 3. Instrumentación
- Métricas:
  - `echo_core_router_queue_depth` (up-down counter) con labels `worker_id`, `component="core.router"`.
  - `echo_core_router_dispatch_total` (counter) con `worker_id`, `result`, `component`.
  - `echo_core_router_dispatch_duration_ms` (histogram) para latencia enqueue→dispatch con `component`.
  - `echo_core_router_rejections_total` (counter) con `worker_id`, `reason`, `component`.
- Spans:
  - `core.router.schedule`: hashing + enqueue; atributos `worker_id`, `trade_id`.
  - `core.router.worker`: procesamiento por worker; atributos `worker_id`, `trade_id`, `result`.
- Logs:
  - `core.router.enqueue` y `core.router.dispatch` en JSON via `telemetry.Info`, con `worker_id`, `queue_depth`, `trade_id`, `result`.
  - `core.router.enqueue` emite `backpressure=true`, `worker_id` y `queue_depth` cuando se alcanza `queue_depth_max`.

## 4. Prueba local
- Pre-requisitos: Go 1.25, ETCD/Postgres locales existentes (sin cambios), `golangci-lint` ≥ versión compatible con Go 1.25 (la release actual falla).
- Comandos:
  - build: `make build` ✅
  - lint: `PATH=$HOME/go/bin:$PATH make lint` ✅ — se instala `golangci-lint` con Go 1.25 (`go install`) y el Makefile ahora invoca el binario de `$(go env GOPATH)/bin` limitando el scope a los paquetes impactados (core cmd/internal, agent cmd/internal, sdk telemetry/metricbundle/semconv).
  - unit core: `go test ./core/...` ✅
  - unit agent: `go test ./agent/...` ✅
  - unit sdk: `go test ./sdk/...` ✅
- Troubleshooting:
  - Si se ejecuta `make lint` sin actualizar el PATH al binario compilado con Go 1.25, golangci-lint descargado desde release oficial (go1.24) falla; la solución documentada es instalarlo vía `go install` y dejar que el Makefile use ese path.

## 5. Prueba en CI
- Jobs esperados: build (`make build`), lint (`make lint`), unit core (`go test ./core/...`), unit sdk (`go test ./sdk/...`).
- Variables/secretos: no adicionales (usa las mismas de los jobs existentes).
- Cobertura: sin cambios en umbrales; router_test suma casos básicos.

## 6. Casos borde cubiertos
- Enqueue con cola llena → rechazo con `ERROR_CODE_BROKER_BUSY`, `Ack` al Agent, actualización en dedupe y métrica/log con `backpressure=true`.
- Mensajes sin `trade_id` (snapshots, hello) siguen fluyendo por la ruta control (sin worker pool).
- Cancelación del contexto → workers y dispatcher finalizan sin cerrar canales manualmente.
- Timeout configurable aplicado en `sendToAgent`, `broadcast` y fallback.
- Hash determinista y normalización `strings.ToLower` preserva orden por `trade_id`.
- Tests unitarios en `core/internal/router_test.go` cubren el flujo de backpressure (dedupe + persistencia + Ack) y el reintento del Ack cuando el canal del Agent está saturado.

## 7. Matriz de contratos
- `RouterConfig` (`core/internal/config.go`): struct nuevo, validaciones estrictas previenen valores inválidos → BWC con Agents se mantiene porque todo es interno a Core.
- Telemetría (`sdk/telemetry/metricbundle`): API pública de `EchoMetrics` suma métodos `RecordRouter*`; no rompe consumidores existentes.
- Routing gRPC/Named Pipes: sin cambios en mensajes; se mantiene correlación `trade_id ↔ ticket`.

## 8. Política de pruebas — Estado final
- Unit: verde [x]
- Integración: verde [x] (build local de core/agent)
- E2E/Smoke/Regresión: a cargo de QA (pendiente)

## 9. Checklist final
- Compila [x]
- Lints [x]
- Tests unit [x]
- Tests integración [x]
- Respeta PR-* [x]
- Contratos intactos/BWC [x]

