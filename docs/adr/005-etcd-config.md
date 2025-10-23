# ADR-005: etcd para Configuración Live

## Estado
**Aprobado** - 2025-10-23

## Contexto

Echo requiere configuración **dinámica** que pueda cambiar sin reiniciar servicios:

- Políticas de trading (spread, slippage, risk%)
- Ventanas de no-ejecución (noticias)
- Toggles (activar/desactivar cuentas)
- Límites (DD, riesgo)

Opciones consideradas:

1. **etcd v3**
2. **Archivos YAML/JSON** + hot-reload
3. **PostgreSQL** (tabla de config)
4. **Consul**

## Decisión

Usaremos **etcd v3** con **watches**.

### Estructura de Claves

```
/echo/
  /core/
    /master_accounts       → ["MT4-001", "MT4-002"]
    /slave_accounts        → ["MT5-001", "MT5-002"]
  /policy/
    /{account_id}/
      /max_spread          → 5.0
      /max_slippage        → 3.0
      /copy_sl_tp          → true
      /risk_per_trade      → 2.0
  /windows/
    /{account_id}/{symbol}/
      /start_utc           → "2025-10-23T14:30:00Z"
      /end_utc             → "2025-10-23T15:00:00Z"
```

## Consecuencias

### Positivas
- ✅ **Watch API**: Cambios en tiempo real sin polling
- ✅ **Consistencia**: Linearizable reads (strong consistency)
- ✅ **HA**: Cluster con Raft (3-5 nodos)
- ✅ **Lease/TTL**: Ventanas temporales auto-expirables
- ✅ **Transacciones**: CAS (compare-and-swap) atómico
- ✅ **API simple**: gRPC nativo

### Negativas
- ⚠️ **Dependencia externa**: Cluster etcd separado
- ⚠️ **Operacional**: Requiere monitoreo y backups
- ⚠️ **Límite de tamaño**: No para datos grandes (payload <1MB)

### Alternativas Descartadas

**YAML/JSON + hot-reload**:
- ❌ No watch nativo (requiere file watcher)
- ❌ No transacciones
- ❌ No HA nativa
- ❌ Race conditions al escribir

**PostgreSQL**:
- ✅ Ya tenemos Postgres
- ❌ Sin watch nativo (requiere polling o LISTEN/NOTIFY)
- ❌ Overhead de queries
- ❌ No optimizado para config frecuente

**Consul**:
- ✅ Similar a etcd
- ❌ Más pesado (service mesh features innecesarios)
- ❌ Menos usado en Go ecosystem

## Implementación

### Cliente

```go
import clientv3 "go.etcd.io/etcd/client/v3"

etcdClient, err := clientv3.New(clientv3.Config{
    Endpoints: []string{"localhost:2379"},
    DialTimeout: 5 * time.Second,
})
defer etcdClient.Close()
```

### Leer Config

```go
resp, err := etcdClient.Get(ctx, "/echo/policy/MT4-001/max_spread")
if err != nil {
    return err
}
maxSpread := string(resp.Kvs[0].Value)
```

### Escribir Config

```go
_, err := etcdClient.Put(ctx, "/echo/policy/MT4-001/max_spread", "5.0")
```

### Watch (Reactividad)

```go
watchChan := etcdClient.Watch(ctx, "/echo/policy/", clientv3.WithPrefix())

for resp := range watchChan {
    for _, ev := range resp.Events {
        log.Printf("Key: %s, Value: %s, Type: %s", 
            ev.Kv.Key, ev.Kv.Value, ev.Type)
        // Aplicar nueva configuración en caliente
        applyPolicyUpdate(string(ev.Kv.Key), string(ev.Kv.Value))
    }
}
```

### TTL para Ventanas

```go
lease, err := etcdClient.Grant(ctx, 300) // 300 segundos
_, err = etcdClient.Put(ctx, "/echo/windows/MT4-001/XAUUSD/active", "true",
    clientv3.WithLease(lease.ID))
```

## Backup

- **Snapshot**: `etcdctl snapshot save`
- **Frecuencia**: Diaria
- **Almacenamiento**: S3 o equivalente

## Observabilidad

Métricas:
- `etcd.server.leader_changes`
- `etcd.network.client.grpc.received.bytes`
- `etcd.disk.wal.fsync.duration`

## Migración Progresiva (Iteración 0 → Iteración 2)

### Iteración 0
- Postgres para todo (incluido config)

### Iteración 1
- etcd para políticas hot (spread, slippage)
- Postgres para catálogos estáticos

### Iteración 2
- etcd como fuente de verdad para toda config dinámica
- Postgres solo para estado y catálogos

## Seguridad

- **V1**: Sin auth
- **V2+**: TLS + client certs, RBAC roles

## Referencias
- [etcd Documentation](https://etcd.io/docs/)
- [etcd Go Client](https://github.com/etcd-io/etcd/tree/main/client/v3)
- [RFC-001](../RFC-001-architecture.md#42-configuraci%C3%B3n-en-etcd)

