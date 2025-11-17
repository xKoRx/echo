# DT Iteración 13a — Paralelismo seguro por slave

## Resumen ejecutivo
- La implementación actual del router paralelo introduce un worker pool y métricas básicas, pero incumple condiciones críticas del RFC-13a (ventana de backpressure, ratio de éxito y p95).
- Existen riesgos de pérdida silenciosa de mensajes, retries inexistentes y métricas incompletas que impiden observar el estado real del fanout.
- i13b depende de que estas brechas se cierren: no se pueden definir límites “explícitos” si la capa base no respeta los umbrales acordados en 13a.

## Deudas técnicas principales

### 1. Cola global (`processCh`) sin control de backpressure
- **Código:**  

```229:255:core/internal/router.go
func (r *Router) HandleAgentMessage(ctx context.Context, agentID string, msg *pb.AgentMessage) {
	tradeID := r.extractTradeID(msg)

	select {
	case r.processCh <- &routerMessage{
		ctx:      ctx,
		agentID:  agentID,
		agentMsg: msg,
		tradeID:  tradeID,
	}:
	case <-r.ctx.Done():
		// Router detenido
		r.core.telemetry.Warn(r.ctx, "Router stopped, message dropped",
			attribute.String("agent_id", agentID),
		)
	default:
		// Canal lleno (no debería pasar con buffer de 1000)
		r.core.telemetry.Error(r.ctx, "Router queue full, message dropped", nil,
			attribute.String("agent_id", agentID),
		)
	}
}
```
- **Impacto:** cuando `processCh` se llena (1000 slots) los mensajes se descartan sin dedupe, métricas ni `Ack`, rompiendo la garantía de entrega.
- **Requisito incumplido:** RFC-13a exige rechazos controlados y trazables cuando se excede `queue_depth_max` durante >5 s.
- **Acción recomendada:** eliminar el `default` (bloquear hasta tener capacidad) o reutilizar la lógica de `handleQueueRejection`, incluyendo dedupe y `Ack`.

### 2. Falta de retries tras `worker_timeout_ms`
- **Código:**  

```2387:2419:core/internal/router.go
func (r *Router) sendToAgent(ctx context.Context, agent *AgentConnection, msg *pb.CoreMessage, order *pb.ExecuteOrder) bool {
	timeout := time.NewTimer(r.workerTimeout)
	defer timeout.Stop()

	select {
	case agent.SendCh <- msg:
		return true
	case <-timeout.C:
		r.core.telemetry.Warn(ctx, "Timeout sending to Agent, channel may be full (i2b)",
			attribute.String("agent_id", agent.AgentID),
			attribute.String("command_id", order.CommandId),
			attribute.String("target_account_id", order.TargetAccountId),
		)
		return false
	case <-ctx.Done():
		r.core.telemetry.Error(ctx, "Context cancelled while sending ExecuteOrder", ctx.Err(),
			attribute.String("agent_id", agent.AgentID),
			attribute.String("command_id", order.CommandId),
		)
		return false
	}
}
```
- **Impacto:** si el Agent está lento, la orden se pierde; no existe requeue ni métrica de retry, lo que invalida el p95 ≤ 40 ms requerido.
- **Acción recomendada:** al recibir `false`, reenfilar el mensaje (con límite de intentos) y exponer `echo_core_router_retry_total`.

### 3. Rechazo inmediato sin ventana >5 s / 80 %
- **Código:**  

```302:393:core/internal/router.go
func (r *Router) dispatchMessage(msg *routerMessage) {
	// ...
	if r.enqueueWorker(worker, msg) {
		return
	}
	r.handleQueueRejection(msg)
}
```
- **Impacto:** un pico momentáneo dispara rechazos en cascada, alejándose del comportamiento “controlado” descrito en la Definition of Done.
- **Acción recomendada:** medir `queue_depth` con un temporizador; solo activar `handleQueueRejection` si el worker excede `queue_depth_max` durante N segundos y establecer histéresis (ej. dejar de rechazar cuando baje de 80 %).

### 4. Métricas sin enforcement de p95 / ratio ≥99 %
- **Observación:** se registran histogramas y counters (`echo_core_router_dispatch_duration_ms`, `echo_core_router_dispatch_total`), pero no existe lógica que valide p95 ≤ 40 ms o que calcule el ratio de éxito por ventana de 5 minutos.
- **Impacto:** el DoD queda sin verificación objetiva; operaciones no pueden saber si el fanout cumple SLA.
- **Acción recomendada:** añadir un job interno (o exportar métricas derivadas) que calcule percentiles y ratios; incluir alertas en dashboards antes de i14.

### 5. Cobertura insuficiente
- **Código de test actual:**  

```79:190:core/internal/router_test.go
func TestSendBackpressureAckRetriesUntilChannelAvailable(t *testing.T) { ... }
func TestRejectTradeIntentBackpressurePersistsAndSendsAck(t *testing.T) { ... }
```
- **Impacto:** no hay pruebas sobre hashing determinista bajo carga, saturación de `processCh`, retries fallidos ni métricas DoD.
- **Acción recomendada:** crear suites table-driven que simulen bursts de trades, Agent slow-path y verifiquen métricas/acks.

## Relación con i13b
- El RFC-13a indica que i13b se enfocará en “límites explícitos de backpressure y métricas de cola” (sección 14). Eso presupone que 13a ya entregó una base funcional con retries y control temporal. Las deudas listadas deben resolverse primero para que i13b se pueda limitar a ajustes de configuración y dashboards.

## Próximos pasos propuestos
| Prioridad | Acción | Responsable sugerido |
|-----------|--------|----------------------|
| Alta | Aplicar lógica de backpressure al buffer global o hacerlo bloqueante | Core |
| Alta | Implementar requeue tras `worker_timeout_ms` con métrica de retries | Core |
| Media | Añadir histéresis (>5 s, 80 %) antes de rechazar por `queue_depth_max` | Core/Arquitectura |
| Media | Instrumentar cálculo de p95 y ratio de éxito (alertas) | Observabilidad |
| Media | Ampliar cobertura de `router_test.go` con escenarios concurrentes | QA/Core |

## Referencias
- `docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md`
- `core/internal/router.go`
- `core/internal/router_test.go`
- `sdk/telemetry/metricbundle/echo.go`

