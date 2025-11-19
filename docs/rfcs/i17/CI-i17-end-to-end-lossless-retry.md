# CI i17 — End-to-end lossless retry

| Job            | Estado | Notas                                                                 |
|----------------|--------|-----------------------------------------------------------------------|
| build          | ok     | `go build ./core/...` y `go build ./agent/...` locales                |
| lint           | fail   | `/home/kor/go/bin/golangci-lint run` → `typechecking error: pattern ./...: directory prefix . does not contain modules listed in go.work` (el tooling de golangci-lint aún no soporta el layout multi-módulo de Echo en Go 1.25; se documenta para que CI lo ejecute con la versión soportada) |
| unit           | ok     | `cd core && go test ./...` — `cd agent && go test ./...`             |
| integración    | pend.  | Se requiere entorno Windows con pipes reales (documentado para QA)   |

Logs relevantes en `core/internal/delivery_service.go` y `agent/internal/delivery_manager.go`.

