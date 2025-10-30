---
title: "Respuesta a Correcciones Arquitecturales — Aplicaciones y Matices"
version: "1.0"
date: "2025-10-29"
author: "Arquitectura Echo"
status: "Applied"
---

## Propósito

Dejar constancia escrita de cómo se abordaron las observaciones de la revisión arquitectural: qué se aplicó, qué se matizó y el porqué. Los documentos base actualizados son `00-contexto-general.md` y `01-arquitectura-y-roadmap.md`.

## Cambios aplicados (alto nivel)

- Clarificación de modo "Local": un Agent único por host Windows que atiende múltiples terminales; Core en el mismo u otro host de la red local; V1 se limita a este modo.
- Temporalidad explícita por iteración: se marcó en contexto qué capacidades existen en i0/i1 y cuáles llegan en i2+ (i6 tolerancias, i7a/i7b SL/TP con offset y StopLevel, i8 ventanas, i9 SL catastrófico, i10 espera de mejora, i11 normalización de errores, i12a/b concurrencia y backpressure).
- Identidad: aclarado que i0 usó UUIDv4 y desde i1 se adopta UUIDv7 para `trade_id`/`command_id`.
- Retries: ubicados en i1 (transporte, límites acotados).
- Backpressure: pospuesto y marcado como i12 (no presente en i0/i1).
- Alcance V1: definido como objetivo acumulado de todas las iteraciones con estado actual explícito (i0/i1).
- Reinicios del Core: hoy se apoya en persistencia PostgreSQL (i1); reconciliación con snapshots del Agent quedará para iteraciones posteriores.
- Responsabilidades por componente:
  - Agent: routing local por `target_account_id`, timestamps `t1`/`t4`, coalesce de estado hacia Core, keepalive/heartbeats y telemetría; no aplica filtros ni sizing.
  - Core: cálculo de `lot_size` (fijo en i0/i1, riesgo fijo en i5), validaciones, dedupe/correlación por ticket, políticas por iteración, persistencia, estado réplica (i2+), telemetría integral.
  - Slave EA: `ExecuteOrder`/`CloseOrder` y reporte mediante `ExecutionResult` (también en cierres) con timestamps completos; MagicNumber replicado.
- Contratos mínimos v1: se añadieron campos críticos (`attempt`, `target_client_id`/`target_account_id`, `timestamp_ms`, reglas de `ticket`=0 en i0 vs exacto en i1 para cierres).
- Flujo de datos con timestamps: se documentaron `t2` (Core recv) y `t3` (Core send) y se unificó el reporte de cierres vía `ExecutionResult`.
- Roadmap refinado: división de i7 en i7a/i7b y de i12 en i12a/i12b para reducir riesgo y tamaño de cambio.

## Matices y justificaciones

- Fanout paralelo y routing: en i0 existía broadcast; al introducir routing selectivo (i2) se mantiene el envío paralelo, pero dirigido solo a los Agents propietarios para reducir ruido y latencia percibida.
- `CloseResult` vs `ExecutionResult` en cierres: se adopta un único mensaje `ExecutionResult` para simplificar correlación y métricas. Si existiera compatibilidad histórica con un `CloseResult` legado, se mantendría sólo como transición, priorizando `ExecutionResult` en v1.

## Detalle por documento

### 00-contexto-general.md

- Modos de despliegue: reescrito para precisar el modelo por host y el foco exclusivo de V1.
- Problemas/soluciones: marcadores de iteración en spread/desvío, espera de mejora (i10), retries (i1), UUIDv7 (desde i1) y backpressure (i12).
- SL/TP: aclarado modo configurable (con offset o sin SL/TP locales) y papel del SL catastrófico (i9); StopLevel y post‑fill en i7b.
- Alcance de V1: redefinido como objetivo acumulado con enumeración por iteración y estado actual i0/i1.
- Reinicios: hoy basados en Postgres (i1); reconciliación con snapshots se deja para futuro.

### 01-arquitectura-y-roadmap.md

- Agent: ampliadas responsabilidades (routing local, coalesce de estado, timestamps y telemetría) y explicitado lo que no hace (políticas, filtros, sizing, persistencia de negocio).
- Core: precisado Money Management (fijo i0/i1, riesgo fijo i5 con clamps y specs), políticas por iteración, persistencia y estado réplica (i2+), telemetría.
- Slave EA: unificación de cierres reportados con `ExecutionResult`.
- Infra: marcado qué es i0/i1 y qué es planificado (Mongo i2+ opcional).
- Contratos: añadidos `attempt`, `target_*`, `timestamp_ms` y reglas de `ticket` para cierres; campos requeridos/opcionales definidos.
- Flujo con timestamps: incluidos `t2`/`t3` y descripción de cierres usando `ExecutionResult`.
- Roadmap: i7 dividido en i7a/i7b; i12 dividido en i12a/i12b; resto se mantiene acotado y secuencial.
- Calidad/SLO: normalización de códigos de error movida a i11.

## Riesgos remanentes y mitigación

- Cambios de contratos: si hay consumidores existentes, validar compatibilidad y ofrecer versión de transición (campos opcionales) antes de endurecer requeridos.
- Cardinalidad de métricas: controlar atributos en contexto y evitar etiquetas de alta cardinalidad (IDs únicos) salvo `trade_id`/`command_id` en spans/logs, no en métricas agregadas.
- Iteraciones concurrentes: forzar objetivo único por iteración y criterios de salida medibles para evitar regresiones.

## Siguientes pasos

- Aprobación de esta respuesta y de los documentos `00` y `01` actualizados.
- Priorizar i2 (routing selectivo) e instrumentar métricas de routing antes de continuar con catálogo (i3) y specs (i4).


