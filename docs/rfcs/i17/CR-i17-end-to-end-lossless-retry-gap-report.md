# CR-i17 — Reporte de no implementación (Gap Analysis)

## Metadatos
- RFC base: `docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md`
- Implementación revisada: `docs/rfcs/i17/IMPLEMENTATION.md`
- Autor del reporte: Arquitecto Autor
- Fecha: 2025-11-18
- Estado: **Observado**

## 1. Resumen ejecutivo
Se revalidó la implementación entregada para i17, retomando los hallazgos detectados tras la ejecución real. Aunque la mayoría de los contratos y componentes descritos en el RFC están presentes (journal Postgres, ledger Bolt, acks por hop, `TradeIntentAck`/`PipeDeliveryAck`, métricas `echo.delivery.*`), persisten dos brechas críticas que impiden afirmar que “ningún mensaje se pierde”:

1. Los heartbeats gRPC continúan usando los valores legacy (60 s/20 s) en Core y Agent; no existe wiring que fuerce el rango 1–5 s solicitado por stakeholders para evitar caídas por keepalive agresivo.
2. Cuando el Core no logra persistir un comando en `delivery_journal`, el router sólo loguea el error y continúa la iteración sin backoff ni mecanismo para volver a intentar; cualquier `ExecuteOrder`/`CloseOrder` enviado durante una falla de Postgres se pierde definitivamente.

Se realizó además una segunda pasada (doble check) sobre contratos, watchers y clientes MT4 para descartar otros faltantes relevantes. No se encontraron diferencias adicionales respecto al RFC.

## 2. Hallazgos bloqueantes (GAP-DEV)

### H1 — Heartbeats gRPC fuera del rango 1–5 s
- **PR afectadas:** PR-ROB, PR-RES, PR-MVP (mandato explícito del RFC §Mandatos i17).
- **Evidencia técnica:** Tanto Core como Agent mantienen defaults de 60 s (time) / 20 s (timeout) y sólo permiten overrides vía las claves legadas `grpc/keepalive/*`; no existe la nueva configuración documentada para fijar 1 s ≤ heartbeat ≤ 5 s ni se actualizó `LoadConfig` para reflejarlo.

```156:166:core/internal/config.go
	cfg := &Config{
		GRPCPort:         50051,
		KeepAliveTime:    60 * time.Second,
		KeepAliveTimeout: 20 * time.Second,
		KeepAliveMinTime: 10 * time.Second,
```

```110:134:agent/internal/config.go
	cfg := &Config{
		...
		KeepAliveTime:       60 * time.Second,
		KeepAliveTimeout:    20 * time.Second,
		PermitWithoutStream: false,
```

- **Impacto:** El problema operativo descrito por los stakeholders (clientes derriban la conexión cuando el keepalive es muy frecuente) sigue sin ser abordado. Además, la documentación del RFC indicaba explícitamente “Heartbeats gRPC se fijan en 1–5 s…”, por lo que el DoD no se cumple.
- **Acción requerida:** Ajustar defaults y lectura de ETCD para que ambos binarios operen dentro del rango 1–5 s, documentando las nuevas claves o enforced defaults y asegurando que `startDeliveryHeartbeatLoop` y el cliente gRPC honren dichos valores.

### H2 — `DeliveryService` descarta órdenes cuando no puede persistir el journal
- **PR afectadas:** PR-ROB, PR-IDEMP, PR-RES.
- **Evidencia técnica:** Si `DeliveryService.ScheduleExecuteOrder` o `ScheduleCloseOrder` falla en `repo.Insert`, el router sólo registra el error y continúa. No hay fallback en memoria, ni requeue, ni señal hacia el Master/Agent de que la orden quedó pendiente: el flujo se pierde a pesar del objetivo “lossless”.

```803:812:core/internal/router.go
	if r.core.deliveryService != nil {
		if err := r.core.deliveryService.ScheduleExecuteOrder(ctx, ownerAgentID, order); err != nil {
			r.core.telemetry.Error(ctx, "Failed to schedule delivery", err,
				attribute.String("target_account_id", targetAccountID),
				attribute.String("agent_id", ownerAgentID),
			)
		} else {
			delivered = true
```

- **Impacto:** Un incidente transitorio en Postgres o en el repositorio `delivery_journal` reintroduce el mismo problema que i17 busca solucionar: órdenes perdidas sin visibilidad ni posibilidad de recuperación automática. No existe mecanismo que bloquee la iteración, reintente en memoria o notifique al Master para conservar el intent.
- **Acción requerida:** Implementar una política explícita cuando `Insert`/`Update` falle (por ejemplo, bloquear el fanout y reintentar, mover la orden a una cola in-memory, o, como mínimo, propagar error para que el Intent sea reinyectado). Mientras eso no exista, la garantía “cero pérdida” no se sostiene.

## 3. Doble verificación (sin nuevas observaciones)
Se revisaron nuevamente los siguientes aspectos para confirmar que el resto de la implementación coincide con el RFC:
- **Contratos y métricas:** `AgentHello/CoreHello`, `CommandAck`, `DeliveryHeartbeat`, `TradeIntentAck`, `PipeDeliveryAck`, `echo_core.delivery.compat_mode_total`, `echo_agent.pipe.delivery_latency_ms` y `echo_ea.trade_intent.buffer_depth` se encuentran declarados y utilizados.
- **Ledger y retries:** `delivery_journal`/`delivery_retry_event` (SQL + repos), `DeliveryService` + reconciliador, `DeliveryManager` + Bolt ledger y `MasterDeliveryConfig` + `delivery_config` en MT4 están operativos.
- **Acks por hop:** Core → Agent (`ScheduleExecuteOrder`/`HandleCommandAck`), Agent → EA (`HandlePipeResult` + `PipeDeliveryAck`), EA Slave → Agent (`SendPipeDeliveryAck`) y EA Master → Agent (`TradeIntentAck`) cumplen el flujo descrito.
- **Configuraciones compartidas:** `/echo/core/delivery/*` + `DeliveryHeartbeat` propagan los parámetros al Agent, y éste los refleja en el buffer del Master (via `BroadcastDeliveryConfig` + `HandleDeliveryConfig` en `master.mq4`).

No se detectaron inconsistencias adicionales en estos componentes.

## 4. Recomendación
Mantener la iteración i17 en estado **Observado** hasta que:
1. Se apliquen los cambios de keepalive (server + client) con evidencia en código y documentación.
2. Se defina y se implemente la política de retry/bloqueo ante fallos de persistencia del journal.

Una vez abordados ambos, repetir la verificación para cerrar formalmente la iteración.

