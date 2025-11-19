#!/bin/bash
set -e

# Configurar endpoints
ETCD_ENDPOINT="localhost:2379"
ENV="development"

echo "🚀 Iniciando actualización de protocolo ETCD..."

# Función helper
put_key() {
    local key=$1
    local value=$2
    echo "Writing $key = $value"
    ETCDCTL_API=3 etcdctl --endpoints=$ETCD_ENDPOINT put "echo/$ENV/$key" "$value"
}

# 1. Actualizar Core Protocol
put_key "core/protocol/min_version" "1"
put_key "core/protocol/max_version" "3"
put_key "core/protocol/blocked_versions" "" 

# 2. Actualizar Agent Protocol
put_key "agent/protocol/min_version" "1"
put_key "agent/protocol/max_version" "3"
put_key "agent/protocol/allow_legacy" "true"

# 3. Configurar Delivery (i17 requirements)
put_key "core/delivery/heartbeat_interval_ms" "1000"
put_key "agent/delivery/ack_timeout_ms" "150"
put_key "agent/delivery/max_retries" "100"

echo "✅ Configuración actualizada exitosamente."
echo "🔄 REINICIA CORE Y AGENT PARA APLICAR CAMBIOS."
