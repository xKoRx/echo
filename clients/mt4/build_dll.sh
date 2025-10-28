#!/bin/bash
# ============================================================================
# build_dll.sh - Script para compilar echo_pipe.dll con MinGW (Linux/WSL)
# ============================================================================
#
# Requisitos:
#   - MinGW-w64 cross-compiler instalado
#   - Ejecutar en Linux o WSL
#
# Instalación de MinGW:
#   sudo apt-get install mingw-w64
#
# Uso:
#   chmod +x build_dll.sh
#   ./build_dll.sh
#
# Output:
#   - echo_pipe.dll (32-bit para MT4)
#
# ============================================================================

set -e  # Exit on error

echo "========================================"
echo "Building echo_pipe.dll for MT4 (32-bit)"
echo "========================================"
echo

# Verificar que MinGW está instalado
if ! command -v i686-w64-mingw32-g++ &> /dev/null; then
    echo "ERROR: i686-w64-mingw32-g++ not found"
    echo
    echo "Please install MinGW-w64:"
    echo "  sudo apt-get install mingw-w64"
    echo
    exit 1
fi

# Compilar para 32-bit (i686)
echo "Compiling echo_pipe.cpp..."
i686-w64-mingw32-g++ -shared -o echo_pipe.dll echo_pipe.cpp \
    -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias \
    -O2 -Wall

if [ $? -ne 0 ]; then
    echo
    echo "ERROR: Compilation failed"
    echo
    exit 1
fi

echo
echo "========================================"
echo "Build SUCCESS"
echo "========================================"
echo

# Verificar exports (con objdump)
echo "Verifying DLL exports..."
i686-w64-mingw32-objdump -p echo_pipe.dll | grep -E "ConnectPipe|WritePipeW|ReadPipeLine|ClosePipe" || true

echo
echo "========================================"
echo "Next steps:"
echo "========================================"
echo "1. Copy echo_pipe.dll to Windows: MT4/MQL4/Libraries/"
echo "2. Recompile master.mq4 and slave.mq4 in MetaEditor"
echo "3. Enable 'Allow DLL imports' in MT4 (Tools → Options → Expert Advisors)"
echo "4. Load EAs in charts"
echo

# Opcional: crear también version 64-bit para MT5
read -p "Also build 64-bit version for MT5? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo
    echo "Building echo_pipe_x64.dll for MT5 (64-bit)..."
    x86_64-w64-mingw32-g++ -shared -o echo_pipe_x64.dll echo_pipe.cpp \
        -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias \
        -O2 -Wall
    
    if [ $? -eq 0 ]; then
        echo "✅ echo_pipe_x64.dll built successfully"
    else
        echo "❌ Failed to build 64-bit version"
    fi
fi

echo
echo "Done!"

