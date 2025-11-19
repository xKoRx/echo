#!/usr/bin/env bash
# ==============================================================================
# ECHO TRADING SYSTEM - BUILD SCRIPT
# ==============================================================================
# Este script orquesta la compilación de todos los módulos del sistema Echo.
# 
# Uso: ./build_all.sh [target]
# Targets: all, core, agent, dll, proto, copy, clean
#
# Autor: Echo Team
# Versión: 1.1.0 (Protocol V2/Lossless)
# ==============================================================================

set -euo pipefail

# Asegurar que herramientas de Go estén en el PATH
export PATH=$PATH:$(go env GOPATH)/bin

# ------------------------------------------------------------------------------
# CONFIGURACIÓN Y CONSTANTES
# ------------------------------------------------------------------------------
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${ROOT_DIR}/bin"

# Paleta de colores (High Frequency Trading Style)
if command -v tput >/dev/null 2>&1 && [ -n "${TERM:-}" ] && [ "$(tput colors 2>/dev/null || echo 0)" -ge 8 ]; then
  BOLD="$(tput bold)"
  RESET="$(tput sgr0)"
  BLUE="$(tput setaf 39)"     # Deep Sky Blue
  CYAN="$(tput setaf 51)"     # Cyan
  GREEN="$(tput setaf 46)"    # Neon Green
  YELLOW="$(tput setaf 226)"  # Yellow
  RED="$(tput setaf 196)"     # Red
  MAGENTA="$(tput setaf 201)" # Magenta
  GRAY="$(tput setaf 245)"    # Gray
else
  BOLD=""
  RESET=""
  BLUE=""
  CYAN=""
  GREEN=""
  YELLOW=""
  RED=""
  MAGENTA=""
  GRAY=""
fi

# ------------------------------------------------------------------------------
# UTILITIES
# ------------------------------------------------------------------------------

# Imprime un timestamp de alta precisión
timestamp() {
  date "+%H:%M:%S"
}

# Header de sección con estilo "Bloque"
print_header() {
  echo ""
  echo -e "${BLUE}${BOLD}╔══════════════════════════════════════════════════════════════════════════════╗${RESET}"
  echo -e "${BLUE}${BOLD}║ $(timestamp) | $1${RESET}"
  echo -e "${BLUE}${BOLD}╚══════════════════════════════════════════════════════════════════════════════╝${RESET}"
}

# Sub-header para pasos dentro de una sección
print_step() {
  echo -e "${CYAN} ➤ $1${RESET}"
}

print_info() {
  echo -e "${GRAY}   ℹ️  $1${RESET}"
}

print_success() {
  echo -e "${GREEN}   ✅ $1${RESET}"
}

print_warning() {
  echo -e "${YELLOW}   ⚠️  $1${RESET}"
}

print_error() {
  echo -e "${RED}   ❌ $1${RESET}"
}

check_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    print_error "Comando no encontrado: $1"
    return 1
  fi
}

# ------------------------------------------------------------------------------
# TASKS
# ------------------------------------------------------------------------------

clean_bin_dir() {
  print_header "LIMPIEZA DE ARTEFACTOS (CLEAN)"
  
  if [ -d "${BIN_DIR}" ]; then
    print_step "Eliminando binarios antiguos en ${BIN_DIR}..."
    # Borrar todo excepto .gitkeep si existe
    find "${BIN_DIR}" -mindepth 1 ! -name '.gitkeep' -exec rm -rf {} +
    print_success "Directorio bin/ limpio."
  else
    print_info "Directorio bin/ no existe, nada que limpiar."
  fi
}

prepare_bin_dir() {
  if [ ! -d "${BIN_DIR}" ]; then
    mkdir -p "${BIN_DIR}"
    print_success "Directorio bin/ creado."
  fi
}

generate_protos() {
  print_header "REGENERACIÓN DE CONTRATOS (PROTO)"
  print_step "Ejecutando make proto..."
  
  if [ -f "${ROOT_DIR}/Makefile" ]; then
    if make proto; then
        print_success "Archivos .pb.go regenerados correctamente."
    else
        print_error "Fallo al ejecutar make proto."
        exit 1
    fi
  else
    print_error "No se encontró Makefile en la raíz."
    exit 1
  fi

  print_step "Sincronizando dependencias (go work sync / go mod tidy)..."
  # Intentar sincronizar si se usa go.work, sino ir módulo por módulo
  if [ -f "${ROOT_DIR}/go.work" ]; then
      if go work sync; then
          print_success "Workspace sincronizado."
      else
          print_warning "Fallo en go work sync, continuando..."
      fi
  fi
}

copy_mt4_sources() {
  print_header "COPIANDO FUENTES MQL4"
  
  local files=("master.mq4" "slave.mq4" "JAson.mqh")
  
  for file in "${files[@]}"; do
    if [ -f "${ROOT_DIR}/clients/mt4/${file}" ]; then
      cp "${ROOT_DIR}/clients/mt4/${file}" "${BIN_DIR}/${file}"
      print_success "Copiado: ${file}"
    else
      print_warning "No encontrado: ${file}"
    fi
  done
}

build_core() {
  print_header "COMPILANDO ECHO CORE (Go)"
  
  local target_os="linux"
  local target_arch="amd64"
  local output="${BIN_DIR}/echo-core"
  
  print_step "Building echo-core [${target_os}/${target_arch}]..."
  
  env GOOS=${target_os} GOARCH=${target_arch} go build -ldflags="-s -w" -o "${output}" "${ROOT_DIR}/core/cmd/echo-core"
  
  if [ -f "${output}" ]; then
    local size=$(ls -lh "${output}" | awk '{print $5}')
    print_success "Core compilado: ${output} (${size})"
  else
    print_error "Fallo compilación echo-core"
    exit 1
  fi
}

build_core_cli() {
  print_step "Building echo-core-cli..."
  local output="${BIN_DIR}/echo-core-cli"
  
  env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "${output}" "${ROOT_DIR}/core/cmd/echo-core-cli"
  
  if [ -f "${output}" ]; then
    local size=$(ls -lh "${output}" | awk '{print $5}')
    print_success "CLI compilado: ${output} (${size})"
  else
    print_error "Fallo compilación echo-core-cli"
    exit 1
  fi
}

build_agent() {
  print_header "COMPILANDO ECHO AGENT (Go)"
  
  local target_os="windows"
  local target_arch="amd64"
  local output="${BIN_DIR}/echo-agent.exe"
  
  print_step "Building echo-agent [${target_os}/${target_arch}]..."
  print_info "Asegurando dependencias actualizadas del SDK..."
  
  # Pequeño hack para asegurar que agent vea el SDK regenerado si no usa workspaces correctamente
  if [ -d "${ROOT_DIR}/agent" ]; then
      (cd "${ROOT_DIR}/agent" && go mod tidy >/dev/null 2>&1 || true)
  fi

  env GOOS=${target_os} GOARCH=${target_arch} go build -ldflags="-s -w" -o "${output}" "${ROOT_DIR}/agent/cmd/echo-agent"
  
  if [ -f "${output}" ]; then
    local size=$(ls -lh "${output}" | awk '{print $5}')
    print_success "Agent compilado: ${output} (${size})"
  else
    print_error "Fallo compilación echo-agent"
    exit 1
  fi
}

build_dlls() {
  print_header "COMPILANDO DLLs (C++)"
  
  # Verificar compiladores
  if ! command -v x86_64-w64-mingw32-g++ >/dev/null 2>&1; then
    print_warning "Compilador Mingw-w64 no detectado. Saltando build de DLLs."
    print_info "Instalar con: sudo apt-get install mingw-w64"
    return 0
  fi

  # x64
  print_step "Compilando echo_pipe_x64.dll..."
  x86_64-w64-mingw32-g++ -O2 -Wall -Wextra -shared -static-libgcc -static-libstdc++ \
    -Wl,--add-stdcall-alias -o "${BIN_DIR}/echo_pipe_x64.dll" "${ROOT_DIR}/pipe/echo_pipe.cpp"
  print_success "x64 DLL generada."

  # x86
  print_step "Compilando echo_pipe_x86.dll..."
  i686-w64-mingw32-g++ -O2 -Wall -Wextra -shared -static-libgcc -static-libstdc++ \
    -Wl,--add-stdcall-alias -o "${BIN_DIR}/echo_pipe_x86.dll" "${ROOT_DIR}/pipe/echo_pipe.cpp"
  print_success "x86 DLL generada."
}

show_summary() {
  print_header "RESUMEN DE ARTEFACTOS"
  
  if [ ! -d "${BIN_DIR}" ]; then
    print_warning "No hay artefactos en bin/"
    return
  fi

  # Listar con formato alineado
  find "${BIN_DIR}" -maxdepth 1 -type f | sort | while read -r file; do
    filename=$(basename "$file")
    size=$(ls -lh "$file" | awk '{print $5}')
    # Timestamp de modificación
    time=$(date -r "$file" "+%H:%M:%S")
    echo -e "${GRAY}   $time${RESET} | ${MAGENTA}${filename}${RESET} \t${BOLD}${size}${RESET}"
  done
  echo ""
}

usage() {
  echo -e "${BOLD}Echo Build System${RESET}"
  echo "Uso: ./build_all.sh [target]"
  echo ""
  echo "Targets:"
  echo -e "  ${CYAN}all${RESET}   : Pipeline completo (Clean -> Proto -> Copy -> Build -> DLLs)"
  echo -e "  ${CYAN}proto${RESET} : Regenera código gRPC/Protobuf"
  echo -e "  ${CYAN}core${RESET}  : Compila Core y CLI"
  echo -e "  ${CYAN}agent${RESET} : Compila Agent (Windows)"
  echo -e "  ${CYAN}dll${RESET}   : Compila DLLs C++"
  echo -e "  ${CYAN}copy${RESET}  : Copia fuentes MQL4"
  echo -e "  ${CYAN}clean${RESET} : Limpia directorio bin/"
  echo ""
}

# ------------------------------------------------------------------------------
# MAIN
# ------------------------------------------------------------------------------

main() {
  local target="${1:-all}"

  # Banner
  echo -e "${BLUE}"
  echo "██████╗ ██╗   ██╗██╗██╗     ██████╗ "
  echo "██╔══██╗██║   ██║██║██║     ██╔══██╗"
  echo "██████╔╝██║   ██║██║██║     ██║  ██║"
  echo "██╔══██╗██║   ██║██║██║     ██║  ██║"
  echo "██████╔╝╚██████╔╝██║███████╗██████╔╝"
  echo "╚═════╝  ╚═════╝ ╚═╝╚══════╝╚═════╝ "
  echo -e "${RESET}"
  echo -e "${GRAY}Echo Trading System Build Tool v1.1${RESET}"

  prepare_bin_dir

  case "${target}" in
    clean)
      clean_bin_dir
      ;;
    proto)
      generate_protos
      ;;
    core)
      generate_protos
      build_core
      build_core_cli
      ;;
    agent)
      generate_protos
      build_agent
      ;;
    dll)
      build_dlls
      ;;
    copy)
      copy_mt4_sources
      ;;
    all)
      clean_bin_dir
      generate_protos
      copy_mt4_sources
      build_core
      build_core_cli
      build_agent
      build_dlls
      ;;
    *)
      usage
      exit 1
      ;;
  esac

  show_summary
  print_success "PROCESO FINALIZADO EXITOSAMENTE"
}

main "$@"
