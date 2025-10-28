@echo off
REM ============================================================================
REM build_dll.bat - Script para compilar echo_pipe.dll con Visual Studio
REM ============================================================================
REM
REM Requisitos:
REM   - Visual Studio 2019+ con C++ tools instalados
REM   - Ejecutar desde "Developer Command Prompt for VS"
REM
REM Uso:
REM   build_dll.bat
REM
REM Output:
REM   - echo_pipe.dll (32-bit para MT4)
REM
REM ============================================================================

echo ========================================
echo Building echo_pipe.dll for MT4 (32-bit)
echo ========================================
echo.

REM Verificar que estamos en Developer Command Prompt
where cl >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: cl.exe not found in PATH
    echo.
    echo Please run this script from "Developer Command Prompt for VS"
    echo Or: "x86 Native Tools Command Prompt for VS"
    echo.
    pause
    exit /b 1
)

REM Compilar para 32-bit (x86)
echo Compiling echo_pipe.cpp...
cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe.dll kernel32.lib

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo ERROR: Compilation failed
    echo.
    pause
    exit /b 1
)

echo.
echo ========================================
echo Build SUCCESS
echo ========================================
echo.

REM Verificar exports
echo Verifying DLL exports...
dumpbin /exports echo_pipe.dll | findstr "ConnectPipe WritePipeW ReadPipeLine ClosePipe"

echo.
echo ========================================
echo Next steps:
echo ========================================
echo 1. Copy echo_pipe.dll to MT4/MQL4/Libraries/
echo 2. Recompile master.mq4 and slave.mq4 in MetaEditor
echo 3. Enable "Allow DLL imports" in MT4 (Tools - Options - Expert Advisors)
echo 4. Load EAs in charts
echo.

pause

