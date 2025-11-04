# Echo Iteración 3 — Estado de Cumplimiento (RFC-004c & RFC-004 vs RFC-001)

| Item | RFC | Compromiso | Estado | Evidencia / Comentarios |
|------|-----|------------|--------|-------------------------|
| **1** | RFC-004c §3.1 | Parser robusto `SymbolMappings` en Slave EA | ✅ | `clients/mt4/slave.mq4` asegura límites, normaliza y evita duplicados. |
| **2** | RFC-004c §3.4 | Reconexión automática EA↔Agent con reenvío de handshake | ✅ | `OnTimer()` + `SafeReconnect()` reenvían handshake y specs tras reconectar. |
| **3** | RFC-004c §3.3 | Snapshots Bid/Ask cada 250 ms | ✅ | `SendQuoteSnapshots()` coalescea usando `g_LastSnapshotMs`. |
| **4** | RFC-004c §3.2 | Handshake incluye `symbols[]` completo | ✅ | `BuildHandshakeSymbolsJSON()` adjunta mappings con specs completas. |
| **5** | RFC-004c §3.2 | Reportar `symbol_spec_report` tras reconexión | ✅ | `SafeReconnect()` llama a `SendSymbolSpecReport()`. |
| **6** | RFC-004c §3.2 | Reenvío periódico de `symbol_spec_report` | ✅ (mejora) | Ahora se envía cada 5 s para resiliencia. |
| **7** | RFC-004c §3.5 | Limpieza de buffers tras ejecuciones/cierres | ✅ | `RegisterCommand` y arrays en Slave; Agent/Core limpian correlaciones. |
| **8** | RFC-004c §3.6 | Agent traduce handshake a `AccountSymbolsReport` | ✅ | `agent/internal/pipe_manager.go` construye `AccountSymbolsReport`. |
| **9** | RFC-004c §3.6 | Agent reenvía snapshots coalescidos | ✅ | Flujos confirmados por logs periódicos. |
| **10** | RFC-004c §3.7 | Core valida canónicos desde ETCD | ✅ | `CanonicalValidator` activo con `unknown_action`. |
| **11** | RFC-004c §3.7 | Core ajusta SL/TP con StopLevel | ✅ | `router.adjustStopsAndTargets()` aplica ajustes. |
| **12** | RFC-004c §3.7 | Persistencia de specs en PostgreSQL | ✅ | `SymbolSpecService.Upsert` + `postgresSymbolSpecRepo.UpsertSpecifications`. |
| **13** | RFC-004c §3.7 | Telemetría mínima (logs/métricas) | ✅ | Nuevos logs DEBUG/INFO/WARN y métricas `RecordSymbolsReported`. |
| **14** | RFC-004c §8 | Observabilidad mínima disponible | ✅ | Todos los componentes usan `sdk/telemetry` (logs JSON, métricas, spans). |
| **15** | RFC-004 §3.2/3.3 | `AccountSymbolsReport` procesado y persistido | ✅ | `handleAccountSymbolsReport` + resolver + repositorio. |
| **16** | RFC-004 §3.3 | Caché/resolver con persistencia async | ✅ | `AccountSymbolResolver` mantiene caché y worker. |
| **17** | RFC-004 §3.3 | Política `unknown_action` `warn/reject` configurable | ✅ | `config.UnknownAction` + `CanonicalValidator`. |
| **18** | RFC-004 §3.3 | Traducción canónico→broker en órdenes y cierres | ✅ | `createExecuteOrders` y `handleTradeClose` usan resolver. |
| **19** | RFC-004 §3.3 | Warm-up lazy desde Postgres | ✅ | Resolver carga desde repositorio ante miss. |
| **20** | RFC-004 §3.4 | Tabla/migración de mapeos por cuenta | ⚠️ parcial | `account_symbol_spec` almacena especificaciones; nombre difiere de `account_symbol_map` descrito en RFC. |
| **21** | RFC-004 §3.5 | Prioridad `core/canonical_symbols` sobre whitelist | ✅ | `config.go` prioriza y limpia valores. |
| **22** | RFC-004 §5 | Métricas `echo.symbols.lookup`, `echo.symbols.reported` | ✅ | Bundles EchoMetrics registran hits/misses y reportes. |
| **23** | RFC-004 §6 | Rollout `warn` → `reject` soportado | ✅ | `UnknownAction` + logs/metrics facilitan transición. |
| **24** | RFC-004 §7 | Validación manual: rechazo por canónico inválido | ✅ | `ValidateSymbolSpecReport` / `ValidateAccountSymbolsReport` retornan error. |
| **25** | RFC-004 §10 | Riesgo “maping incompleto” mitigado con métricas/logs | ✅ | WARN + métricas + reenvío periódico de specs. |
| **26** | RFC-001 §7.4 | Mapeo canónico⇄broker antes de ejecutar | ✅ | Resolver aplicado en pipeline en i3. |
| **27** | RFC-001 §7.4 | Persistencia mapeo/spec por cuenta | ✅ | Specs y mappings guardados vía repositorio. |
| **28** | RFC-001 §7.4 | Snapshots 250 ms desde Slaves | ✅ | Confirmado por implementación/logs. |
| **29** | RFC-001 §7.4 | Reconexión automática EA↔Agent y handshake | ✅ | Implementado con backoff y reenvío. |
| **30** | RFC-001 §7.4 | Limpieza buffers post close | ✅ | Cubierto en EA/Agent/Core (ver Ítem 7). |
| **31** | RFC-001 §7.4 | Validación StopLevel en Core | ✅ | Ajustes aplicados en router con logs. |
| **32** | RFC-001 §7.4 | Observabilidad end-to-end (logs/metrics/traces) | ✅ | Todos los componentes cableados a OTel. |
| **33** | RFC-001 §6 | Money Management (riesgo fijo) | ❌ (futuro) | Planificado para iteración 4; aún no implementado. |


## Observaciones adicionales

- **Reenvío periódico de especificaciones**: añadido para robustez ante reinicios; documentar en RFCs como práctica estándar.
- **Naming de tabla**: la implementación usa `account_symbol_spec` (especificaciones). La RFC menciona `account_symbol_map`; validar si se necesita renombrar o crear una vista/documentación aclaratoria.
- **Telemetría amplificada**: la incorporación de `core/log_level` (nuevo en config) permite habilitar DEBUG sin redeploy, alineado con las reglas de observabilidad.
- **Pruebas integrales**: la RFC-004c marca “⏳ Pruebas integrales ejecutadas con evidencia”; falta formalizar la evidencia de QA para completar el checklist.

## Sugerencias

1. Actualizar RFC-004c/RFC-004 con la decisión de reenviar `symbol_spec_report` periódicamente.
2. Revisar documentación para reflejar que `account_symbol_spec` cumple rol de especificaciones registradas por cuenta.
3. Registrar la evidencia de pruebas integrales (QA) para cerrar el único ítem pendiente.
4. Planificar la siguiente iteración (i4) enfocada en Money Management (riesgo fijo) según RFC-001 §6.

