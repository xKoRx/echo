---
title: "Correcciones y Mejoras Arquitecturales — Documentación Base"
version: "1.0"
date: "2025-10-29"
author: "Arquitecto Senior Expert — Revisión Crítica"
status: "Review Completed"
---

# Revisión Crítica de Documentación Base Echo

## Resumen Ejecutivo

Este documento contiene la revisión arquitectural exhaustiva de los documentos `00-contexto-general.md` y `01-arquitectura-y-roadmap.md`. Como arquitecto senior expert, he identificado inconsistencias, ambigüedades y oportunidades de mejora que deben corregirse antes de que estos documentos sirvan como base para las iteraciones posteriores.

**Conclusión general**: Los documentos tienen una **estructura sólida** y cubren los aspectos esenciales, pero contienen **inconsistencias temporales** (mezclan estado actual con estado futuro), **ambigüedades de alcance** y **falta de precisión** en responsabilidades de componentes. Las correcciones propuestas garantizan que la documentación sea una referencia clara e inequívoca.

---

## 1. Correcciones Críticas: 00-contexto-general.md

### 1.1 Problema: Confusión entre "Local" y modo de despliegue del Agent

**Ubicación**: Línea 19-20

**Texto actual**:
```
- Local (en el mismo host/VPS de las plataformas): latencia mínima, control total del entorno. En Echo V1 el enfoque es local.
- SaaS (nube): multi-plataforma y alta disponibilidad, con mayores latencias y fricción de integración. Fuera de alcance de V1.
```

**Problema**:  
El término "local" es ambiguo. No aclara que en V1:
- Hay **UN Agent por host Windows** (servicio único)
- Múltiples terminales (Masters + Slaves) en el mismo host se conectan a ese Agent único
- El Core puede estar en el mismo host o en otro host de la red local (no necesariamente en el mismo Windows que los terminales)

**Corrección propuesta**:
```markdown
- Local (despliegue por host): cada host Windows ejecuta UN Agent como servicio único que atiende múltiples terminales MT4/MT5 (masters y slaves). El Core puede estar en el mismo host o en otro host de la red local (10 GbE). Latencia mínima (objetivo < 100 ms intra-host), control total del entorno. **V1 se enfoca exclusivamente en este modo.**
- SaaS (nube): Core centralizado en la nube atendiendo múltiples hosts remotos. Mayor latencia, alta disponibilidad, menor fricción de integración. **Fuera de alcance de V1.**
```

---

### 1.2 Problema: Presentación de características futuras como presentes

**Ubicación**: Líneas 36-42 (Soluciones a problemas de precio/spread)

**Texto actual**:
```
- Soluciones:
  - Tolerancia de desvío configurable (puntos/pips) y filtro de spread máximo.
  - Opción de "espera de mejora" con timeout estricto (solo si reduce slippage neto).
  - Fanout paralelo a múltiples slaves para minimizar colas y sesgos por orden.
```

**Problema**:  
La "espera de mejora" se implementa en **iteración 10** según el roadmap, NO está presente en i0/i1 ni en el diseño inicial de V1. Presentarla aquí como solución general induce a error.

**Corrección propuesta**:
```markdown
- Soluciones (V1):
  - Tolerancia de desvío configurable (puntos/pips) y filtro de spread máximo. **Implementado desde i6 (filtros de spread y desvío)**.
  - Fanout paralelo a múltiples slaves para minimizar colas y sesgos por orden. **Desde i2 (routing selectivo)**.
  - Opción de "espera de mejora" con timeout estricto (solo si reduce slippage neto). **Implementado en i10 (opcional, time-boxed)**.
```

---

### 1.3 Problema: "Modo sin SL/TP locales" sin aclaración de alcance

**Ubicación**: Línea 48-49

**Texto actual**:
```
  - Modo sin SL/TP locales con SL de emergencia amplio; cierre por señal del master.
```

**Problema**:  
No está claro si este "modo sin SL/TP" está implementado en V1 o es una opción de configuración. Según RFC-001 y el roadmap, la iteración 7 implementa "SL/TP con offset (modo B) y StopLevel-aware", pero no menciona explícitamente un "modo sin SL/TP con SL de emergencia".

**Corrección propuesta**:
```markdown
  - Modo configurable: con SL/TP copiados (con offset) o sin SL/TP locales. En el segundo caso, el cierre ocurre solo por señal del master. **SL catastrófico** (iteración 9) actúa como protección de contingencia independiente del master en ambos modos.
```

---

### 1.4 Problema: "Retries acotados" presentado como existente

**Ubicación**: Línea 54

**Texto actual**:
```
  - Retries acotados para errores de transporte (no price-chase en hot-path).
```

**Problema**:  
Según RFC-003 (i1), los retries son "simples y acotados, solo en canales de transporte". Pero en i0 no existen retries. Presentarlo como solución general sin contextualización temporal es impreciso.

**Corrección propuesta**:
```markdown
  - Retries acotados para errores de transporte (no price-chase en hot-path). **Implementado desde i1 (retries simples con límites)**.
```

---

### 1.5 Problema: UUIDv7 sin aclaración de evolución temporal

**Ubicación**: Líneas 59-61

**Texto actual**:
```
  - Identidad con UUIDv7 por `trade_id` y `command_id` y deduplicación determinística.
```

**Problema**:  
Según RFC-002 y RFC-003, **i0 usaba UUIDv4** por compatibilidad rápida con MT4, y **i1 migra a UUIDv7**. No aclarar esto genera confusión sobre cuándo se adoptó UUIDv7.

**Corrección propuesta**:
```markdown
  - Identidad con UUIDv7 por `trade_id` y `command_id` (desde i1; i0 usaba UUIDv4 temporalmente) y deduplicación determinística.
```

---

### 1.6 Problema: "Backpressure con canales con buffer" como característica presente

**Ubicación**: Líneas 73-75

**Texto actual**:
```
  - Backpressure con canales con buffer y serialización mínima por `trade_id`.
```

**Problema**:  
Según el roadmap, la iteración 12 es "Concurrencia segura por `trade_id` y backpressure". En i0/i1 el procesamiento es **secuencial**. Presentar backpressure como característica presente es incorrecto.

**Corrección propuesta**:
```markdown
  - Backpressure con canales con buffer y serialización por `trade_id`. **Implementado en i12 (concurrencia segura y control de presión de cola)**.
```

---

### 1.7 Problema: Alcance funcional de V1 mezcla características de múltiples iteraciones

**Ubicación**: Líneas 99-106 (Alcance funcional de V1)

**Texto actual**:
```
- Órdenes a mercado: replicación de entradas y cierres del master a múltiples slaves.
- Hedged only (incluido MT5 con cuentas hedged).
- MagicNumber replicado sin cambios en el slave.
- SL/TP opcionales en slaves con offset configurable y respeto a StopLevel (con fallback de modificación post-fill si aplica).
- Tolerancias: desvío máximo (pips/points), filtro de spread y delay máximo de ejecución.
- Ventanas de no-ejecución: bloquean nuevas entradas (no bloquean cierres); cancelación de pendientes heredadas (cuando aplique); sin reinserción automática en V1.
- SL catastrófico configurable por cuenta/estrategia como protección de contingencia.
```

**Problema**:  
Esta sección mezcla características implementadas en i0/i1 con características de iteraciones futuras (i6, i7, i8, i9) sin dejar claro **cuándo** se implementan. Para un documento de contexto general está bien describir el alcance final de V1, pero debe aclarar que es el **objetivo acumulativo** de todas las iteraciones.

**Corrección propuesta**:
```markdown
## Alcance funcional de V1 (objetivo acumulado al completar todas las iteraciones)

1. **Órdenes a mercado**: replicación de entradas y cierres del master a múltiples slaves. *(Desde i0)*
2. **Hedged only**: solo cuentas hedged, incluido MT5. *(Desde i0)*
3. **MagicNumber replicado**: sin cambios en el slave. *(Desde i0)*
4. **SL/TP opcionales** en slaves con offset configurable y respeto a StopLevel (con fallback de modificación post-fill si aplica). *(Iteración 7)*
5. **Tolerancias**: desvío máximo (pips/points), filtro de spread y delay máximo de ejecución. *(Iteración 6)*
6. **Ventanas de no-ejecución**: bloquean nuevas entradas (no bloquean cierres); cancelación de pendientes heredadas (cuando aplique); sin reinserción automática en V1. *(Iteración 8)*
7. **SL catastrófico** configurable por cuenta/estrategia como protección de contingencia. *(Iteración 9)*

**Estado actual implementado**: i0 (POC 48h) e i1 (persistencia, idempotencia UUIDv7, keepalive gRPC, correlación determinística por ticket).
```

---

### 1.8 Problema: Reinicios del core sin aclaración de estado actual

**Ubicación**: Línea 122

**Texto actual**:
```
- Reinicios del core sin duplicación: reconstrucción de estado desde reportes del agente y correlación consistente por `trade_id` y tickets.
```

**Problema**:  
Según RFC-003 (i1), la persistencia básica está implementada, pero la "reconstrucción completa desde reportes del agente" no está detallada. Esto puede ser una característica futura (iteración 12 o posterior).

**Corrección propuesta**:
```markdown
- Reinicios del core sin duplicación: reconstrucción de estado desde persistencia PostgreSQL (i1) y, en iteraciones posteriores, reconciliación con reportes de estado del agente (StateSnapshot).
```

---

## 2. Correcciones Críticas: 01-arquitectura-y-roadmap.md

### 2.1 Problema: Responsabilidades del Agent poco claras

**Ubicación**: Líneas 35-43 (Agent - Responsabilidades)

**Texto actual**:
```
- Responsabilidades
  - Bridge entre EAs (Named Pipes) y Core (gRPC bidi).
  - Registro de cuentas y ownership por host (handshake); fanout hacia los slaves locales.
  - Añadir timestamps de hop; publicar métricas y logs.
  - Respetar keepalive gRPC y heartbeats; reconectar con backoff.
- No responsabilidades
  - No decide políticas de negocio ni transforma reglas de sizing.
  - No persiste estado de negocio (más allá de colas/buffers internos).
```

**Problema**:  
Falta aclarar:
1. ¿El Agent aplica filtros de spread/desvío locales o solo el Core?
2. ¿El Agent hace coalesce de ticks/equity hacia el Core?
3. ¿El Agent hace routing por `target_account_id` hacia el pipe del Slave correcto?

Según el código (`agent/internal/stream.go`, `agent/internal/pipe_manager.go`) y RFC-001:
- El Agent SÍ hace routing local por `target_account_id` hacia el pipe correcto del Slave.
- El Agent SÍ reporta coalesce de ticks/equity/estado (según PRD y RFC-001).
- El Agent NO aplica filtros de negocio (eso es del Core).

**Corrección propuesta**:
```markdown
- Responsabilidades
  - **Bridge** entre EAs (Named Pipes) y Core (gRPC bidi).
  - **Registro de cuentas** y ownership por host en handshake; el Core usa esto para routing selectivo (i2+).
  - **Routing local** por `target_account_id`: envía `ExecuteOrder`/`CloseOrder` al pipe del Slave correcto.
  - **Añadir timestamps** de hop (t1 en recepción desde Master, t4 en recepción desde Core).
  - **Reportar estado coalescido** hacia el Core: equity, balance, margen, posiciones/órdenes, ticks, specs de símbolos (cada ~250 ms según PRD).
  - **Keepalive gRPC** y heartbeats; reconectar con backoff exponencial en caso de desconexión.
  - **Telemetría propia**: logs, métricas y trazas del Agent y de los EAs conectados.
- No responsabilidades
  - **No decide políticas de negocio** (ventanas, tolerancias, Money Management).
  - **No transforma reglas de sizing** (eso es del Core).
  - **No aplica filtros de spread/desvío** (eso es del Core en V1; en futuras iteraciones el Agent puede aplicar filtros locales si se decide).
  - **No persiste estado de negocio** (solo colas/buffers internos volátiles).
```

---

### 2.2 Problema: Responsabilidades del Core poco claras sobre Money Management

**Ubicación**: Líneas 46-55 (Core - Responsabilidades)

**Texto actual**:
```
- Responsabilidades
  - Orquestación y enrutamiento determinista Master→Slaves.
  - Validaciones y deduplicación por `trade_id`; correlación por ticket de cada slave.
  - Money Management central (evolutivo) y evaluación de políticas (tolerancias, ventanas, SL catastrófico).
  - Persistencia de órdenes/ejecuciones/estado vivo y auditoría de eventos crudos.
  - Telemetría integral del funnel (latencias por hop y E2E; resultados y rechazos).
- No responsabilidades
  - No implementa IPC con EAs; esto es del Agent.
```

**Problema**:  
"Money Management central (evolutivo)" es vago. Según PRD y RFC-001:
- En i0/i1: lot fijo hardcoded (0.10).
- En i5: sizing con riesgo fijo (modo A).
- El Core SÍ debe calcular el lot_size del slave en `ExecuteOrder`, pero en i0/i1 es fijo.

Falta aclarar también:
- ¿El Core persiste en MongoDB eventos crudos?
- ¿El Core mantiene "estado réplica" de posiciones/equity de cada cuenta?

**Corrección propuesta**:
```markdown
- Responsabilidades
  - **Orquestación y enrutamiento** determinista Master→Slaves (broadcast en i0/i1; selectivo desde i2).
  - **Validaciones** de forma (símbolos, campos obligatorios) y deduplicación por `trade_id`; correlación por ticket de cada slave.
  - **Money Management central**: cálculo de `lot_size` para cada slave. En i0/i1 es lot fijo hardcoded (0.10); desde i5 soporta riesgo fijo (modo A) con ATR y tick value.
  - **Evaluación de políticas**: tolerancias (i6), ventanas de no-ejecución (i8), SL catastrófico (i9).
  - **Persistencia**: PostgreSQL para órdenes, ejecuciones, correlación `trade_id ↔ ticket(s)` y dedupe (desde i1). MongoDB para eventos crudos en append-only (planificado, puede implementarse en i2+).
  - **Estado réplica** por cuenta: posiciones, órdenes, equity, margen, último tick relevante (construido desde reportes del Agent; i2+).
  - **Telemetría integral** del funnel: latencias por hop y E2E, resultados (éxito/rechazo), causas de rechazo, slippage (cuando aplique).
- No responsabilidades
  - **No implementa IPC** con EAs (esto es del Agent).
  - **No ejecuta órdenes** en brokers (esto es del Slave EA vía Agent).
```

---

### 2.3 Problema: Contratos funcionales incompletos

**Ubicación**: Líneas 80-86 (Contratos funcionales)

**Texto actual**:
```
- `TradeIntent` (master→core vía agent): trade_id, ts, client_id, account_id, symbol, side, lot_size master, price, magic_number, ticket, timestamps.
- `ExecuteOrder` (core→slave vía agent): command_id, trade_id, client_id slave, account_id, symbol, side, lot_size slave, magic_number, timestamps, SL/TP opcionales.
- `ExecutionResult` (slave→core vía agent): command_id, trade_id, client_id slave, success, ticket, error_code, executed_price opcional, timestamps completos.
- `TradeClose` y `CloseOrder`/`CloseResult` para cierres coordinados por ticket.
```

**Problema**:  
Estos contratos están bien a alto nivel, pero faltan campos críticos según los protos reales:
- `TradeIntent`: no menciona `attempt` (reintentos).
- `TradeClose`: no menciona `account_id` (añadido en i1 según Issue #M4).
- `CloseOrder`: no menciona que `ticket` puede ser 0 en i0 (fallback a búsqueda por magic+symbol) pero debe ser el ticket exacto desde i1.

**Corrección propuesta**:
```markdown
## Contratos funcionales (mensajes mínimos v1)

Basados en `sdk/proto/v1/*.proto`:

- **`TradeIntent`** (master→core vía agent):
  - `trade_id` (UUIDv7), `timestamp_ms`, `client_id`, `symbol`, `side`, `lot_size` (del master), `price`, `magic_number`, `ticket` (del master), `timestamps` (TimestampMetadata con t0).
  - Opcionales: `stop_loss`, `take_profit`, `comment`, `attempt` (número de reintento, desde i1).

- **`ExecuteOrder`** (core→slave vía agent):
  - `command_id` (UUIDv7), `trade_id`, `timestamp_ms`, `target_client_id`, `target_account_id` (slave destino), `symbol`, `side`, `lot_size` (calculado para el slave), `magic_number`, `timestamps` (propaga t0..t3).
  - Opcionales: `stop_loss`, `take_profit`, `comment`.

- **`ExecutionResult`** (slave→core vía agent):
  - `command_id`, `trade_id`, `success` (bool), `ticket` (si success=true, ticket generado por el broker), `error_code`, `timestamps` (completo con t0..t7).
  - Opcionales: `error_message`, `executed_price`, `execution_time_ms`.

- **`TradeClose`** (master→core vía agent):
  - `trade_id`, `timestamp_ms`, `client_id`, `account_id` (del master, Issue #M4), `ticket` (del master), `symbol`, `magic_number`, `close_price`.
  - Opcionales: `profit`, `reason` (manual/sl/tp/signal).

- **`CloseOrder`** (core→slave vía agent):
  - `command_id` (UUIDv7), `trade_id`, `timestamp_ms`, `ticket` (del slave; 0 en i0 si no se conoce, exacto desde i1), `target_client_id`, `target_account_id`, `symbol`, `magic_number`, `timestamps`.
  - Opcionales: `lot_size` (para cierres parciales).

**Nota**: `TimestampMetadata` contiene t0..t7 para medir latencia por hop (ver Flujo de datos).
```

---

### 2.4 Problema: Flujo de datos con timestamps mal explicados

**Ubicación**: Líneas 88-92 (Flujo de datos)

**Texto actual**:
```
1. Master EA publica `TradeIntent` → Agent añade `t1` → Core valida, deduplica y crea `ExecuteOrder`.
2. Core enruta a los Agent propietarios de las cuentas slave → Agent añade `t4` y entrega a Slave EA.
3. Slave EA ejecuta y envía `ExecutionResult` con `t5..t7` → Core actualiza correlación y métricas.
4. En cierre, el master emite `TradeClose` → Core envía `CloseOrder` por ticket exacto → Slave EA responde `CloseResult`.
```

**Problema**:  
Falta claridad sobre:
- ¿Cuándo se añade t2 (Core recv)?
- ¿Cuándo se añade t3 (Core send)?
- `CloseResult` no existe como mensaje separado; el Slave EA envía `ExecutionResult` también para cierres (el Agent/Core lo distingue por el `command_id`).

**Corrección propuesta**:
```markdown
## Flujo de datos (alto nivel con timestamps)

**Apertura de trade**:
1. **Master EA** genera `TradeIntent` con `trade_id` (UUIDv7) y timestamp `t0` (al momento de detectar el fill).
2. **Agent** recibe desde pipe, añade `t1` (Agent recv), valida JSON y convierte a Proto. Envía `AgentMessage` al Core por gRPC.
3. **Core** recibe, añade `t2` (Core recv), valida símbolo, deduplica por `trade_id`, calcula `lot_size` para cada slave y genera N `ExecuteOrder` (uno por slave configurado). Marca `t3` (Core send) y envía a Agents.
4. **Agent** recibe `CoreMessage` con `ExecuteOrder`, añade `t4` (Agent recv desde Core), hace routing local por `target_account_id` al pipe del Slave correcto.
5. **Slave EA** recibe comando, marca `t5` (Slave EA recv), `t6` (antes de OrderSend), `t7` (después de OrderSend, cuando obtiene ticket o error). Envía `ExecutionResult` con todos los timestamps al Agent.
6. **Agent** convierte a Proto y envía al Core. **Core** persiste ejecución, actualiza correlación `trade_id ↔ ticket` y calcula latencia E2E (t7 - t0) si ambos timestamps existen.

**Cierre de trade**:
1. **Master EA** emite `TradeClose` con `trade_id` y `ticket` del master.
2. **Core** recibe vía Agent, consulta persistencia para obtener el `ticket` exacto de cada slave para ese `trade_id`.
3. **Core** genera `CloseOrder` por slave con el `ticket` exacto (no 0). Marca `t3` y envía a Agents.
4. **Slave EA** recibe, cierra por ticket, marca t5..t7 y envía `ExecutionResult` (que el Core identifica como cierre por el `command_id`).
5. **Core** persiste el cierre en tabla `closes` (i1) y libera `command_id` del índice interno.

**Nota**: En i0, los cierres usaban ticket=0 y fallback a búsqueda por magic+symbol. Desde i1, el Core envía el ticket exacto y el Slave EA prioriza el ticket sobre el magic.
```

---

### 2.5 Problema: Roadmap con iteraciones que atacan múltiples puntos

**Ubicación**: Líneas 106-191 (Roadmap evolutivo)

**Crítica del usuario original**:
> "las iteraciones definidas no tienen un sentido y atacan múltiples puntos a la vez, esto lo que provoca es que se generen muchos más errores en el camino y los modelos de IA normalmente dejan la cagada."

**Análisis**:  
Revisando el roadmap propuesto, encuentro que:

1. **Iteración 2 (Routing selectivo)**: BIEN ENFOCADA. Solo routing por ownership.
2. **Iteración 3 (Catálogo canónico de símbolos)**: BIEN ENFOCADA. Solo mapeo de símbolos.
3. **Iteración 4 (Especificaciones de broker)**: BIEN ENFOCADA. Solo validación de specs.
4. **Iteración 5 (Sizing con riesgo fijo)**: BIEN ENFOCADA. Solo cálculo de lot_size.
5. **Iteración 6 (Filtros de spread y desvío)**: BIEN ENFOCADA. Solo tolerancias en apertura.
6. **Iteración 7 (SL/TP con offset y StopLevel-aware)**: **PODRÍA SER DEMASIADO**. Incluye validación, offset, StopLevel y modificación post-fill. Sugiero dividir en:
   - i7a: SL/TP con offset fijo (sin StopLevel check).
   - i7b: Validación de StopLevel y fallback a modificación post-fill.
7. **Iteración 8 (Ventanas de no-ejecución)**: BIEN ENFOCADA. Solo bloqueo de entradas por ventanas.
8. **Iteración 9 (SL catastrófico)**: BIEN ENFOCADA. Solo protección de contingencia.
9. **Iteración 10 (Espera de mejora opcional)**: BIEN ENFOCADA. Solo time-boxed price improvement.
10. **Iteración 11 (Normalización de códigos de error)**: BIEN ENFOCADA. Solo diccionario central.
11. **Iteración 12 (Concurrencia segura y backpressure)**: **PODRÍA SER DEMASIADO**. Incluye serialización por trade_id, canales con buffer, límites y métricas. Sugiero:
    - i12a: Serialización por `trade_id` con worker pool.
    - i12b: Backpressure y límites de cola.
12. **Iteración 13 (Telemetría avanzada)**: BIEN ENFOCADA. Solo métricas de dominio.
13. **Iteración 14 (Paquetización y operación)**: BIEN ENFOCADA. Solo CLI y scripts de operación.

**Corrección propuesta**:  
Modificar iteraciones 7 y 12 para dividirlas en sub-iteraciones más pequeñas. Ver sección 3.

---

## 3. Propuesta de Roadmap Mejorado

Basado en el roadmap original, propongo estas **modificaciones mínimas** para hacerlo más seguro:

### **Mantener sin cambios**: i2, i3, i4, i5, i6, i8, i9, i10, i11, i13, i14.

### **Dividir Iteración 7** (SL/TP con offset):

#### **Iteración 7a — SL/TP con offset fijo (sin StopLevel)**
- **Objetivo**: habilitar SL/TP opcionales con offset configurable en apertura, sin validación de StopLevel.
- **Alcance**: si el master trae SL/TP, aplicar offset (+/− pips) y enviar en `ExecuteOrder`. Si el broker rechaza, registrar error y continuar sin SL/TP (omit with comment).
- **Exclusiones**: validación de StopLevel, modificación post-fill, trailing.
- **Criterios de salida**: métricas de inserciones con SL/TP vs rechazos por broker; 0 bloqueos en flujo normal.

#### **Iteración 7b — Validación de StopLevel y modificación post-fill**
- **Objetivo**: validar StopLevel antes de enviar; si no cumple, insertar sin SL/TP y modificar inmediatamente después del fill.
- **Alcance**: consultar StopLevel reportado por el Slave EA (desde i4); si distancia < StopLevel, insertar market sin SL/TP y enviar `ModifyOrder` inmediatamente tras recibir `ExecutionResult` exitoso.
- **Exclusiones**: trailing, reglas dinámicas.
- **Criterios de salida**: % de inserciones en apertura vs post-fill medido; 0 rechazos por StopLevel en flujo normal (con fallback a post-fill).

### **Dividir Iteración 12** (Concurrencia segura):

#### **Iteración 12a — Serialización por `trade_id` con worker pool**
- **Objetivo**: mantener orden de procesamiento por `trade_id` sin bloquear mensajes de otros trades.
- **Alcance**: worker pool con N goroutines; mensajes con mismo `trade_id` se procesan secuencialmente, distintos `trade_id` en paralelo. Usar canales por `trade_id` hash.
- **Exclusiones**: límites de cola (próxima iteración).
- **Criterios de salida**: sin reordenamientos ni duplicados por `trade_id`; p95 estable bajo carga.

#### **Iteración 12b — Backpressure y límites de cola**
- **Objetivo**: evitar saturación y OOM bajo ráfagas extremas.
- **Alcance**: canales con buffer configurable; métricas de cola (profundidad, tiempo de espera); rechazo de mensajes si buffer lleno (con alerta).
- **Exclusiones**: paralelismo agresivo global.
- **Criterios de salida**: métricas de backpressure activas; sistema estable bajo ráfagas de 100 trades/s.

### **Resultado**:  
En lugar de 14 iteraciones, tenemos **17 iteraciones** más pequeñas y enfocadas (i2..i6, i7a, i7b, i8..i11, i12a, i12b, i13, i14).

---

## 4. Otras Observaciones Menores

### 4.1 Infraestructura de soporte (01-arquitectura-y-roadmap.md, líneas 74-78)

**Texto actual**:
```
- ETCD v3: configuración central y declarativa con watches.
- PostgreSQL 16: estado vivo, correlación `trade_id ↔ ticket(s)`, dedupe y auditoría operacional.
- MongoDB 7: eventos crudos (append‑only) para análisis/auditoría futura.
- Observabilidad: OTLP (trazas/métricas/logs) hacia Prometheus, Loki y Jaeger; dashboards en Grafana.
```

**Observación**:  
MongoDB no está implementado en i0/i1. Debería aclararse que es "planificado" y no un requisito de V1 temprano.

**Corrección propuesta**:
```markdown
- **ETCD v3**: configuración central y declarativa con watches (desde i1).
- **PostgreSQL 16**: estado vivo, correlación `trade_id ↔ ticket(s)`, dedupe y auditoría operacional (desde i1).
- **MongoDB 7**: eventos crudos (append-only) para análisis/auditoría futura. **Planificado para i2+ (opcional)**.
- **Observabilidad**: OTLP (trazas/métricas/logs) hacia Prometheus, Loki y Jaeger; dashboards en Grafana (desde i0).
```

---

### 4.2 Calidad, seguridad y SLO (01-arquitectura-y-roadmap.md, líneas 94-98)

**Texto actual**:
```
- Latencia objetivo: p95 intra‑host < 100 ms (E2E), sin duplicados.
- Observabilidad obligatoria; errores siempre registrados con contexto y código normalizado.
- Seguridad V1: sin mTLS ni KMS; diseño listo para activarse en iteraciones posteriores.
```

**Observación**:  
La normalización de códigos de error ocurre en i11, no desde el inicio. "Código normalizado" no es correcto para i0/i1.

**Corrección propuesta**:
```markdown
- **Latencia objetivo**: p95 intra-host < 100 ms (E2E), sin duplicados.
- **Observabilidad obligatoria**: logs estructurados, métricas y trazas desde i0. Errores siempre registrados con contexto. Códigos de error normalizados desde i11.
- **Seguridad V1**: sin mTLS ni KMS en primera fase; diseño listo para activarse en iteraciones posteriores (fuera de alcance de V1 inicial).
```

---

## 5. Checklist de Validación para los Documentos Corregidos

Antes de reemplazar los documentos actuales, verificar:

- [ ] Todas las características presentadas como "presentes" están realmente implementadas en i0 o i1.
- [ ] Las características futuras (i2+) están claramente marcadas con su número de iteración.
- [ ] Las responsabilidades de cada componente son precisas y verificables en el código.
- [ ] Los contratos de mensajes coinciden con los protos en `sdk/proto/v1/*.proto`.
- [ ] El flujo de datos con timestamps es correcto y coincide con el código del Agent/Core.
- [ ] El roadmap tiene iteraciones pequeñas y enfocadas (una o dos características por iteración como máximo).
- [ ] No hay ambigüedades que puedan llevar a implementaciones incorrectas.

---

## 6. Recomendaciones Adicionales

### 6.1 Documentación de iteraciones futuras

Cuando se documenten iteraciones i2+, usar esta plantilla:

```markdown
---
title: "RFC-XXX — Iteración N: [Nombre Corto y Descriptivo]"
version: "1.0"
date: "YYYY-MM-DD"
status: "Draft | Proposed | Approved | Implemented"
authors: ["Equipo Echo"]
depends_on: ["RFC-YYY", "RFC-ZZZ"]
---

## 1. Resumen Ejecutivo
[Una frase describiendo QUÉ se va a implementar]

## 2. Objetivo Único y Medible
[Un solo objetivo claro, sin ambigüedades]

## 3. Alcance
- **Incluye**: [lista específica de lo que SÍ se implementa]
- **Excluye**: [lista específica de lo que NO se toca en esta iteración]

## 4. Cambios en Componentes
[Detallar cambios por componente: Core, Agent, SDK, EAs]

## 5. Contratos Afectados
[Solo si se cambian mensajes Proto o APIs]

## 6. Criterios de Salida (Bloqueantes)
- [ ] Criterio 1 medible y verificable
- [ ] Criterio 2 medible y verificable
- [ ] Sin regresiones en tests existentes

## 7. Métricas de Validación
[Cómo se medirá que la iteración cumplió su objetivo]
```

### 6.2 Nomenclatura consistente

- **Agent**: siempre con "A" mayúscula cuando se refiera al componente (no "agent").
- **Core**: siempre con "C" mayúscula.
- **Master EA** y **Slave EA**: capitalizar "EA".
- **trade_id**, **command_id**, **ticket**: siempre en `code style`.
- **UUIDv7**: siempre con "v7" en minúscula (no "UUIDv7" o "UUID v7").

### 6.3 Evitar referencias cruzadas

Dado que el usuario mencionó:
> "NO HAGAS REFERENCIAS A OTROS DOCUMENTOS PORQUE LOS ELIMINARÉ Y QUEDARÁN SOLO ESTOS DOS DE BASE"

Los documentos 00 y 01 deben ser **auto-contenidos**. Si necesitan referenciar algo de RFCs, **copiar la información relevante inline**.

---

## 7. Conclusión

Los documentos `00-contexto-general.md` y `01-arquitectura-y-roadmap.md` tienen una **base sólida**, pero requieren correcciones para ser precisos, inequívocos y útiles como referencia única.

### Principales problemas identificados:
1. **Mezcla de tiempo**: características presentes vs futuras sin distinción clara.
2. **Responsabilidades ambiguas**: sobre todo en Agent (¿aplica filtros? ¿hace routing local?).
3. **Iteraciones agrupadas**: i7 y i12 atacan demasiados puntos a la vez.
4. **Falta de contextualización**: UUIDv7, retries, backpressure presentados como si siempre existieran.

### Recomendaciones finales:
1. **Corregir** los puntos señalados en las secciones 1 y 2.
2. **Adoptar** el roadmap mejorado de la sección 3 (dividir i7 y i12).
3. **Validar** contra el código real antes de publicar.
4. **Usar** la plantilla de la sección 6.1 para iteraciones futuras.

Con estas correcciones, la documentación será una referencia **profesional, precisa y confiable** para el desarrollo de Echo V1.

---

**Revisión completada por**: Arquitecto Senior Expert  
**Fecha**: 2025-10-29  
**Siguiente paso**: Aplicar correcciones y regenerar documentos 00 y 01.

