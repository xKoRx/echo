#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${ROOT_DIR}/bin"

COLOR_BLUE=""
COLOR_GREEN=""
COLOR_RED=""
COLOR_YELLOW=""
COLOR_RESET=""

if command -v tput >/dev/null 2>&1 && [ -n "${TERM:-}" ] && [ "$(tput colors 2>/dev/null || echo 0)" -ge 8 ]; then
  COLOR_BLUE="$(tput setaf 4)"
  COLOR_GREEN="$(tput setaf 2)"
  COLOR_RED="$(tput setaf 1)"
  COLOR_YELLOW="$(tput setaf 3)"
  COLOR_RESET="$(tput sgr0)"
fi

print_header() {
  echo -e "${COLOR_BLUE}===================================================================${COLOR_RESET}"
  echo -e "${COLOR_BLUE}  $1${COLOR_RESET}"
  echo -e "${COLOR_BLUE}===================================================================${COLOR_RESET}"
}

print_info() {
  echo -e "${COLOR_YELLOW}ℹ️  $1${COLOR_RESET}"
}

print_success() {
  echo -e "${COLOR_GREEN}✅ $1${COLOR_RESET}"
}

print_error() {
  echo -e "${COLOR_RED}❌ $1${COLOR_RESET}"
}

clean_bin_dir() {
  if [ -d "${BIN_DIR}" ]; then
    print_info "Limpiando bin/"
    find "${BIN_DIR}" -mindepth 1 ! -name '.gitkeep' -exec rm -rf {} +
    print_success "bin/ limpio"
  else
    print_info "bin/ no existe, nada que limpiar"
  fi
}

prepare_bin_dir() {
  mkdir -p "${BIN_DIR}"
  print_success "bin/ listo"
}

copy_mt4_sources() {
  print_header "Copiando fuentes MT4"
  cp "${ROOT_DIR}/clients/mt4/master.mq4" "${BIN_DIR}/master.mq4"
  cp "${ROOT_DIR}/clients/mt4/slave.mq4" "${BIN_DIR}/slave.mq4"
  cp "${ROOT_DIR}/clients/mt4/JAson.mqh" "${BIN_DIR}/JAson.mqh"
  print_success "Fuentes MT4 copiadas"
}

build_core() {
  print_header "Compilando echo-core (linux amd64)"
  GOOS=linux GOARCH=amd64 go build -o "${BIN_DIR}/echo-core" "${ROOT_DIR}/core/cmd/echo-core"
  ls -lh "${BIN_DIR}/echo-core" | awk '{print "   echo-core → "$5" ("$6" "$7" "$8")"}'
}

build_core_cli() {
  print_header "Compilando echo-core-cli (linux amd64)"
  GOOS=linux GOARCH=amd64 go build -o "${BIN_DIR}/echo-core-cli" "${ROOT_DIR}/core/cmd/echo-core-cli"
  ls -lh "${BIN_DIR}/echo-core-cli" | awk '{print "   echo-core-cli → "$5" ("$6" "$7" "$8")"}'
}

build_agent() {
  print_header "Compilando echo-agent (windows amd64)"
  GOOS=windows GOARCH=amd64 go build -o "${BIN_DIR}/echo-agent.exe" "${ROOT_DIR}/agent/cmd/echo-agent"
  ls -lh "${BIN_DIR}/echo-agent.exe" | awk '{print "   echo-agent.exe → "$5" ("$6" "$7" "$8")"}'
}

verify_compiler() {
  local compiler="$1"
  if ! command -v "${compiler}" >/dev/null 2>&1; then
    print_error "Compilador ${compiler} no encontrado"
    print_info "Instalar con: sudo apt-get install mingw-w64"
    return 1
  fi
  return 0
}

build_dlls() {
  print_header "Compilando DLLs echo_pipe"
  verify_compiler "x86_64-w64-mingw32-g++"
  verify_compiler "i686-w64-mingw32-g++"

  print_info "Compilando DLL x64"
  x86_64-w64-mingw32-g++ -O2 -Wall -Wextra -shared -static-libgcc -static-libstdc++ \
    -Wl,--add-stdcall-alias -o "${BIN_DIR}/echo_pipe_x64.dll" "${ROOT_DIR}/pipe/echo_pipe.cpp"
  print_success "echo_pipe_x64.dll generada"
  ls -lh "${BIN_DIR}/echo_pipe_x64.dll" | awk '{print "   echo_pipe_x64.dll → "$5" ("$6" "$7" "$8")"}'

  print_info "Compilando DLL x86"
  i686-w64-mingw32-g++ -O2 -Wall -Wextra -shared -static-libgcc -static-libstdc++ \
    -Wl,--add-stdcall-alias -o "${BIN_DIR}/echo_pipe_x86.dll" "${ROOT_DIR}/pipe/echo_pipe.cpp"
  print_success "echo_pipe_x86.dll generada"
  ls -lh "${BIN_DIR}/echo_pipe_x86.dll" | awk '{print "   echo_pipe_x86.dll → "$5" ("$6" "$7" "$8")"}'
}

show_summary() {
  print_header "Resumen de artefactos"
  if [ ! -d "${BIN_DIR}" ]; then
    print_info "bin/ no existe aún"
    return
  fi

  find "${BIN_DIR}" -maxdepth 1 -type f | sort | while read -r file; do
    ls -lh "$file" | awk '{print "  "$9" → "$5" ("$6" "$7" "$8")"}'
  done
}

usage() {
  cat <<EOF
Uso: $(basename "$0") [all|core|agent|dll|copy|clean]

  all   : limpia bin/, copia fuentes MT4 y compila todos los binarios
  core  : compila echo-core y echo-core-cli
  agent : compila echo-agent.exe (Windows amd64)
  dll   : compila echo_pipe_x86.dll y echo_pipe_x64.dll
  copy  : copia las fuentes del cliente MT4 al bin/
  clean : limpia el contenido de bin/
EOF
}

main() {
  local target="${1:-all}"

  case "${target}" in
    clean)
      print_header "Limpiando bin/"
      clean_bin_dir
      exit 0
      ;;
    all|core|agent|dll|copy)
      ;;
    *)
      usage
      exit 1
      ;;
  esac

  prepare_bin_dir

  case "${target}" in
    all)
      clean_bin_dir
      copy_mt4_sources
      build_core
      build_core_cli
      build_agent
      build_dlls
      ;;
    core)
      build_core
      build_core_cli
      ;;
    agent)
      build_agent
      ;;
    dll)
      build_dlls
      ;;
    copy)
      copy_mt4_sources
      ;;
  esac

  show_summary
  print_success "Proceso finalizado"
}

main "$@"
