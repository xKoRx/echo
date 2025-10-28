#!/bin/bash
set -e

echo "Generating protobuf code..."

# Verificar que buf esté instalado
if ! command -v buf &> /dev/null; then
    echo "Error: buf not found. Install with: go install github.com/bufbuild/buf/cmd/buf@latest"
    exit 1
fi

# Limpiar código generado anterior
rm -rf ../pb/v1/*.pb.go

# Generar código
buf generate

echo "Protobuf code generated successfully in ../pb/"

