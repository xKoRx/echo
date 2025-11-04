---
title: "Respuesta a revisión arquitectónica — RFC-004c"
date: "2025-11-04"
status: "Cerrado con ajustes"
related_rfc: "docs/rfcs/RFC-004c-iteracion-3-parte-final-slave-registro.md"
review_source: "docs/rfcs/RFC-004c-REVISION-ARQUITECTONICA.md"
---

## Síntesis ejecutiva

Tomé todos los hallazgos críticos levantados en la revisión y actualicé el RFC para que la iteración 3 entregue un flujo de autoregistro sólido, versionado y con feedback en tiempo real. El nuevo escenario asegura que el Slave siempre sepa si su catálogo quedó filete, que el Core persista especificaciones completas (incluyendo `contract_size`) y que el Agent mida cualquier drift. Con esto cerramos el gap operativo y dejamos la cancha lista para operar con `unknown_action=reject` sin generar slippage extra.

## Acciones aplicadas

- **Versionamiento del handshake**: incorporé `protocol_version=3.0`, validación estricta en el Agent y métrica `echo.handshake.version_mismatch`.
- **Modo degradado en el EA**: `OnInit()` nunca revienta la sesión; el EA queda activo, rechaza órdenes con `EA_DEGRADED` y reintenta parseo cada 60s.
- **Parser robusto**: límites de longitud, trimming, duplicados bidireccionales, fallback para `IsSymbolValid` y registros por cada par inválido.
- **Reintentos MarketInfo**: `BuildSymbolsJsonWithRetries` intenta tres veces con backoff antes de declarar fallo, evitando perder alfa por glitches del broker.
- **Hash de configuración**: `gLastSymbolsHash` evita handshakes innecesarios y asegura idempotencia cuando la configuración no cambia.
- **Feedback loop**: documenté y exigí `SymbolRegistrationResult` desde el Core, propagado por el Agent, con códigos de error por símbolo.
- **Persistencia extendida**: el Core ahora guarda `contract_size` y aplica constraint `account_id + broker_symbol` para evitar drift.
- **Observabilidad**: reforcé logs JSON con `account_id`, `degraded_mode`, `canonical_list` y métricas nuevas (`registration_result`, `degraded_mode`).
- **Plan de pruebas**: añadí suites unitarias/integración cubriendo versiones inválidas, drift de ETCD y fallos de `MarketInfo`.

## Puntos challengeados

- **Timeout en `PipeWriteLn()`**: lo dejé como mejora futura. Hoy el cuello está en la construcción del payload, no en la escritura; forzar un timeout sin reingeniería del pipe puede generar más ruido que señal. Lo documenté para un follow-up en i4.
- **trace_id end-to-end**: el review proponía sumar correlación inmediata. Prefiero tratarlo en la línea base de telemetría del Agent cuando definamos spans compartidos; meterlo ahora en el EA sin soporte homogéneo en el Agent/Core generaría deuda y no resuelve un riesgo real de esta iteración.
- **Formato JSON en el input**: se sugirió reemplazar la cadena `broker:canonical` por JSON. Mantengo el formato actual porque encaja con las restricciones de inputs en MT4 y evita fricción con operadores; el parser ya quedó blindado para mitigar el riesgo.

## Próximos pasos

- Preparar la migración SQL (`contract_size`, constraint de unicidad) y validarla en staging.
- Actualizar la guía operativa y comunicar el nuevo `SymbolMappings` + señal de feedback al equipo de operaciones.
- Ejecutar pruebas piloto con `unknown_action=warn`, monitorear `version_mismatch` y `degraded_mode`, luego mover a `reject` cuando quede estable.

Con estas acciones la arquitectura queda disciplinada, modular y lista para escalar cuentas sin sorpresas. De aquí en adelante solo nos queda ejecutar la pega y medir que el sistema siga seco en performance.
