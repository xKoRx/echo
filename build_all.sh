#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$ROOT_DIR/bin"

echo "[build] Cleaning bin directory..."
find "$BIN_DIR" -mindepth 1 ! -name '.gitkeep' -exec rm -rf {} +

echo "[build] Ensuring bin directory exists..."
mkdir -p "$BIN_DIR"

echo "[build] Copying MT4 sources (master/slave + dependencies)..."
cp "$ROOT_DIR/clients/mt4/master.mq4" "$BIN_DIR/master.mq4"
cp "$ROOT_DIR/clients/mt4/slave.mq4" "$BIN_DIR/slave.mq4"
cp "$ROOT_DIR/clients/mt4/JAson.mqh" "$BIN_DIR/JAson.mqh"

echo "[build] Compiling core (linux amd64)..."
GOOS=linux GOARCH=amd64 go build -o "$BIN_DIR/echo-core" "$ROOT_DIR/core/cmd/echo-core"

echo "[build] Compiling core CLI (linux amd64)..."
GOOS=linux GOARCH=amd64 go build -o "$BIN_DIR/echo-core-cli" "$ROOT_DIR/core/cmd/echo-core-cli"

echo "[build] Compiling agent (windows amd64)..."
GOOS=windows GOARCH=amd64 go build -o "$BIN_DIR/echo-agent.exe" "$ROOT_DIR/agent/cmd/echo-agent"

echo "[build] Compiling echo_pipe DLLs..."
i686-w64-mingw32-g++ -shared -O2 -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias \
  -o "$BIN_DIR/echo_pipe_x86.dll" "$ROOT_DIR/pipe/echo_pipe.cpp"

x86_64-w64-mingw32-g++ -shared -O2 -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias \
  -o "$BIN_DIR/echo_pipe_x64.dll" "$ROOT_DIR/pipe/echo_pipe.cpp"

echo "[build] Build completed successfully. Artifacts available in bin/."
