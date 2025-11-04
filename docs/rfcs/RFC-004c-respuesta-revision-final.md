---
title: "Respuesta a RFC-004c-REVISION-FINAL"
date: "2025-11-04"
status: "Completado"
related_rfc: "docs/rfcs/RFC-004c-iteracion-3-parte-final-slave-registro.md"
review_source: "docs/rfcs/RFC-004c-REVISION-FINAL.md"
---

## Resumen

Alineé el RFC-004c con el alcance real de la Iteración 3 definido en `RFC-001-architecture.md`. Eliminé la sobre-ingeniería (versionamiento, feedback activo, modo degradado complejo) y añadí los requerimientos que faltaban (snapshots de precios 250 ms, reconexión automática, limpieza de buffers, validación de StopLevel). El documento actualizado (`v1.2`) cubre todo el entregable de i3 y mantiene la implementación dentro del esfuerzo planificado (~2 días).

## Tabla de decisiones

| Punto de la revisión | Acción | Resultado |
| --- | --- | --- |
| Versionamiento `protocol_version` | Eliminado del scope de i3 | ✅ Aceptado – se traslada a i4 como indica el roadmap |
| Feedback loop Core→Agent→EA | Eliminado (logs + métricas pasivas en i3) | ✅ Aceptado |
| Modo degradado con hash y reintentos periódicos | Simplificado: parser falla → EA arranca sin handshake y espera reconexión | ✅ Aceptado |
| Reporting de precios cada 250 ms | Implementado con coalescing en `OnTick()` + reenvío del Agent | ✅ Implementado |
| Reconexión automática EA↔Agent | Documentado y añadido al pseudocódigo (timer de 2 s) | ✅ Implementado |
| Limpieza de buffers | Agregado para EA, Agent y Core con métricas `echo.buffers.cleared_count` | ✅ Implementado |
| Validación StopLevel en Core | Incluida antes de `ExecuteOrder` con métrica de fallos | ✅ Implementado |
| Parser sin duplicados | Actualizado con deduplicación bidireccional y límites de longitud | ✅ Implementado |
| Reintentos `MarketInfo()` | Especificados (3 intentos, backoff 1 s) | ✅ Implementado |
| Migración SQL incompleta | Definida con `contract_size` y PK `(account_id, broker_symbol)` | ✅ Implementado |
| Traducción handshake en Agent | Pseudocódigo incluido para `AccountSymbolsReport` y snapshots | ✅ Implementado |

## Puntos challengeados

No se rechazó ningún hallazgo de la revisión final. Todos los elementos se incorporaron conforme al alcance de i3; las funcionalidades trasladadas a iteraciones futuras fueron documentadas en la sección "Fuera de alcance" del RFC.

## Próximos pasos

1. Aprobar `RFC-004c v1.2` como definición final de la Iteración 3.
2. Ejecutar la implementación siguiendo el plan de despliegue y pruebas del RFC.
3. Registrar en el backlog que versionamiento de protocolo y feedback activo quedan agendados para i4.
