---
title: "Echo — Contexto General del Copiador de Operaciones"
version: "1.1"
date: "2025-11-07"
status: "Base"
owner: "Aranea Labs"
---

## Propósito

Este documento establece el contexto único y oficial del proyecto Echo: qué es un copiador de operaciones, por qué existe, ventajas y desventajas, problemas típicos del dominio y los patrones de solución probados. Además define la visión y los principios que guían a Echo para ser un copiador de clase mundial.

## Qué es un copiador de operaciones

Un copiador replica en una o varias cuentas seguidoras (slaves) las operaciones ejecutadas en una cuenta maestra (master) con el menor desvío operativo posible. Su utilidad principal es estandarizar la ejecución de estrategias en múltiples cuentas, brokers y condiciones de mercado, manteniendo sincronía y trazabilidad.

## Modos de despliegue

- Local (despliegue por host): cada host Windows ejecuta UN Agent como servicio único que atiende múltiples terminales MT4/MT5 (masters y slaves). El Core puede estar en el mismo host o en otro host de la red local (10 GbE). Latencia mínima (objetivo < 100 ms intra-host), control total del entorno. V1 se enfoca exclusivamente en este modo.
- SaaS (nube): Core centralizado en la nube atendiendo múltiples hosts remotos. Mayor latencia, alta disponibilidad y menor fricción de integración. Fuera de alcance de V1.

## Ventajas y desventajas

- Ventajas
  - Latencia baja y reproducibilidad de la ejecución.
  - Operación multi-cuenta con trazabilidad (por estrategia y por cuenta).
  - Centralización de reglas (tolerancias, ventanas, money management).
  - Observabilidad integral (métricas, logs y trazas) para medir slippage y estabilidad.
- Desventajas
  - Divergencias entre brokers (precios, spreads, StopLevel, reglas FIFO/netting).
  - Riesgo de desincronización por fallas de transporte o ejecuciones locales.
  - Complejidad operativa: correlación de órdenes, idempotencia y manejo de errores.

## Problemas típicos del dominio y patrones de solución

1) Diferencias de precio y spread entre brokers
- Problema: al copiar después del master, el precio del slave suele haber cambiado; spreads y feeds distintos amplifican slippage.
- Soluciones (por iteración):
  - Tolerancia de desvío configurable (puntos/pips) y filtro de spread máximo. Implementado desde i6.
  - Fanout paralelo a múltiples slaves para minimizar colas y sesgos por orden, con envío dirigido a Agents propietarios desde i2 (routing selectivo).
  - Opción de "espera de mejora" con timeout estricto (solo si reduce slippage neto). Implementado en i10 (opcional, time-boxed).
  - Hardening multi-activo (i6): master y slave validan precios por símbolo usando `MarketInfo(symbol, MODE_ASK/BID)` y rechazan ejecuciones cuando el feed no corresponde, evitando replicar quotes de otro instrumento.

2) SL/TP desincronizados y StopLevel
- Problema: el slave puede tocar SL/TP que el master no toca; StopLevel del broker puede impedir setear niveles en la apertura.
- Soluciones:
  - ✅ i8a — Offsets configurables por cuenta×estrategia (`account_strategy_risk_policy.sl_offset_pips/tp_offset_pips`), aplicados en el Core con clamps al StopLevel y fallback inmediato que reenvía `ExecuteOrder` sin offsets ante `ERROR_CODE_INVALID_STOPS`.
  - 🚧 i8b — Validación de StopLevel posterior al fill mediante `ModifyOrder`, pendiente para cubrir brokers que exigen insertar SL/TP después de la ejecución.

3) Missed trades y rechazos
- Problema: el slave no entra por precio fuera de tolerancia, spread alto o error transitorio.
- Soluciones:
  - Política clara por iteración: en V1 no re-entrar automáticamente; registrar y medir.
  - Retries acotados para errores de transporte (no price-chase en hot-path). Implementado desde i1 (retries simples con límites).

4) Correlación y trazabilidad
- Problema: mapear determinísticamente master→slave(s) para aperturas y cierres, evitando duplicados.
- Soluciones:
  - Identidad con UUIDv7 por `trade_id` y `command_id` (desde i1; i0 usaba UUIDv4 temporalmente) y deduplicación determinística.
  - Correlación por `trade_id` y tickets de cada slave; cierre por ticket exacto.
  - Estampar `trade_id` en comentarios del orden del slave como respaldo.

5) Mapeo de símbolos y reglas de broker
- Problema: sufijos/prefijos, tamaños mínimos, lot steps, FIFO/netting, StopLevel.
- Soluciones:
  - Catálogo de símbolos canónico y mapeo por cuenta.
  - Reporte y validación de especificaciones del broker en la conexión del agente.
  - Reglas de cierre compatibles (por ticket o por posición agregada en netting).

6) Conexiones inestables y backpressure
- Problema: timeouts, requotes, desconexiones o jitter en transporte.
- Soluciones:
  - Streams gRPC bidi con keepalive correcto y heartbeats.
  - Backpressure con canales con buffer y serialización por `trade_id`. Implementado en i12 (concurrencia segura y control de presión de cola).
  - Reintentos acotados y telemetría de fallos para diagnósticos.

## Qué hace único a Echo (visión de clase mundial)

- Latencia objetivo: < 1 s extremo a extremo, con objetivo intra-host < 100 ms.
- Idempotencia y correlación estrictas: `trade_id` (UUIDv7) y cierre por ticket del slave.
- MagicNumber replicado en los slaves para trazabilidad con brokers/prop firms.
- Money Management central en el core (por cuenta × estrategia), evolutivo.
- Observabilidad obligatoria y homogénea (logs estructurados, métricas y trazas con atributos por contexto) desde día uno.
- Configuración central y declarativa, con carga única y watches.
- Diseño modular, programando contra interfaces y sin acoplamientos cruzados entre módulos.

## Principios obligatorios de ingeniería

- Modularidad y bajo acoplamiento; SOLID y clean code.
- Programar contra interfaces e invertir dependencias (composición sobre herencia).
- Observabilidad end-to-end con cliente unificado y bundles de métricas por dominio.
- Contexto primero: propagación de `context.Context` y atributos en contexto (comunes, de evento y de métricas) para evitar repetición.
- Configuración exclusivamente desde ETCD; sin variables de entorno ni YAML como fuente de verdad en runtime.
- Reglas de negocio y entidades en la SDK compartida; reutilización estricta entre core y agent.
- Errores: jamás ignorarlos; registrarlos y propagar de forma explícita.

## Alcance funcional de V1 (objetivo acumulado al completar todas las iteraciones)

1. Órdenes a mercado: replicación de entradas y cierres del master a múltiples slaves. (Desde i0)
2. Hedged only: solo cuentas hedged, incluido MT5. (Desde i0)
3. MagicNumber replicado: sin cambios en el slave. (Desde i0)
4. SL/TP opcionales con offset aplicado desde el Core y fallback determinista ante StopLevel (`stop_offset_*`, i8a ✅); `ModifyOrder` post-fill sigue en roadmap (i8b 🚧).
5. Tolerancias: desvío máximo (pips/points), filtro de spread y delay máximo de ejecución. (i6)
6. Ventanas de no-ejecución: bloquean nuevas entradas; no bloquean cierres; cancelación de pendientes heredadas donde aplique; sin reinserción automática. (i8)
7. SL catastrófico configurable por cuenta/estrategia como protección de contingencia. (i9)

Estado actual implementado: i0 (POC) e i1 (persistencia, idempotencia UUIDv7, keepalive gRPC, correlación por ticket).

## Métricas clave y SLO

- Latencia E2E (p50/p95/p99, ms) y por hop (agent→core, core→agent, ejecución en slave).
- Slippage medio y máximo (puntos/pips) y spread al entrar.
- Tasa de copias exitosas y causas de rechazo (StopLevel, spread, desvío, tiempo).
- Missed trades y ratio por cuenta/estrategia.
- Uso del agente (coalesce de ticks, CPU/RAM si aplica) y estabilidad de streams.

## Criterios de aceptación de V1

- Replicar 100% de entradas a mercado y cierres para 3–6 masters y ~15 slaves en entorno local, con MagicNumber replicado.
- Ventanas bloquean entradas y no bloquean cierres; cancelan pendientes heredadas cuando aplique.
- Tolerancias aplicadas y rechazos registrados con motivo claro.
- SL catastrófico configurable y efectivo como última línea de defensa.
- Observabilidad operativa disponible (métricas, logs y trazas) para el funnel completo.
- Reinicios del core sin duplicación: reconstrucción de estado desde persistencia PostgreSQL (i1) y, en iteraciones posteriores, reconciliación con reportes de estado del Agent (StateSnapshot).

## Expectativas de calidad

- Cobertura de pruebas elevada en rutas críticas; pruebas table-driven y mocks donde corresponda.
- Linters y análisis estático limpios; funciones cortas y nombres descriptivos.
- Sin duplicación de lógica transversal; todo lo reutilizable vive en la SDK.

## Resumen ejecutivo

Echo es un copiador local diseñado para operar con latencia muy baja, con identidad e idempotencia estrictas, y con observabilidad integral desde el primer día. La prioridad es la robustez operacional y la trazabilidad precisa, evolucionando por iteraciones pequeñas y seguras hasta completar el alcance de V1.


