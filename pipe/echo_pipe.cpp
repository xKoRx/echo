/*
 * echo_pipe.dll - Named Pipes IPC para MetaTrader 4/5
 * 
 * Permite a EAs MQL4/MQL5 comunicarse con el Agent de Echo via Named Pipes.
 * 
 * Compilar con MinGW:
 *   x64: x86_64-w64-mingw32-g++ -shared -o echo_pipe_x64.dll echo_pipe.cpp -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias
 *   x86: i686-w64-mingw32-g++ -shared -o echo_pipe_x86.dll echo_pipe.cpp -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias
 * 
 * Compilar con Visual Studio:
 *   x64: cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe_x64.dll /DEF:echo_pipe_x64.def
 *   x86: cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe_x86.dll /DEF:echo_pipe_x86.def
 * 
 * Versión: 1.1.0
 * Fecha: 2025-10-24
 * Proyecto: Echo Trade Copier
 * RFC: RFC-002 (Iteración 0)
 * 
 * CORRECCIONES v1.1.0:
 * - Uso de INT_PTR para handles (evita truncamiento en x64)
 * - Agregado WritePipeW para conversión UTF-16 → UTF-8
 * - Validación robusta de handles
 * - Documentación mejorada sobre comportamiento bloqueante
 */

#include <windows.h>
#include <stdio.h>
#include <string.h>

// ============================================================================
// FUNCIÓN 1: ConnectPipe
// ============================================================================
// Conecta a un Named Pipe existente creado por el Agent (cliente)
// 
// Parámetros:
//   - pipeName: Nombre del pipe (ej: L"\\.\pipe\echo_master_12345")
//              MQL4/MQL5 pasa strings como wchar_t* (UTF-16)
// 
// Retorna:
//   - Handle del pipe (INT_PTR > 0) si éxito
//   - INVALID_HANDLE_VALUE (-1) si error
// 
// IMPORTANTE: En MQL4/MQL5 importar como 'long' (válido en 32 y 64 bits)
// 
extern "C" __declspec(dllexport) INT_PTR __stdcall ConnectPipe(const wchar_t* pipeName)
{
    if (pipeName == NULL) {
        return (INT_PTR)INVALID_HANDLE_VALUE;
    }

    // i2b FIX: Esperar a que el pipe esté disponible antes de conectar
    // Esto evita ERROR_PIPE_BUSY si el servidor no ha llamado a Accept() aún
    // Timeout de 2 segundos (2000ms)
    if (!WaitNamedPipeW(pipeName, 2000)) {
        // Pipe no disponible después del timeout
        // Códigos comunes:
        // - ERROR_FILE_NOT_FOUND (2): El pipe no existe
        // - ERROR_SEM_TIMEOUT (121): Timeout esperando
        return (INT_PTR)INVALID_HANDLE_VALUE;
    }

    HANDLE hPipe = CreateFileW(
        pipeName,                     // Nombre del pipe
        GENERIC_READ | GENERIC_WRITE, // Acceso lectura/escritura
        0,                            // No compartir
        NULL,                         // Seguridad por defecto
        OPEN_EXISTING,                // El pipe ya debe existir
        FILE_ATTRIBUTE_NORMAL,        // Atributos normales
        NULL                          // No template
    );

    if (hPipe == INVALID_HANDLE_VALUE) {
        // Error: pipe no existe o acceso denegado
        // Para debugging: DWORD err = GetLastError();
        return (INT_PTR)INVALID_HANDLE_VALUE;
    }

    // Configurar modo de lectura byte por byte (line-delimited JSON)
    DWORD mode = PIPE_READMODE_BYTE;
    BOOL result = SetNamedPipeHandleState(hPipe, &mode, NULL, NULL);
    
    if (!result) {
        // Error al configurar modo (raro, pero posible)
        // Para debugging: DWORD err = GetLastError();
        CloseHandle(hPipe);
        return (INT_PTR)INVALID_HANDLE_VALUE;
    }

    return (INT_PTR)hPipe;
}

// ============================================================================
// FUNCIÓN 2: WritePipeW (RECOMENDADA para MQL4/MQL5)
// ============================================================================
// Escribe datos UTF-16 (desde MQL) convirtiéndolos a UTF-8 en el pipe
// 
// Parámetros:
//   - handle: Handle del pipe retornado por ConnectPipe (usar 'long' en MQL)
//   - wdata: String UTF-16 desde MQL (debe terminar en \n)
// 
// Retorna:
//   - Número de bytes UTF-8 escritos si éxito (> 0)
//   - -1 si error
// 
// IMPORTANTE: Esta es la función que debe usar el Master EA
// 
extern "C" __declspec(dllexport) int __stdcall WritePipeW(INT_PTR handle, const wchar_t* wdata)
{
    // Validar handle
    if (handle == 0 || handle == (INT_PTR)INVALID_HANDLE_VALUE || wdata == NULL) {
        return -1;
    }

    // Calcular tamaño necesario para UTF-8
    int bytesNeeded = WideCharToMultiByte(CP_UTF8, 0, wdata, -1, NULL, 0, NULL, NULL);
    if (bytesNeeded <= 0) {
        return -1;
    }

    // Alocar buffer para UTF-8
    char* utf8Buffer = (char*)HeapAlloc(GetProcessHeap(), 0, bytesNeeded);
    if (utf8Buffer == NULL) {
        return -1;
    }

    // Convertir UTF-16 → UTF-8
    int converted = WideCharToMultiByte(CP_UTF8, 0, wdata, -1, utf8Buffer, bytesNeeded, NULL, NULL);
    if (converted <= 0) {
        HeapFree(GetProcessHeap(), 0, utf8Buffer);
        return -1;
    }

    // Escribir al pipe (sin null terminator)
    HANDLE hPipe = (HANDLE)handle;
    DWORD bytesToWrite = (DWORD)(converted - 1); // Excluye el null terminator
    DWORD bytesWritten = 0;

    BOOL result = WriteFile(hPipe, utf8Buffer, bytesToWrite, &bytesWritten, NULL);
    // Nota: NO llamar FlushFileBuffers aquí. En Named Pipes puede BLOQUEAR
    // hasta que el servidor lea completamente el buffer, generando freezes
    // en el hilo del EA. La baja latencia se logra manteniendo el buffer
    // pequeño y asegurando que el servidor lea continuamente.

    HeapFree(GetProcessHeap(), 0, utf8Buffer);

    return result ? (int)bytesWritten : -1;
}

// ============================================================================
// FUNCIÓN 3: WritePipe (LEGACY - usar WritePipeW desde MQL)
// ============================================================================
// Escribe datos char* en el pipe (para clientes C/C++, no MQL)
// 
// Parámetros:
//   - handle: Handle del pipe retornado por ConnectPipe
//   - data: String UTF-8 a enviar (debe terminar en \n)
// 
// Retorna:
//   - Número de bytes escritos si éxito (> 0)
//   - -1 si error
// 
// NOTA: MQL4/MQL5 debe usar WritePipeW, no esta función
// 
extern "C" __declspec(dllexport) int __stdcall WritePipe(INT_PTR handle, const char* data)
{
    if (handle == 0 || handle == (INT_PTR)INVALID_HANDLE_VALUE || data == NULL) {
        return -1;
    }

    HANDLE hPipe = (HANDLE)handle;
    DWORD dataLen = (DWORD)strlen(data);
    DWORD bytesWritten = 0;

    BOOL result = WriteFile(hPipe, data, dataLen, &bytesWritten, NULL);
    // NO FlushFileBuffers; ver comentario en WritePipeW

    return result ? (int)bytesWritten : -1;
}

// ============================================================================
// FUNCIÓN 4: ReadPipeLine (NO BLOQUEANTE con PeekNamedPipe)
// ============================================================================
// Lee una línea completa del pipe (hasta \n o hasta llenar buffer)
// 
// Parámetros:
//   - handle: Handle del pipe (usar 'long' en MQL)
//   - buffer: Buffer donde se almacenarán los datos leídos (UTF-8)
//   - bufferSize: Tamaño máximo del buffer (incluyendo null terminator)
// 
// Retorna:
//   - Número de bytes leídos si éxito (> 0, incluyendo \n)
//   - 0 si no hay datos disponibles (no bloquea)
//   - -1 si error
// 
// IMPORTANTE: Esta función NO bloquea. Si no hay datos, retorna 0 inmediatamente.
// El EA debe llamarla periódicamente (ej: en OnTimer cada 100-1000ms).
// 
// Nota: Lee byte a byte hasta \n (simple para i0). En i1+ optimizar con buffering.
// 
extern "C" __declspec(dllexport) int __stdcall ReadPipeLine(INT_PTR handle, char* buffer, int bufferSize)
{
    if (handle == 0 || handle == (INT_PTR)INVALID_HANDLE_VALUE || buffer == NULL || bufferSize <= 0) {
        return -1;
    }

    HANDLE hPipe = (HANDLE)handle;
    
    // Verificar si hay datos disponibles (no bloqueante)
    DWORD bytesAvailable = 0;
    if (!PeekNamedPipe(hPipe, NULL, 0, NULL, &bytesAvailable, NULL)) {
        // Error al hacer peek (pipe cerrado probablemente)
        return -1;
    }
    
    if (bytesAvailable == 0) {
        // No hay datos disponibles ahora, retornar sin bloquear
        return 0;
    }

    // Leer byte a byte hasta encontrar \n o llenar buffer
    int totalBytesRead = 0;
    while (totalBytesRead < bufferSize - 1) {
        DWORD bytesRead = 0;
        char byte;

        BOOL result = ReadFile(hPipe, &byte, 1, &bytesRead, NULL);

        if (!result) {
            // Error de lectura
            if (totalBytesRead > 0) {
                break; // Retornar lo que se leyó hasta ahora
            }
            return -1;
        }

        if (bytesRead == 0) {
            // No hay más datos disponibles
            break;
        }

        buffer[totalBytesRead++] = byte;

        // Si encontramos \n, terminamos la línea (incluimos el \n)
        if (byte == '\n') {
            break;
        }
    }

    // Null-terminate el string
    buffer[totalBytesRead] = '\0';

    return totalBytesRead;
}

// ============================================================================
// FUNCIÓN 5: ClosePipe
// ============================================================================
// Cierra el handle del pipe
// 
// Parámetros:
//   - handle: Handle del pipe retornado por ConnectPipe (usar 'long' en MQL)
// 
// IMPORTANTE: Siempre llamar en OnDeinit() del EA para evitar resource leaks
// 
extern "C" __declspec(dllexport) void __stdcall ClosePipe(INT_PTR handle)
{
    if (handle == 0 || handle == (INT_PTR)INVALID_HANDLE_VALUE) {
        return;
    }

    HANDLE hPipe = (HANDLE)handle;
    
    // i2b FIX: Cancelar I/O pendiente antes de cerrar
    // Esto evita que el handle quede en estado inconsistente
    CancelIoEx(hPipe, NULL);
    
    CloseHandle(hPipe);
}

// ============================================================================
// FUNCIÓN 6: GetPipeLastError (i2b - Debugging)
// ============================================================================
// Retorna el último error de Win32 de las operaciones de pipe
// 
// Retorna:
//   - Código de error Win32 (DWORD)
// 
// Códigos comunes:
//   - 0: ERROR_SUCCESS (sin error)
//   - 2: ERROR_FILE_NOT_FOUND (pipe no existe)
//   - 5: ERROR_ACCESS_DENIED (permisos)
//   - 109: ERROR_BROKEN_PIPE (pipe cerrado por el otro lado)
//   - 121: ERROR_SEM_TIMEOUT (WaitNamedPipe timeout)
//   - 231: ERROR_PIPE_BUSY (servidor no aceptando conexiones)
//   - 233: ERROR_NO_PROCESS_ON_OTHER_END (servidor cerrado)
// 
// Uso en MQL4:
//   int err = GetPipeLastError();
//   Log("ERROR", "Pipe error", "code=" + IntegerToString(err));
// 
extern "C" __declspec(dllexport) DWORD __stdcall GetPipeLastError()
{
    return GetLastError();
}

// ============================================================================
// DllMain - Punto de entrada de la DLL
// ============================================================================
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved)
{
    (void)hModule;
    (void)lpReserved;

    switch (ul_reason_for_call)
    {
    case DLL_PROCESS_ATTACH:
        // Inicialización (si fuera necesaria)
        break;
    case DLL_THREAD_ATTACH:
    case DLL_THREAD_DETACH:
    case DLL_PROCESS_DETACH:
        break;
    }
    return TRUE;
}

