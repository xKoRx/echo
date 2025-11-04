#!/bin/bash

# ==============================================================================
# Script de Compilación Completa - Proyecto ECHO
# ==============================================================================
# Compila todos los binarios del proyecto y los coloca en bin/
#
# Uso:
#   ./build.sh           - Compila todo
#   ./build.sh core      - Solo echo-core (Linux)
#   ./build.sh agent     - Solo echo-agent (Windows)
#   ./build.sh dll       - Solo DLLs (x86 y x64)
#   ./build.sh clean     - Limpia bin/
# ==============================================================================

set -e  # Exit on error

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Directorios
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${PROJECT_ROOT}/bin"

# ==============================================================================
# Funciones
# ==============================================================================

print_header() {
    echo -e "${BLUE}===================================================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}===================================================================${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ️  $1${NC}"
}

# ==============================================================================
# Crear directorio bin
# ==============================================================================

create_bin_dir() {
    mkdir -p "${BIN_DIR}"
    print_success "Directorio bin/ creado"
}

# ==============================================================================
# Compilar echo-core (Linux)
# ==============================================================================

build_core() {
    print_header "Compilando echo-core (Linux)"
    
    cd "${PROJECT_ROOT}/core"
    go build -o "${BIN_DIR}/echo-core" ./cmd/echo-core
    
    print_success "echo-core compilado en bin/echo-core"
    ls -lh "${BIN_DIR}/echo-core" | awk '{print "   Tamaño: "$5" | "$6" "$7" "$8}'
}

# ==============================================================================
# Compilar echo-agent (Windows)
# ==============================================================================

build_agent() {
    print_header "Compilando echo-agent (Windows)"
    
    cd "${PROJECT_ROOT}/agent"
    GOOS=windows GOARCH=amd64 go build -o "${BIN_DIR}/echo-agent-windows-amd64.exe" ./cmd/echo-agent
    
    print_success "echo-agent-windows-amd64.exe compilado en bin/"
    ls -lh "${BIN_DIR}/echo-agent-windows-amd64.exe" | awk '{print "   Tamaño: "$5" | "$6" "$7" "$8}'
}

# ==============================================================================
# Compilar DLLs (x86 y x64)
# ==============================================================================

build_dll() {
    print_header "Compilando DLLs (x86 y x64)"
    
    # Verificar compiladores
    if ! command -v x86_64-w64-mingw32-g++ &> /dev/null; then
        print_error "x86_64-w64-mingw32-g++ no encontrado"
        print_info "Instalar con: sudo apt-get install mingw-w64"
        return 1
    fi
    
    if ! command -v i686-w64-mingw32-g++ &> /dev/null; then
        print_error "i686-w64-mingw32-g++ no encontrado"
        print_info "Instalar con: sudo apt-get install mingw-w64"
        return 1
    fi
    
    cd "${PROJECT_ROOT}/pipe"
    
    # Compilar x64
    print_info "Compilando DLL x64..."
    x86_64-w64-mingw32-g++ -O2 -Wall -Wextra -shared -static-libgcc -static-libstdc++ \
        -Wl,--add-stdcall-alias -o "${BIN_DIR}/echo_pipe_x64.dll" echo_pipe.cpp
    print_success "echo_pipe_x64.dll compilado"
    ls -lh "${BIN_DIR}/echo_pipe_x64.dll" | awk '{print "   Tamaño: "$5" | "$6" "$7" "$8}'
    
    # Compilar x86
    print_info "Compilando DLL x86..."
    i686-w64-mingw32-g++ -O2 -Wall -Wextra -shared -static-libgcc -static-libstdc++ \
        -Wl,--add-stdcall-alias -o "${BIN_DIR}/echo_pipe_x86.dll" echo_pipe.cpp
    print_success "echo_pipe_x86.dll compilado"
    ls -lh "${BIN_DIR}/echo_pipe_x86.dll" | awk '{print "   Tamaño: "$5" | "$6" "$7" "$8}'
}

# ==============================================================================
# Limpiar bin/
# ==============================================================================

clean() {
    print_header "Limpiando bin/"
    
    if [ -d "${BIN_DIR}" ]; then
        rm -rf "${BIN_DIR}"
        print_success "Directorio bin/ eliminado"
    else
        print_info "No hay nada que limpiar"
    fi
}

# ==============================================================================
# Resumen final
# ==============================================================================

show_summary() {
    print_header "Resumen de Compilación"
    
    echo ""
    echo -e "${GREEN}📦 Binarios en bin/:${NC}"
    echo ""
    
    if [ -f "${BIN_DIR}/echo-core" ]; then
        echo -e "${BLUE}🐧 Linux:${NC}"
        ls -lh "${BIN_DIR}/echo-core" | awk '{print "   echo-core - "$5" ("$6" "$7" "$8")"}'
    fi
    
    if [ -f "${BIN_DIR}/echo-agent-windows-amd64.exe" ]; then
        echo ""
        echo -e "${BLUE}🪟 Windows:${NC}"
        ls -lh "${BIN_DIR}/echo-agent-windows-amd64.exe" | awk '{print "   echo-agent-windows-amd64.exe - "$5" ("$6" "$7" "$8")"}'
    fi
    
    if [ -f "${BIN_DIR}/echo_pipe_x64.dll" ] || [ -f "${BIN_DIR}/echo_pipe_x86.dll" ]; then
        echo ""
        echo -e "${BLUE}🔌 DLLs:${NC}"
        [ -f "${BIN_DIR}/echo_pipe_x64.dll" ] && ls -lh "${BIN_DIR}/echo_pipe_x64.dll" | awk '{print "   echo_pipe_x64.dll - "$5" ("$6" "$7" "$8")"}'
        [ -f "${BIN_DIR}/echo_pipe_x86.dll" ] && ls -lh "${BIN_DIR}/echo_pipe_x86.dll" | awk '{print "   echo_pipe_x86.dll - "$5" ("$6" "$7" "$8")"}'
    fi
    
    echo ""
    print_success "Compilación completada: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""
}

# ==============================================================================
# Main
# ==============================================================================

main() {
    local target="${1:-all}"
    
    case "$target" in
        core)
            create_bin_dir
            build_core
            show_summary
            ;;
        agent)
            create_bin_dir
            build_agent
            show_summary
            ;;
        dll)
            create_bin_dir
            build_dll
            show_summary
            ;;
        clean)
            clean
            ;;
        all)
            create_bin_dir
            build_core
            build_agent
            build_dll
            show_summary
            ;;
        *)
            echo "Uso: $0 [all|core|agent|dll|clean]"
            echo ""
            echo "Opciones:"
            echo "  all     - Compila todo (default)"
            echo "  core    - Solo echo-core (Linux)"
            echo "  agent   - Solo echo-agent (Windows)"
            echo "  dll     - Solo DLLs (x86 y x64)"
            echo "  clean   - Limpia bin/"
            exit 1
            ;;
    esac
}

# Ejecutar
main "$@"

