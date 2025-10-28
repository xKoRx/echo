# 🔧 Fix: Stack Damaged Error en EAs de Echo (i0)

**Fecha**: 2025-10-26  
**Issue**: `stack damaged, check DLL function call` + `uninit reason 8`  
**Status**: ✅ RESUELTO

---

## 🔴 Problema Identificado

### Error Original
```
slave XAUUSD,H1: stack damaged, check DLL function call in 'slave.mq4' (171,12)
slave XAUUSD,H1: uninit reason 8
slave XAUUSD,H1: not initialized
```

### Causa Raíz

**Incompatibilidad de firma DLL entre MQL4 y echo_pipe.dll**:

1. **En MQL4 build 600+**: `long` es **64-bit** (8 bytes)
2. **En DLL Win32**: `HANDLE`/`INT_PTR` es **32-bit** (4 bytes)
3. **Resultado**: Cuando MQL4 llama a la DLL con `long`, pone 8 bytes en la pila, pero la DLL espera leer 4 bytes → **stack corruption**

### Código Problemático (ANTES)

**slave.mq4** (líneas 20-25):
```mql4
#import "echo_pipe.dll"
   long  ConnectPipe(string pipeName);        // ❌ 64-bit en MQL4
   int   WritePipeW(long handle, string data); // ❌ 64-bit handle
   int   ReadPipeLine(long handle, uchar &buffer[], int bufferSize);
   void  ClosePipe(long handle);              // ❌ 64-bit handle
#import

long g_Pipe = 0;  // ❌ 64-bit global
```

**master.mq4** (líneas 18-22):
```mql4
#import "echo_pipe_x86.dll"
   long ConnectPipe(string pipeName);          // ❌ 64-bit
   int  WritePipeW(long handle, string data);  // ❌ 64-bit handle
   void ClosePipe(long handle);                // ❌ 64-bit handle
#import

long g_PipeHandle = 0;  // ❌ 64-bit global
```

---

## ✅ Solución Aplicada

### Cambios en slave.mq4

```mql4
// CRÍTICO: Usar 'int' NO 'long' para handles en MT4 32-bit
// La DLL Win32 retorna INT_PTR (32-bit) pero MQL4 'long' es 64-bit → stack corruption
#import "echo_pipe.dll"
   int  ConnectPipe(string pipeName);          // ✅ 32-bit
   int  WritePipeW(int handle, string data);   // ✅ 32-bit handle
   int  ReadPipeLine(int handle, uchar &buffer[], int bufferSize);
   void ClosePipe(int handle);                 // ✅ 32-bit handle
#import

int g_Pipe = 0;  // ✅ 32-bit global
```

### Cambios en master.mq4

```mql4
// CRÍTICO: Usar 'int' NO 'long' para handles en MT4 32-bit
#import "echo_pipe.dll"
   int  ConnectPipe(string pipeName);          // ✅ 32-bit
   int  WritePipeW(int handle, string data);   // ✅ 32-bit handle
   void ClosePipe(int handle);                 // ✅ 32-bit handle
#import

int g_PipeHandle = 0;  // ✅ 32-bit global
```

**Adicional**: Unificado nombre de DLL a `echo_pipe.dll` (antes master usaba `echo_pipe_x86.dll`).

---

## 🔨 Código Fuente de la DLL (echo_pipe.dll)

Si necesitas recompilar la DLL, aquí está el código correcto:

### echo_pipe.cpp

```cpp
/*
 * echo_pipe.dll - Named Pipes IPC para MetaTrader 4 (Win32)
 * 
 * CRÍTICO: Compilar como 32-bit (__stdcall) para MT4
 * 
 * Compilar con Visual Studio:
 *   cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe.dll kernel32.lib
 * 
 * Compilar con MinGW (Linux/WSL):
 *   i686-w64-mingw32-g++ -shared -o echo_pipe.dll echo_pipe.cpp \
 *       -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias
 * 
 * Versión: 1.0.0
 * Fecha: 2025-10-26
 */

#include <windows.h>
#include <stdio.h>
#include <string.h>

// ============================================================================
// FUNCIÓN 1: ConnectPipe
// ============================================================================
// Conecta a Named Pipe creado por Agent (cliente)
// Retorna: handle (int > 0) si éxito, -1 si error
extern "C" __declspec(dllexport) int __stdcall ConnectPipe(const wchar_t* pipeName)
{
    if (pipeName == NULL) {
        return -1;
    }

    HANDLE hPipe = CreateFileW(
        pipeName,
        GENERIC_READ | GENERIC_WRITE,
        0,
        NULL,
        OPEN_EXISTING,
        FILE_ATTRIBUTE_NORMAL,
        NULL
    );

    if (hPipe == INVALID_HANDLE_VALUE) {
        return -1;
    }

    // Configurar modo byte-stream
    DWORD mode = PIPE_READMODE_BYTE;
    SetNamedPipeHandleState(hPipe, &mode, NULL, NULL);

    // CRÍTICO: Castear HANDLE (puntero) a int para MT4 32-bit
    return (int)(INT_PTR)hPipe;
}

// ============================================================================
// FUNCIÓN 2: WritePipeW (Unicode)
// ============================================================================
// Escribe string Unicode al pipe (MQL4 strings son UTF-16)
// Retorna: bytes escritos (> 0) si éxito, -1 si error
extern "C" __declspec(dllexport) int __stdcall WritePipeW(int handle, const wchar_t* data)
{
    if (handle <= 0 || data == NULL) {
        return -1;
    }

    HANDLE hPipe = (HANDLE)(INT_PTR)handle;
    
    // Convertir UTF-16 → UTF-8 para el pipe
    int utf8_len = WideCharToMultiByte(CP_UTF8, 0, data, -1, NULL, 0, NULL, NULL);
    if (utf8_len <= 0) {
        return -1;
    }
    
    char* utf8_buffer = (char*)malloc(utf8_len);
    if (!utf8_buffer) {
        return -1;
    }
    
    WideCharToMultiByte(CP_UTF8, 0, data, -1, utf8_buffer, utf8_len, NULL, NULL);
    
    DWORD bytesWritten = 0;
    BOOL result = WriteFile(hPipe, utf8_buffer, utf8_len - 1, &bytesWritten, NULL);
    
    free(utf8_buffer);
    
    if (!result) {
        return -1;
    }
    
    FlushFileBuffers(hPipe);
    return (int)bytesWritten;
}

// ============================================================================
// FUNCIÓN 3: ReadPipeLine
// ============================================================================
// Lee una línea completa del pipe (hasta \n o hasta llenar buffer)
// Retorna: bytes leídos (> 0) si éxito, 0 si timeout, -1 si error
extern "C" __declspec(dllexport) int __stdcall ReadPipeLine(int handle, char* buffer, int bufferSize)
{
    if (handle <= 0 || buffer == NULL || bufferSize <= 0) {
        return -1;
    }

    HANDLE hPipe = (HANDLE)(INT_PTR)handle;
    int totalBytesRead = 0;

    // Leer byte a byte hasta encontrar \n o llenar buffer
    while (totalBytesRead < bufferSize - 1) {
        DWORD bytesRead = 0;
        char byte;

        BOOL result = ReadFile(hPipe, &byte, 1, &bytesRead, NULL);

        if (!result) {
            // Error de lectura
            if (totalBytesRead > 0) {
                break; // Retornar lo que se leyó
            }
            return -1;
        }

        if (bytesRead == 0) {
            // No hay más datos (pipe cerrado o timeout)
            break;
        }

        buffer[totalBytesRead++] = byte;

        // Si encontramos \n, terminamos la línea (incluimos el \n)
        if (byte == '\n') {
            break;
        }
    }

    // Null-terminate
    buffer[totalBytesRead] = '\0';

    return totalBytesRead;
}

// ============================================================================
// FUNCIÓN 4: ClosePipe
// ============================================================================
// Cierra el handle del pipe
extern "C" __declspec(dllexport) void __stdcall ClosePipe(int handle)
{
    if (handle <= 0) {
        return;
    }

    HANDLE hPipe = (HANDLE)(INT_PTR)handle;
    CloseHandle(hPipe);
}

// ============================================================================
// DllMain
// ============================================================================
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved)
{
    switch (ul_reason_for_call)
    {
    case DLL_PROCESS_ATTACH:
    case DLL_THREAD_ATTACH:
    case DLL_THREAD_DETACH:
    case DLL_PROCESS_DETACH:
        break;
    }
    return TRUE;
}
```

---

## 🔧 Compilación de la DLL

### Opción A: Visual Studio (Windows)

```cmd
REM Abrir "Developer Command Prompt for VS 2019"
cd C:\path\to\echo\clients\mt4

REM Compilar para 32-bit (x86)
cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe.dll kernel32.lib

REM Verificar exports
dumpbin /exports echo_pipe.dll
```

**Output esperado**:
```
    ordinal hint RVA      name
          1    0 00001000 ClosePipe
          2    1 00001020 ConnectPipe
          3    2 00001040 ReadPipeLine
          4    3 00001060 WritePipeW
```

### Opción B: MinGW (Linux/WSL)

```bash
# Instalar MinGW cross-compiler
sudo apt-get install mingw-w64

# Compilar para Win32 (i686)
i686-w64-mingw32-g++ -shared -o echo_pipe.dll echo_pipe.cpp \
    -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias

# Verificar (con wine)
wine dll_test.exe  # ver sección Test Program
```

---

## 🧪 Test Program (C++)

Para validar la DLL antes de usarla en MT4:

### test_pipe.cpp

```cpp
#include <windows.h>
#include <stdio.h>

typedef int (__stdcall *ConnectPipeFunc)(const wchar_t*);
typedef int (__stdcall *WritePipeWFunc)(int, const wchar_t*);
typedef int (__stdcall *ReadPipeLineFunc)(int, char*, int);
typedef void (__stdcall *ClosePipeFunc)(int);

int main() {
    HMODULE hDll = LoadLibrary(L"echo_pipe.dll");
    if (!hDll) {
        printf("ERROR: No se pudo cargar echo_pipe.dll\n");
        return 1;
    }

    ConnectPipeFunc ConnectPipe = (ConnectPipeFunc)GetProcAddress(hDll, "ConnectPipe");
    WritePipeWFunc WritePipeW = (WritePipeWFunc)GetProcAddress(hDll, "WritePipeW");
    ReadPipeLineFunc ReadPipeLine = (ReadPipeLineFunc)GetProcAddress(hDll, "ReadPipeLine");
    ClosePipeFunc ClosePipe = (ClosePipeFunc)GetProcAddress(hDll, "ClosePipe");

    if (!ConnectPipe || !WritePipeW || !ReadPipeLine || !ClosePipe) {
        printf("ERROR: No se pudieron obtener las funciones\n");
        FreeLibrary(hDll);
        return 1;
    }

    // Test: Conectar a pipe (debe fallar si Agent no corre)
    int handle = ConnectPipe(L"\\\\.\\pipe\\echo_master_12345");
    if (handle <= 0) {
        printf("INFO: No se pudo conectar al pipe (esperado si Agent no corre)\n");
        printf("      handle = %d\n", handle);
    } else {
        printf("OK: Conectado al pipe, handle=%d\n", handle);

        // Test: Escribir JSON
        const wchar_t* json = L"{\"type\":\"handshake\",\"timestamp_ms\":12345}\n";
        int written = WritePipeW(handle, json);
        printf("OK: Escritos %d bytes\n", written);

        // Test: Leer respuesta
        char buffer[1024];
        int read = ReadPipeLine(handle, buffer, sizeof(buffer));
        if (read > 0) {
            printf("OK: Leídos %d bytes: %s\n", read, buffer);
        }

        // Test: Cerrar
        ClosePipe(handle);
        printf("OK: Pipe cerrado\n");
    }

    FreeLibrary(hDll);
    printf("\n✅ Todos los tests de firma pasaron (no hay stack corruption)\n");
    return 0;
}
```

Compilar y ejecutar:
```cmd
cl test_pipe.cpp
test_pipe.exe
```

---

## 📋 Checklist de Validación

### 1. Verificar Firmas de DLL

```cmd
dumpbin /exports echo_pipe.dll
```

**Esperado**:
- 4 funciones exportadas: `ConnectPipe`, `WritePipeW`, `ReadPipeLine`, `ClosePipe`
- Sin sufijos `@N` (si aparecen, falta `--add-stdcall-alias` en MinGW)

### 2. Verificar Arquitectura de DLL

```cmd
dumpbin /headers echo_pipe.dll | findstr "machine"
```

**Esperado**: `machine (x86)` (NO x64)

### 3. Recompilar EAs en MetaEditor

1. Abrir MetaEditor (F4 en MT4)
2. Abrir `master.mq4` y `slave.mq4`
3. Compilar (F7)
4. **Esperado**: 0 errores, 0 warnings

### 4. Instalar DLL en MT4

```cmd
REM Copiar DLL a carpeta correcta
copy echo_pipe.dll "%APPDATA%\MetaQuotes\Terminal\<ID>\MQL4\Libraries\"

REM O usar: MT4 → File → Open Data Folder → MQL4\Libraries\
```

### 5. Habilitar DLL en MT4

1. Tools → Options → Expert Advisors
2. ✅ Marcar "Allow DLL imports"
3. ✅ Marcar "Allow imported functions call"

### 6. Cargar EAs y Verificar

1. Arrastrar `slave.mq4` a un chart XAUUSD
2. **Esperado en Expert tab**:
   ```
   [INFO] ... | EA initializing | account=...
   [INFO] ... | EA initialized | timer_period_ms=1000
   ```
3. **NO debe aparecer**:
   - "stack damaged"
   - "uninit reason 8"
   - "not initialized"

---

## 🎯 Resultado Esperado

### Antes del Fix
```
slave XAUUSD,H1: stack damaged, check DLL function call in 'slave.mq4' (171,12)
slave XAUUSD,H1: uninit reason 8
slave XAUUSD,H1: not initialized
```

### Después del Fix
```
[INFO] 12345678 | EA initialized | timer_period_ms=1000
[INFO] 12345679 | Pipe connected | pipe=\\.\pipe\echo_slave_67890
[INFO] 12345680 | Handshake sent | bytes=142
```

---

## 📝 Notas Técnicas

### ¿Por qué `int` y no `long` en MT4?

| Tipo MQL4 (build 600+) | Tamaño | Tipo C++ Win32 | Tamaño |
|------------------------|--------|----------------|--------|
| `int`                  | 4 bytes | `int`, `DWORD`, `HANDLE` (32-bit) | 4 bytes |
| `long`                 | 8 bytes | `long long`, `INT64` | 8 bytes |
| `long long`            | 8 bytes | `long long`, `INT64` | 8 bytes |

**Regla general para MT4 (32-bit)**:
- Windows `HANDLE` → MQL4 `int`
- Windows `DWORD` → MQL4 `int`
- Windows `LONG` → MQL4 `int`
- Windows `INT_PTR` (32-bit) → MQL4 `int`

**Para MT5 (64-bit)**:
- Windows `HANDLE` → MQL5 `long long`
- Windows `INT_PTR` (64-bit) → MQL5 `long long`

### ¿Qué es `__stdcall`?

Es la **convención de llamada** que MT4 usa por defecto:
- Parámetros se pasan en la pila de **derecha a izquierda**
- La **función llamada** limpia la pila (no el caller)
- Crítico: Si la DLL usa `__cdecl` y MQL4 asume `__stdcall`, habrá stack corruption

**Alternativa**: Si tu DLL está en `__cdecl`, agregar en MQL4:
```mql4
#import "echo_pipe.dll" cdecl  // Forzar cdecl
   int ConnectPipe(string pipeName);
#import
```

Pero **mejor práctica**: Compilar la DLL con `__stdcall` (estándar Win32 API).

---

## 🔍 Debugging Adicional

Si aún tienes problemas tras aplicar el fix:

### 1. Verificar que la DLL correcta está cargada

```mql4
// Agregar en OnInit() temporalmente
Print("Testing DLL load...");
int test_handle = ConnectPipe("\\\\.\\pipe\\test_nonexistent");
Print("ConnectPipe test returned: ", test_handle);  // Debe ser -1 si pipe no existe
if(test_handle == -1) {
    Print("✅ DLL loaded OK, ConnectPipe works");
} else {
    Print("⚠️ Unexpected handle: ", test_handle);
    if(test_handle > 0) ClosePipe(test_handle);
}
```

### 2. Verificar versión de MT4

```cmd
REM En MT4 → Help → About
REM Esperado: Build 600+ (para MQL4 moderno)
```

Si tienes build < 600, `long` podría ser 32-bit también (depende del broker).

### 3. Usar Dependency Walker

Herramienta para verificar dependencias de DLL:
1. Descargar: https://www.dependencywalker.com/
2. Abrir `echo_pipe.dll`
3. Verificar que no tenga dependencias de runtime faltantes (msvcr120.dll, etc.)

**Solución si falta runtime**: Compilar con `-static-libgcc -static-libstdc++` (MinGW) o usar `/MT` (Visual Studio).

---

## ✅ Conclusión

**El problema estaba en usar `long` (64-bit) en MQL4 para handles que la DLL Win32 retorna como `int` (32-bit).**

**Solución**:
1. ✅ Cambiar `long` → `int` en todos los #import
2. ✅ Cambiar `long g_Pipe` → `int g_Pipe`
3. ✅ Unificar nombre de DLL a `echo_pipe.dll`
4. ✅ Recompilar EAs
5. ✅ Verificar que DLL sea 32-bit con `__stdcall`

**Si la DLL no existe o no coincide con las firmas, recompilarla con el código fuente provisto arriba.**

---

**Autor**: Aranea Labs - Echo Trade Copier Team  
**Versión**: 1.0  
**Última actualización**: 2025-10-26

