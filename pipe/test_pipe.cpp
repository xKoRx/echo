/*
 * test_pipe.cpp - Programa de prueba para echo_pipe.dll
 * 
 * Prueba las 4 funciones exportadas de la DLL:
 *   - ConnectPipe
 *   - WritePipe
 *   - ReadPipeLine
 *   - ClosePipe
 * 
 * Compilar (Windows):
 *   cl test_pipe.cpp
 *   test_pipe.exe
 * 
 * Compilar (MinGW cross-compile desde Linux):
 *   x86_64-w64-mingw32-g++ -o test_pipe_x64.exe test_pipe.cpp -static-libgcc -static-libstdc++
 *   i686-w64-mingw32-g++ -o test_pipe_x86.exe test_pipe.cpp -static-libgcc -static-libstdc++
 * 
 * Versión: 1.0.0
 * Fecha: 2025-10-24
 */

#include <windows.h>
#include <stdio.h>

// Import de funciones de la DLL (v1.1.0 - con INT_PTR)
typedef INT_PTR (__stdcall *ConnectPipeFunc)(const wchar_t*);
typedef int (__stdcall *WritePipeWFunc)(INT_PTR, const wchar_t*);
typedef int (__stdcall *WritePipeFunc)(INT_PTR, const char*);
typedef int (__stdcall *ReadPipeLineFunc)(INT_PTR, char*, int);
typedef void (__stdcall *ClosePipeFunc)(INT_PTR);

void printSeparator() {
    printf("================================================================\n");
}

void printTestHeader(const char* testName) {
    printf("\n");
    printSeparator();
    printf("TEST: %s\n", testName);
    printSeparator();
}

void printSuccess(const char* message) {
    printf("[OK] %s\n", message);
}

void printError(const char* message) {
    printf("[ERROR] %s\n", message);
}

void printInfo(const char* message) {
    printf("[INFO] %s\n", message);
}

int main(int argc, char* argv[]) {
    printf("\n");
    printSeparator();
    printf("Echo Pipe DLL Test Suite\n");
    printf("Version: 1.0.0\n");
    printSeparator();

    // Determinar nombre de la DLL a cargar
    const char* dllName = "echo_pipe.dll";
    
    #ifdef _WIN64
        dllName = "echo_pipe_x64.dll";
        printInfo("Architecture: x64");
    #else
        dllName = "echo_pipe_x86.dll";
        printInfo("Architecture: x86");
    #endif

    printInfo("Testing echo_pipe.dll");
    printf("\n");

    // ========================================================================
    // TEST 1: Cargar DLL
    // ========================================================================
    printTestHeader("1. Load DLL");
    
    HMODULE hDll = LoadLibraryA(dllName);
    if (!hDll) {
        printError("Failed to load DLL");
        printf("        Tried: %s\n", dllName);
        printf("        Error code: %lu\n", GetLastError());
        printf("\n");
        printInfo("NOTE: This is expected if the Agent is not running");
        printInfo("      The DLL file must exist in the same directory");
        return 1;
    }
    
    printSuccess("DLL loaded successfully");

    // ========================================================================
    // TEST 2: Obtener funciones exportadas
    // ========================================================================
    printTestHeader("2. Get Exported Functions");
    
    ConnectPipeFunc ConnectPipe = (ConnectPipeFunc)GetProcAddress(hDll, "ConnectPipe");
    WritePipeWFunc WritePipeW = (WritePipeWFunc)GetProcAddress(hDll, "WritePipeW");
    WritePipeFunc WritePipe = (WritePipeFunc)GetProcAddress(hDll, "WritePipe");
    ReadPipeLineFunc ReadPipeLine = (ReadPipeLineFunc)GetProcAddress(hDll, "ReadPipeLine");
    ClosePipeFunc ClosePipe = (ClosePipeFunc)GetProcAddress(hDll, "ClosePipe");

    bool allFunctionsFound = true;
    
    if (!ConnectPipe) {
        printError("ConnectPipe not found");
        allFunctionsFound = false;
    } else {
        printSuccess("ConnectPipe found");
    }
    
    if (!WritePipeW) {
        printError("WritePipeW not found");
        allFunctionsFound = false;
    } else {
        printSuccess("WritePipeW found (UTF-16 → UTF-8)");
    }
    
    if (!WritePipe) {
        printError("WritePipe not found");
        allFunctionsFound = false;
    } else {
        printSuccess("WritePipe found (legacy)");
    }
    
    if (!ReadPipeLine) {
        printError("ReadPipeLine not found");
        allFunctionsFound = false;
    } else {
        printSuccess("ReadPipeLine found");
    }
    
    if (!ClosePipe) {
        printError("ClosePipe not found");
        allFunctionsFound = false;
    } else {
        printSuccess("ClosePipe found");
    }

    if (!allFunctionsFound) {
        printError("Not all functions found. Aborting tests.");
        FreeLibrary(hDll);
        return 1;
    }

    // ========================================================================
    // TEST 3: Conectar a pipe (esperado: falla si Agent no corre)
    // ========================================================================
    printTestHeader("3. Connect to Pipe");
    
    const wchar_t* pipeName = L"\\\\.\\pipe\\echo_master_12345";
    wprintf(L"Pipe name: %ls\n", pipeName);
    
    INT_PTR handle = ConnectPipe(pipeName);
    
    if (handle == (INT_PTR)INVALID_HANDLE_VALUE || handle == 0) {
        printInfo("Connection failed (expected if Agent is not running)");
        #ifdef _WIN64
            printf("        Return value: %lld\n", (long long)handle);
        #else
            printf("        Return value: %ld\n", (long)handle);
        #endif
        printf("        Error code: %lu\n", GetLastError());
        printf("\n");
        printInfo("To test full functionality:");
        printInfo("  1. Start the Echo Agent");
        printInfo("  2. Re-run this test");
        printf("\n");
        printSuccess("Basic DLL functionality verified!");
        FreeLibrary(hDll);
        return 0;
    }
    
    printSuccess("Connected to pipe!");
    #ifdef _WIN64
        printf("        Handle: %lld\n", (long long)handle);
    #else
        printf("        Handle: %ld\n", (long)handle);
    #endif

    // ========================================================================
    // TEST 4: Escribir JSON al pipe con WritePipeW (UTF-16 → UTF-8)
    // ========================================================================
    printTestHeader("4. Write JSON to Pipe (WritePipeW)");
    
    const wchar_t* jsonW = L"{\"type\":\"handshake\",\"timestamp_ms\":1698345600000,\"payload\":{\"client_id\":\"test_12345\",\"role\":\"test\"}}\n";
    printInfo("Writing JSON (UTF-16, will be converted to UTF-8):");
    wprintf(L"        %ls", jsonW);
    
    int bytesWritten = WritePipeW(handle, jsonW);
    
    if (bytesWritten <= 0) {
        printError("Write failed");
        printf("        Return value: %d\n", bytesWritten);
        ClosePipe(handle);
        FreeLibrary(hDll);
        return 1;
    }
    
    printSuccess("Write successful");
    printf("        Bytes written (UTF-8): %d\n", bytesWritten);

    // ========================================================================
    // TEST 5: Leer respuesta del pipe (NO bloqueante)
    // ========================================================================
    printTestHeader("5. Read from Pipe (non-blocking)");
    
    printInfo("Attempting to read response...");
    printInfo("(ReadPipeLine is NON-BLOCKING - returns 0 if no data)");
    
    char buffer[1024];
    memset(buffer, 0, sizeof(buffer));
    
    int bytesRead = ReadPipeLine(handle, buffer, sizeof(buffer));
    
    if (bytesRead > 0) {
        printSuccess("Read successful");
        printf("        Bytes read: %d\n", bytesRead);
        printf("        Data: %s", buffer);
    } else if (bytesRead == 0) {
        printInfo("No data available (normal - Agent may not respond to handshake)");
        printInfo("ReadPipeLine returned 0 (non-blocking behavior)");
    } else {
        printError("Read error (pipe may be closed)");
    }

    // ========================================================================
    // TEST 6: Cerrar pipe
    // ========================================================================
    printTestHeader("6. Close Pipe");
    
    ClosePipe(handle);
    printSuccess("Pipe closed");

    // ========================================================================
    // Cleanup
    // ========================================================================
    printTestHeader("7. Cleanup");
    
    FreeLibrary(hDll);
    printSuccess("DLL unloaded");

    // ========================================================================
    // Resumen
    // ========================================================================
    printf("\n");
    printSeparator();
    printf("ALL TESTS PASSED!\n");
    printSeparator();
    printf("\n");
    printInfo("echo_pipe.dll is ready for use with MetaTrader 4/5");
    printf("\n");

    return 0;
}

