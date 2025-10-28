package utils

import (
	"time"
)

// NowUnixMilli retorna el timestamp actual en milisegundos desde Unix epoch.
//
// Compatible con GetTickCount() de MQL4 para sincronización de timestamps.
//
// Example:
//
//	ts := utils.NowUnixMilli()
//	// => 1698345601234
func NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

// NowUnixMicro retorna el timestamp actual en microsegundos desde Unix epoch.
//
// Útil para mediciones de latencia de alta precisión.
func NowUnixMicro() int64 {
	return time.Now().UnixMicro()
}

// UnixMilliToTime convierte un timestamp Unix en milisegundos a time.Time.
//
// Example:
//
//	ts := int64(1698345601234)
//	t := utils.UnixMilliToTime(ts)
func UnixMilliToTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}

// TimeToUnixMilli convierte un time.Time a timestamp Unix en milisegundos.
//
// Example:
//
//	t := time.Now()
//	ms := utils.TimeToUnixMilli(t)
func TimeToUnixMilli(t time.Time) int64 {
	return t.UnixMilli()
}

// ElapsedMs calcula los milisegundos transcurridos desde un timestamp dado.
//
// Example:
//
//	start := utils.NowUnixMilli()
//	// ... operación ...
//	elapsed := utils.ElapsedMs(start)
//	// => 45 (ms)
func ElapsedMs(startMs int64) int64 {
	return NowUnixMilli() - startMs
}

// ElapsedMsSince calcula los milisegundos transcurridos desde un time.Time dado.
//
// Example:
//
//	start := time.Now()
//	// ... operación ...
//	elapsed := utils.ElapsedMsSince(start)
func ElapsedMsSince(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

// TimestampMetadata representa metadatos de timestamp para latencia E2E.
// Compatible con el mensaje TimestampMetadata de proto.
type TimestampMetadata struct {
	T0MasterEAMs      int64 // Master EA genera intent
	T1AgentRecvMs     int64 // Agent recibe de pipe
	T2CoreRecvMs      int64 // Core recibe de stream
	T3CoreSendMs      int64 // Core envía ExecuteOrder
	T4AgentRecvMs     int64 // Agent recibe ExecuteOrder
	T5SlaveEARecvMs   int64 // Slave EA recibe comando
	T6OrderSendMs     int64 // Slave EA llama OrderSend
	T7OrderFilledMs   int64 // Slave EA recibe fill
}

// NewTimestampMetadata crea una nueva instancia de TimestampMetadata.
func NewTimestampMetadata() *TimestampMetadata {
	return &TimestampMetadata{}
}

// SetT0 establece el timestamp t0 (Master EA).
func (tm *TimestampMetadata) SetT0(ts int64) {
	tm.T0MasterEAMs = ts
}

// SetT1 establece el timestamp t1 (Agent recv).
func (tm *TimestampMetadata) SetT1(ts int64) {
	tm.T1AgentRecvMs = ts
}

// SetT2 establece el timestamp t2 (Core recv).
func (tm *TimestampMetadata) SetT2(ts int64) {
	tm.T2CoreRecvMs = ts
}

// SetT3 establece el timestamp t3 (Core send).
func (tm *TimestampMetadata) SetT3(ts int64) {
	tm.T3CoreSendMs = ts
}

// SetT4 establece el timestamp t4 (Agent recv execute order).
func (tm *TimestampMetadata) SetT4(ts int64) {
	tm.T4AgentRecvMs = ts
}

// SetT5 establece el timestamp t5 (Slave EA recv).
func (tm *TimestampMetadata) SetT5(ts int64) {
	tm.T5SlaveEARecvMs = ts
}

// SetT6 establece el timestamp t6 (Slave EA OrderSend).
func (tm *TimestampMetadata) SetT6(ts int64) {
	tm.T6OrderSendMs = ts
}

// SetT7 establece el timestamp t7 (Slave EA order filled).
func (tm *TimestampMetadata) SetT7(ts int64) {
	tm.T7OrderFilledMs = ts
}

// LatencyE2E calcula la latencia total E2E (t7 - t0) en milisegundos.
func (tm *TimestampMetadata) LatencyE2E() int64 {
	if tm.T7OrderFilledMs == 0 || tm.T0MasterEAMs == 0 {
		return 0
	}
	return tm.T7OrderFilledMs - tm.T0MasterEAMs
}

// LatencyAgentToCore calcula la latencia Agent → Core (t2 - t1).
func (tm *TimestampMetadata) LatencyAgentToCore() int64 {
	if tm.T2CoreRecvMs == 0 || tm.T1AgentRecvMs == 0 {
		return 0
	}
	return tm.T2CoreRecvMs - tm.T1AgentRecvMs
}

// LatencyCoreProcess calcula la latencia de procesamiento en Core (t3 - t2).
func (tm *TimestampMetadata) LatencyCoreProcess() int64 {
	if tm.T3CoreSendMs == 0 || tm.T2CoreRecvMs == 0 {
		return 0
	}
	return tm.T3CoreSendMs - tm.T2CoreRecvMs
}

// LatencyCoreToAgent calcula la latencia Core → Agent (t4 - t3).
func (tm *TimestampMetadata) LatencyCoreToAgent() int64 {
	if tm.T4AgentRecvMs == 0 || tm.T3CoreSendMs == 0 {
		return 0
	}
	return tm.T4AgentRecvMs - tm.T3CoreSendMs
}

// LatencySlaveExecution calcula la latencia de ejecución en Slave (t7 - t6).
func (tm *TimestampMetadata) LatencySlaveExecution() int64 {
	if tm.T7OrderFilledMs == 0 || tm.T6OrderSendMs == 0 {
		return 0
	}
	return tm.T7OrderFilledMs - tm.T6OrderSendMs
}

