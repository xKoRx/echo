#!/bin/bash

# ==============================================================================
# Echo Pipe DLL Build Script
# ==============================================================================
# 
# Este script compila echo_pipe.dll para ambas arquitecturas (x86 y x64)
# usando MinGW cross-compiler desde Linux.
# 
# Requisitos:
#   - mingw-w64 instalado (apt-get install mingw-w64)
#   - CMake 3.10+ (opcional, para usar CMake)
# 
# Uso:
#   ./build.sh              # Compilar x64 y x86 con MinGW directo
#   ./build.sh cmake        # Compilar usando CMake
#   ./build.sh clean        # Limpiar artefactos
#   ./build.sh test         # Mostrar exports de las DLLs
# 
# Versión: 1.0.0
# Fecha: 2025-10-24
# ==============================================================================

set -e  # Exit on error

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
print_header() {
    echo -e "${BLUE}================================================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}================================================================${NC}"
}

print_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

print_step() {
    echo -e "\n${BLUE}>>> $1${NC}\n"
}

# Get script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Build output directory
BUILD_DIR="$SCRIPT_DIR/build"
BIN_DIR="$SCRIPT_DIR/bin"

# ==============================================================================
# Function: Check MinGW Installation
# ==============================================================================
check_mingw() {
    print_step "Checking MinGW installation..."
    
    if ! command -v x86_64-w64-mingw32-g++ &> /dev/null; then
        print_error "MinGW x64 compiler not found"
        print_info "Install with: sudo apt-get install mingw-w64"
        return 1
    fi
    
    if ! command -v i686-w64-mingw32-g++ &> /dev/null; then
        print_error "MinGW x86 compiler not found"
        print_info "Install with: sudo apt-get install mingw-w64"
        return 1
    fi
    
    print_success "MinGW compilers found"
    
    # Show versions
    echo ""
    x86_64-w64-mingw32-g++ --version | head -n 1
    i686-w64-mingw32-g++ --version | head -n 1
    echo ""
    
    return 0
}

# ==============================================================================
# Function: Build with MinGW (Direct)
# ==============================================================================
build_mingw_direct() {
    print_header "Building with MinGW (Direct)"
    
    # Create bin directory
    mkdir -p "$BIN_DIR"
    
    # Compile flags
    CFLAGS="-O2 -Wall -Wextra"
    LDFLAGS="-shared -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias"
    
    # ========================================================================
    # Build x64 DLL
    # ========================================================================
    print_step "Building x64 DLL..."
    
    x86_64-w64-mingw32-g++ $CFLAGS $LDFLAGS \
        -o "$BIN_DIR/echo_pipe_x64.dll" \
        echo_pipe.cpp
    
    if [ $? -eq 0 ]; then
        print_success "x64 DLL built successfully"
        ls -lh "$BIN_DIR/echo_pipe_x64.dll"
    else
        print_error "x64 DLL build failed"
        return 1
    fi
    
    # ========================================================================
    # Build x86 DLL
    # ========================================================================
    print_step "Building x86 DLL..."
    
    i686-w64-mingw32-g++ $CFLAGS $LDFLAGS \
        -o "$BIN_DIR/echo_pipe_x86.dll" \
        echo_pipe.cpp
    
    if [ $? -eq 0 ]; then
        print_success "x86 DLL built successfully"
        ls -lh "$BIN_DIR/echo_pipe_x86.dll"
    else
        print_error "x86 DLL build failed"
        return 1
    fi
    
    # ========================================================================
    # Build Test Executables
    # ========================================================================
    print_step "Building test executables..."
    
    # Test x64
    x86_64-w64-mingw32-g++ -O2 -Wall -Wextra -static-libgcc -static-libstdc++ \
        -o "$BIN_DIR/test_pipe_x64.exe" \
        test_pipe.cpp
    
    if [ $? -eq 0 ]; then
        print_success "x64 test executable built"
    fi
    
    # Test x86
    i686-w64-mingw32-g++ -O2 -Wall -Wextra -static-libgcc -static-libstdc++ \
        -o "$BIN_DIR/test_pipe_x86.exe" \
        test_pipe.cpp
    
    if [ $? -eq 0 ]; then
        print_success "x86 test executable built"
    fi
    
    echo ""
    print_success "Build completed!"
    echo ""
    print_info "Output directory: $BIN_DIR"
    ls -lh "$BIN_DIR"
    echo ""
}

# ==============================================================================
# Function: Build with CMake
# ==============================================================================
build_cmake() {
    print_header "Building with CMake"
    
    if ! command -v cmake &> /dev/null; then
        print_error "CMake not found"
        print_info "Install with: sudo apt-get install cmake"
        return 1
    fi
    
    # ========================================================================
    # Build x64
    # ========================================================================
    print_step "Building x64 with CMake..."
    
    BUILD_X64="$BUILD_DIR/x64"
    mkdir -p "$BUILD_X64"
    cd "$BUILD_X64"
    
    cmake ../.. \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_TOOLCHAIN_FILE="$SCRIPT_DIR/toolchain-mingw-x64.cmake" \
        -DCMAKE_INSTALL_PREFIX="$BIN_DIR"
    
    cmake --build . --config Release
    cmake --install .
    
    cd "$SCRIPT_DIR"
    print_success "x64 build completed"
    
    # ========================================================================
    # Build x86
    # ========================================================================
    print_step "Building x86 with CMake..."
    
    BUILD_X86="$BUILD_DIR/x86"
    mkdir -p "$BUILD_X86"
    cd "$BUILD_X86"
    
    cmake ../.. \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_TOOLCHAIN_FILE="$SCRIPT_DIR/toolchain-mingw-x86.cmake" \
        -DCMAKE_INSTALL_PREFIX="$BIN_DIR"
    
    cmake --build . --config Release
    cmake --install .
    
    cd "$SCRIPT_DIR"
    print_success "x86 build completed"
    
    echo ""
    print_success "CMake build completed!"
    echo ""
    print_info "Output directory: $BIN_DIR"
    ls -lh "$BIN_DIR/bin"
    echo ""
}

# ==============================================================================
# Function: Show DLL Exports
# ==============================================================================
show_exports() {
    print_header "Displaying DLL Exports"
    
    if [ ! -f "$BIN_DIR/echo_pipe_x64.dll" ]; then
        print_error "DLLs not found. Build first."
        return 1
    fi
    
    print_step "x64 DLL Exports:"
    x86_64-w64-mingw32-objdump -p "$BIN_DIR/echo_pipe_x64.dll" | grep -A 20 "Export Address Table"
    
    echo ""
    print_step "x86 DLL Exports:"
    i686-w64-mingw32-objdump -p "$BIN_DIR/echo_pipe_x86.dll" | grep -A 20 "Export Address Table"
    
    echo ""
}

# ==============================================================================
# Function: Clean Build Artifacts
# ==============================================================================
clean() {
    print_header "Cleaning Build Artifacts"
    
    rm -rf "$BUILD_DIR"
    rm -rf "$BIN_DIR"
    
    print_success "Clean completed"
}

# ==============================================================================
# Main
# ==============================================================================

print_header "Echo Pipe DLL Build Script v1.0.0"

case "${1:-}" in
    cmake)
        check_mingw || exit 1
        build_cmake
        ;;
    test)
        show_exports
        ;;
    clean)
        clean
        ;;
    *)
        check_mingw || exit 1
        build_mingw_direct
        ;;
esac

print_header "Done!"

