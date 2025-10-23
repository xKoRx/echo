---
title: "PRD — Copiador de Operaciones Algorítmico (MVP V1)"
version: "1.0"
date: "2025-10-23"
owner: "Aranea Labs — Trading Copiador"
status: "Draft"
---

# 1. Resumen ejecutivo

MVP local para copiar **solo operaciones a mercado** desde **masters MT4** hacia **slaves MT4/MT5**, con **replicación de MagicNumber**, **modo hedged** siempre, **Money Management central en el core**, **SL/TP opcionales con tolerancia en points/pips**, y **ventanas de no-ejecución** manuales para evitar operar en noticias. Sin seguridad en V1. Observabilidad total vía OpenTelemetry.

Alcance inicial: **3–6 masters** y **~15 slaves** en red local (10 GbE). Latencia objetivo **< 1 s** extremo a extremo; preferible **< 100 ms** intra-host con agente local.

---

# 2. Objetivos y no‑objetivos

## 2.1 Objetivos
- Replicar **entradas a mercado** y **cierres** del master en múltiples slaves con divergencia controlada.
- Mantener **MagicNumber idéntico** en los slaves para trazabilidad con brokers/prop firms.
- Administrar **Money Management** por **cuenta × estrategia** de forma central en el core.
- Aplicar **ventanas de no‑ejecución** por cuenta/símbolo para evitar entradas alrededor de noticias.
- Telemetría completa: **métricas, logs y trazas** en core, agente y clientes.
- Configuración **centralizada** en el core y declarativa.

## 2.2 No‑objetivos (V1)
- No soportar **órdenes pendientes** (limit/stop), ni re‑inserción de pendientes post‑ventana.
- No implementar **reintentos inteligentes** por slippage/spread (V1 = “omit with comment”).
- No hacer **replay determinista** de estado (sin event sourcing).
- Sin **seguridad** avanzada (sin mTLS, sin KMS).

---

# 3. Alcance funcional V1

1) **Entradas a mercado**: copiar en slaves con **tolerancia de desvío** configurable y **filtro de spread** máximo.  
2) **SL/TP en slaves**: opcionales. Si se usan, aplicar **offset configurable** para reducir stops prematuros y respetar **StopLevel**. Si el broker rechaza por distancia mínima, colocar SL/TP vía **modificación posterior**.  
3) **Cierres**: copiar **todo cierre** del master (manual, SL/TP, TP manual, cierre por señal). Ventanas bloquean **entradas**; los **cierres no se bloquean**.  
4) **Missed trades**: si el slave no entra por precio o spread, **no reentrar** en V1. Registrar evento y métrica.  
5) **Modificaciones del master**: si el master cambia SL/TP y están habilitados en slave, reflejar ajuste con el **mismo offset**.  
6) **MagicNumber**: replicar **sin cambios** en cada trade del slave.  
7) **Hedge**: **solo cuentas hedged** en V1, incluso en MT5.  
8) **Ventanas de no‑ejecución**: bloquear **nuevas entradas**; al iniciar, **cancelar pendientes** heredadas; al cerrar, **no reinsertar** (V1).  
9) **SL catastrófico**: configurable por cuenta/estrategia. Cierre del slave seguirá al master, pero el SL duro protege en contingencias.  
10) **Configuración en core**: por **account_id** y **strategy_id** (MagicNumber) con toggles on/off, tolerancias, offsets, límites de DD, filtros de spread, buffers de ventana, etc.

---

# 4. Requisitos no funcionales

- **Latencia**: objetivo < 1 s extremo a extremo; intra-host con agente < 100 ms.
- **Disponibilidad**: operación local; no se exige HA en V1, pero el core debe reiniciar y **reconstruir estado** desde snapshots del agente.
- **Escalabilidad**: diseño modular para escalar a más masters/slaves y nuevos clientes en V2+.
- **Observabilidad**: OTEL end‑to‑end. Dashboards en Grafana. Logs estructurados en Loki. Trazas en Jaeger.
- **Confiabilidad**: idempotencia por `trade_id` global y deduplicación en core.
- **Seguridad**: **desactivada** en V1 salvo TLS opcional. Diseño listo para activar más adelante.
- **Portabilidad**: despliegue local con Docker/Compose en red privada.

---

# 5. Métricas clave

- `latency_e2e_ms` entrada/cierre.  
- `missed_trades_count` y ratio por cuenta/estrategia.  
- `avg_slippage_points` y `max_slippage_points`.  
- `spread_at_entry_points`.  
- `orders_rejected_count` por motivo (StopLevel, spread, desvío).  
- `blocked_by_window_count`.  
- `policy_violations_count` (DD diario, riesgo por trade idea, etc.).  
- `agent_tick_coalesce_ms` y uso de CPU/RAM del agente.

---

# 6. Supuestos y restricciones

- Red local confiable (10 GbE). Sin colas persistentes en V1.  
- Brokers y props permiten **hedged**.  
- Masters en MT4; slaves en MT4/MT5.  
- No hay WSL2 requerido. Todo nativo Windows en hosts de terminales.  
- Sin requisitos de PII o cifrado en reposo.  
- TZ: core opera en **UTC interno**; los cortes diarios se definen **por broker/tipo de cuenta**.

---

# 7. Stack tecnológico

- **Lenguajes**: Go (core, agente), MQL4/MQL5 (EAs).  
- **Contratos**: Protobuf + **gRPC bidi‑streaming** core↔agente. REST read‑only vía grpc‑gateway.  
- **Configuración**: **etcd v3** con watches para políticas y ventanas.  
- **Persistencia**: **Postgres 16** (estado vivo, políticas, catálogos, calendario JSONB). **MongoDB 7** (eventos crudos).  
- **Cache**: **Ristretto** in‑process.  
- **Observabilidad**: **OpenTelemetry SDK Go** → OTLP → **Prometheus**, **Loki**, **Jaeger**; **Grafana**.  
- **Windows host**: **Agente único por máquina**. IPC con EAs vía **Named Pipes**.  
- **Colectores**: `windows_exporter` para métricas de sistema, **Promtail Windows** para logs, **OTel Collector** opcional.  
- **Formato de feeds**: JSON para calendario y mapeos.  
- **Identidad/Idempotencia**: **UUIDv7** `trade_id` + `attempt`, `source_master_id`, `magic_number`, `strategy_id` lógico.

---

# 8. Componentes y responsabilidades

## 8.1 Core (Go)
- Orquestación de copiado y **Money Management** central.  
- Políticas por **prop firm** y por **cuenta×símbolo** (riesgo por trade idea, DD diario, DD total, horario de cierre, profit target).  
- **Ventanas de no‑ejecución**: evaluación y aplicación; **cola de órdenes diferidas** solo para entradas de mercado diferidas por latencia interna (no pendientes).  
- **Idempotencia/dedupe** y backpressure.  
- **Catálogo de símbolos**: `canonical_symbol` y validación de specs reportadas por cada slave.  
- **Estado réplica** por cuenta: posiciones, órdenes, equity, margen, último tick relevante.  
- **Sizing en slave**: con ATR del master y specs locales del slave; respeta lot step y min volume.  
- **APIs**: gRPC bidi para órdenes/eventos/estado; REST lectura.  
- **Persistencia**: Postgres (estado/políticas/calendario JSONB), Mongo (eventos crudos).  
- **Configuración live**: etcd watches.  
- **TZ**: lógica interna en UTC; cortes por broker/tipo de cuenta.

## 8.2 Agente por máquina (Go, Windows Service)
- **Un solo agente por host** que sirve a **múltiples terminales** MT4/MT5, masters y slaves.  
- Conexión **gRPC bidi** al core.  
- **IPC Named Pipes** con EAs: commands in, events out.  
- Ejecución: market in/out, modify SL/TP, close parcial; aplicación de **filtros de spread/desvío**.  
- Reporte hacia el core: coalesce por tick ~~250 ms de **equity**, **balance**, **margen**, **posiciones/órdenes**, último tick y **specs** por símbolo.  
- Telemetría: métricas/logs/trazas del agente y de cada EA.  
- Regla V1 ante desvío/spread: **omit with comment**.

## 8.3 Cliente Master (EA MQL4/MQL5)
- Publica **intents** de entrada a mercado con metadatos: `magic_number`, `symbol`, `TF`, ATR, SL/TP recomendados, `strategy_id`.  
- Emite **eventos de cierre** y modificaciones de SL/TP del master.  
- **No** calcula MM. **No** evalúa ventanas.

## 8.4 Cliente Slave (EA MQL4/MQL5)
- Ejecuta comandos del agente: **market buy/sell**, modify SL/TP, **close parcial/total**.  
- Reporta: ticks mínimos, estado de cuenta, posiciones y órdenes.  
- Expone **specs del símbolo** del broker y mapeo `broker_symbol ⇄ canonical_symbol`.  
- Mantiene **MagicNumber igual** al del master en cada trade.

---

# 9. Lógica funcional V1 (solo mercado)

## 9.1 Entrada
- Validar **ventanas** y **políticas** de la cuenta.  
- Verificar **spread máximo** y **desvío** respecto al precio del master.  
- Si pasa filtros: enviar **market order** con tamaño calculado.  
- **SL/TP**: opcionales con **offset**. Si StopLevel impide, colocar por **modificación** tras el fill.  
- Si no pasa filtros: **omit with comment**.

## 9.2 Cierre
- Cerrar en slave cuando el master cierre, independiente de ventanas.

## 9.3 Modificaciones
- Si SL/TP están activos en slave y el master los cambia, reflejar cambio con **mismo offset**.

## 9.4 Missed trade / Stop‑out
- No reabrir. Registrar evento y métricas. Alertar.

---

# 10. Datos y contratos (alto nivel, sin código)

- **Orden**: `trade_id`, `magic_number`, `source_master_id`, `canonical_symbol`, `broker_symbol`, `side`, `size`, `price_master`, `ts_master`, `tolerances`, `sl/tp` (si aplica), `account_id`, `strategy_id`, `attempt`.
- **Evento de estado**: posición/orden por cuenta, equity, margen, ticks coalescidos, specs de símbolo.
- **Política**: JSONB por cuenta×símbolo: límites de riesgo, DD, spread/desvío, SL catastrófico, ventanas, offsets SL/TP.
- **Calendario**: JSON POST al core con `scope` (cuenta/símbolo), `start/end` UTC, `pre/post` buffers, `notes`.

---

# 11. Configuración y catálogos

- **etcd v3**: claves versionadas para políticas, ventanas, toggles por cuenta/estrategia.  
- **Postgres**: `accounts`, `strategies` (por MagicNumber), `symbols`, `broker_symbols`, `policies`, `news_windows`, `orders_state`, `positions_state`.  
- **Mongo**: `events` (append‑only) para auditoría futura.  
- **Mapeo símbolos**: el agente reporta el mapeo y specs al conectar.

---

# 12. Observabilidad

- **Métricas** de core y agente vía OTEL → Prometheus.  
- **Logs** estructurados a Loki: nivel, causa de rechazo, spread/desvío, StopLevel.  
- **Trazas** de órdenes y latencia E2E en Jaeger.  
- **Dashboards** en Grafana para latencia, fill rate, misses, rechazo por políticas, uso agente.

---

# 13. Criterios de aceptación V1

- Copia **100%** de **entradas a mercado** y **cierres** de 3–6 masters hacia ~15 slaves con MagicNumber replicado.  
- **Ventanas** bloquean entradas y cancelan pendientes heredadas; cierres no bloqueados.  
- **Tolerancias** aplicadas: spread y desvío; rechazos logueados con motivo.  
- **SL catastrófico** aplicable por cuenta/estrategia.  
- Telemetría operativa disponible en Grafana/Jaeger/Loki.  
- Re‑arranque del core reconstruye estado desde reportes del agente sin duplicar órdenes.

---

# 14. Riesgos y mitigaciones

- **StopLevel** impide SL/TP con offset → colocar por modificación posterior.  
- **Unidades** (pips vs points) → estandarizar en **points** y usar tick value por símbolo.  
- **Desfase horario** → UTC interno; corte diario por hora de broker.  
- **Divergencia PnL** por precios distintos → métricas de slippage y spread, tolerancias y offsets.  
- **Alta frecuencia de modificaciones** → coalescing de eventos y límites de tasa en agente/core.

---

# 15. Roadmap V2+ (no comprometido)

- **Órdenes pendientes** y re‑inserción post‑ventana.  
- **Reintentos/price‑chase** con límites.  
- **Reingreso** de missed trade si el master sigue abierto.  
- **Seguridad**: mTLS, tokens cortos, roles, rotación.  
- **Event sourcing** y bus interno (NATS/JetStream).  
- **Front web** para administración y multi‑tenant opcional.  
- **Soporte de clientes**: NinjaTrader, IB, cTrader/eToro bots.  
- **SLA y HA** del core.  
- **Cola local ligera** en agente para tolerar cortes breves.  
- **Políticas avanzadas** por prop firm y “trade idea” multi‑cuenta.

---

# 16. Decisiones cerradas (V1)

- Transporte core↔agente: **gRPC bidi‑streaming**.  
- IPC EA↔agente: **Named Pipes**.  
- Persistencia: **Postgres (estado)** + **Mongo (eventos)**.  
- Config: **etcd v3**.  
- Observabilidad: **OTEL → Prometheus/Loki/Jaeger**.  
- Hedged only; **MagicNumber replicado**.  
- **Solo mercado**; SL/TP opcionales con offset; **omit with comment** ante desvío/spread.
