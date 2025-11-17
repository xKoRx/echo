# RFC-13a — Estado real de implementación

## Fuentes revisadas
- Código Go en `core/internal/router.go`, `core/internal/config.go`, `core/internal/core.go`.
- Métricas en `sdk/telemetry/metricbundle/echo.go`.
- Pruebas unitarias en `core/internal/router_test.go`.
- Documentación de la iteración (`RFC-13a*.md`, `IMPLEMENTATION.md`).

## Funcionamiento observado en código

- **Worker pool y hashing por `trade_id`.** El router despacha cada mensaje a un worker calculado con `hash(trade_id)` y usa spans/metrics para registrar la operación. El orden FIFO se conserva dentro del worker porque la cola es un canal con buffer `queue_depth_max`.

```302:368:core/internal/router.go
func (r *Router) dispatchMessage(msg *routerMessage) {
	if msg == nil {
		return
	}

	if msg.tradeID == "" {
		r.processMessage(msg)
		return
	}

	worker := r.selectWorker(msg.tradeID)
	if worker == nil {
		r.core.telemetry.Warn(msg.ctx, "No workers available for router message",
			semconv.Echo.TradeID.String(msg.tradeID),
		)
		r.processMessage(msg)
		return
	}

	msg.workerID = worker.id
	msg.enqueued = time.Now()

	if r.enqueueWorker(worker, msg) {
		return
	}

	r.handleQueueRejection(msg)
}

func (r *Router) selectWorker(tradeID string) *routerWorker {
	if len(r.workers) == 0 {
		return nil
	}

	hash := hashTradeID(tradeID)
	index := 0
	if r.workerMask != 0 {
		index = int(hash & r.workerMask)
	} else {
		index = int(hash % uint32(len(r.workers)))
	}

	return r.workers[index]
}

func (r *Router) enqueueWorker(worker *routerWorker, msg *routerMessage) bool {
	ctx, span := r.core.telemetry.StartSpan(msg.ctx, "core.router.schedule")
	span.SetAttributes(
		attribute.Int("worker_id", worker.id),
		semconv.Echo.TradeID.String(msg.tradeID),
	)
	defer span.End()

	select {
	case worker.queue <- msg:
		r.recordQueueDepthDelta(ctx, worker.id, 1)
		r.core.telemetry.Info(ctx, "core.router.enqueue",
			attribute.Int("worker_id", worker.id),
			semconv.Echo.Component.String(routerComponentValue),
			semconv.Echo.TradeID.String(msg.tradeID),
			attribute.Int("queue_depth", len(worker.queue)),
		)
		return true
	default:
		return false
	}
}
```

- **Backpressure determinista.** Cuando la cola del worker se llena se marca el trade como rechazado, se persiste el estado y se envía un `Ack` con `ERROR_CODE_BROKER_BUSY`.

```370:520:core/internal/router.go
func (r *Router) handleQueueRejection(msg *routerMessage) {
	queueDepth := r.currentQueueDepth(msg.workerID)
	attrs := []attribute.KeyValue{
		semconv.Echo.Component.String(routerComponentValue),
		attribute.Bool("backpressure", true),
		attribute.Int("worker_id", msg.workerID),
		attribute.Int("queue_depth", queueDepth),
	}
	if msg.tradeID != "" {
		attrs = append(attrs, semconv.Echo.TradeID.String(msg.tradeID))
	}

	r.core.telemetry.Warn(msg.ctx, "Router queue full, backpressure engaged", attrs...)
	if r.core.echoMetrics != nil {
		r.core.echoMetrics.RecordRouterRejection(msg.ctx, msg.workerID, "queue_full")
	}

	switch payload := msg.agentMsg.Payload.(type) {
	case *pb.AgentMessage_TradeIntent:
		r.rejectTradeIntentBackpressure(msg, payload.TradeIntent, queueDepth)
	default:
		r.processMessage(msg)
	}
}

func (r *Router) rejectTradeIntentBackpressure(msg *routerMessage, intent *pb.TradeIntent, queueDepth int) {
	// ... dedupe + persistencia ...
	r.sendBackpressureAck(ctx, msg.agentID, tradeID, queueDepth, msg.workerID)
}
```

```554:619:core/internal/router.go
func (r *Router) sendBackpressureAck(ctx context.Context, agentID, tradeID string, queueDepth, workerID int) {
	// Construye Ack con ERROR_CODE_BROKER_BUSY y reintenta cada worker_timeout
	for {
		select {
		case agent.SendCh <- ack:
			r.core.telemetry.Info(ctx, "Backpressure ack sent to agent",
				attribute.String("agent_id", agentID),
				semconv.Echo.Component.String(routerComponentValue),
				semconv.Echo.TradeID.String(tradeID),
				attribute.Bool("backpressure", true),
				attribute.Int("worker_id", workerID),
				attribute.Int("queue_depth", queueDepth),
			)
			return
		case <-ctx.Done():
			return
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			// sigue intentando
		}
	}
}
```

- **Configuración única via ETCD.** `RouterConfig` se llena al boot leyendo claves `core/router/*` y se valida (power-of-two, límites de profundidad y timeout) antes de exponer los valores al router.

```336:519:core/internal/config.go
if val, err := etcdClient.GetVarWithDefault(ctx, "core/router/worker_pool_size", ""); err == nil && val != "" {
	if size, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
		cfg.Router.WorkerPoolSize = size
	} else {
		return nil, fmt.Errorf("invalid core/router/worker_pool_size: %w", err)
	}
}
// ... queue_depth_max y worker_timeout_ms ...
if cfg.Router.WorkerPoolSize < 2 || cfg.Router.WorkerPoolSize > 32 {
	return nil, fmt.Errorf("core/router/worker_pool_size must be between 2 and 32")
}
if !isPowerOfTwo(cfg.Router.WorkerPoolSize) {
	return nil, fmt.Errorf("core/router/worker_pool_size must be a power of two")
}
if cfg.Router.QueueDepthMax < 4 || cfg.Router.QueueDepthMax > 128 {
	return nil, fmt.Errorf("core/router/queue_depth_max must be between 4 and 128")
}
if cfg.Router.WorkerTimeout <= 0 {
	cfg.Router.WorkerTimeout = 50 * time.Millisecond
}
timeoutMs := cfg.Router.WorkerTimeout / time.Millisecond
if timeoutMs < 20 || timeoutMs > 200 {
	return nil, fmt.Errorf("core/router/worker_timeout_ms must be between 20 and 200")
}
```

- **Métricas y spans específicos del router.** El bundle `EchoMetrics` expone contadores e histogramas dedicados y los helpers `RecordRouter*` añaden automáticamente `component="core.router"` y `worker_id`, alineado con lo pedido en el RFC.

```421:456:sdk/telemetry/metricbundle/echo.go
routerQueueDepth, err := meter.Int64UpDownCounter(
	"echo_core_router_queue_depth",
	metric.WithDescription("Mensajes pendientes en la cola del worker del router"),
)
routerDispatch, err := meter.Int64Counter(
	"echo_core_router_dispatch_total",
	metric.WithDescription("Procesamientos del router por resultado y worker"),
)
routerDispatchDuration, err := meter.Float64Histogram(
	"echo_core_router_dispatch_duration_ms",
	metric.WithDescription("Tiempo desde el enqueue hasta que el worker procesa el mensaje"),
)
routerRejections, err := meter.Int64Counter(
	"echo_core_router_rejections_total",
	metric.WithDescription("Rechazos del router por backpressure"),
)
```

## Diferencias frente al RFC-13a
- **Backpressure inmediato vs. criterios operativos definidos.** El RFC (§3 DoD y §5.3) exige que `queue_depth_max` se considere violado solo si la cola permanece por encima del límite >5 s y que el router rechace hasta volver al 80 % del umbral. El código rechaza al primer intento fallido de `enqueueWorker` sin medir duración ni histéresis.
- **Timeouts sin reintentos reales.** La sección 5.1 paso 5 del RFC indica que cada worker debe aplicar `worker_timeout_ms` contra el Agent y reinsertar el comando en retry. `sendToAgent` simplemente devuelve `false` ante timeout y el caller abandona la orden; no existe requeue automático ni reporte adicional.
- **Cola global sin visibilidad ni manejo.** El RFC no menciona una cola previa al worker pool, pero el código usa `processCh` (buffer 1000) antes del hash. Ese buffer no publica métricas ni aplica backpressure, por lo que puede perder mensajes silenciosamente sin alinearse con los criterios de rechazo documentados.
- **DoD de métricas no implementado.** Aunque las métricas existen, no hay lógica (en código ni scripts) que compute p95, supervise `queue_depth` o dispare alertas como se detalla en los objetivos medibles del RFC.

## Riesgos técnicos detectados
- **Pérdida de mensajes al saturar `processCh`.** `HandleAgentMessage` usa un `select` non-blocking y, si el canal interno se llena, descarta el mensaje con un log pero sin dedupe, métricas ni `Ack`, dejando al master esperando indefinidamente.

```229:255:core/internal/router.go
func (r *Router) HandleAgentMessage(ctx context.Context, agentID string, msg *pb.AgentMessage) {
	tradeID := r.extractTradeID(msg)

	select {
	case r.processCh <- &routerMessage{ ... }:
	case <-r.ctx.Done():
		r.core.telemetry.Warn(r.ctx, "Router stopped, message dropped",
			attribute.String("agent_id", agentID),
		)
	default:
		r.core.telemetry.Error(r.ctx, "Router queue full, message dropped", nil,
			attribute.String("agent_id", agentID),
		)
	}
}
```

- **Timeouts hacia el Agent pierden la orden.** Cuando el `SendCh` del Agent está bloqueado, `sendToAgent` retorna `false` tras `worker_timeout` y el flujo del TradeIntent no reenfila la orden ni hace fallback, rompiendo la garantía de entrega declarada en el RFC.

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

- **Métricas pueden contar rechazos que sí se procesan.** `handleQueueRejection` registra un rechazo para cualquier mensaje sin distinguir si luego se procesa inline (caso ExecutionResult/TradeClose), inflando `echo_core_router_rejections_total` y dificultando diagnósticos.

## Cobertura de pruebas
- `core/internal/router_test.go` verifica únicamente helpers (`extractTradeID`, `hashTradeID`) y dos casos de backpressure (persistencia + Ack, reintento de Ack). No hay pruebas de hashing concurrente, saturación de workers, ni del path `processCh` ⇒ drop, por lo que los escenarios principales descritos en el RFC siguen sin validación automatizada.

```79:190:core/internal/router_test.go
func TestSendBackpressureAckRetriesUntilChannelAvailable(t *testing.T) { ... }
func TestRejectTradeIntentBackpressurePersistsAndSendsAck(t *testing.T) { ... }
```

## Recomendaciones inmediatas
- Propagar backpressure también para la cola global `processCh` (reutilizando la lógica existente) o hacerla bloqueante para preservar orden/esfuerzo del master.
- Implementar reintentos reales tras `worker_timeout` (requeue o fallback explícito) y exponer métricas de retry, tal como exige el RFC.
- Añadir un control de histéresis sobre `queue_depth` para cumplir los objetivos medibles (alertar al cruzar 80 % por más de N segundos en lugar de rechazar al primer overflow puntual).
- Expandir las pruebas automatizadas para cubrir hashing determinista, saturación de colas, `sendToAgent` fallando y el flujo de métricas.

