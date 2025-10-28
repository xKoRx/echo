# Changelog - echo_pipe.dll

## v1.1.0 (2025-10-24) - CORRECCIONES CRÍTICAS

### 🚨 Breaking Changes

- **Firmas de funciones actualizadas**: Todas las funciones ahora usan `INT_PTR` en lugar de `int` para handles
- **Nueva función WritePipeW**: Agregada para conversión UTF-16 → UTF-8 (RECOMENDADA para MQL4/MQL5)
- **ReadPipeLine ahora es NO bloqueante**: Usa `PeekNamedPipe` para verificar datos antes de leer

### ✅ Correcciones Aplicadas

#### 1. Uso de INT_PTR para Handles (Evita Truncamiento en x64)

**Antes (v1.0.0)**:
```cpp
extern "C" __declspec(dllexport) int __stdcall ConnectPipe(const wchar_t* pipeName);
extern "C" __declspec(dllexport) int __stdcall WritePipe(int handle, const char* data);
```

**Ahora (v1.1.0)**:
```cpp
extern "C" __declspec(dllexport) INT_PTR __stdcall ConnectPipe(const wchar_t* pipeName);
extern "C" __declspec(dllexport) int     __stdcall WritePipeW(INT_PTR handle, const wchar_t* wdata);
```

**Impacto**: En x64, los handles de Windows son punteros de 64 bits. Usar `int` (32 bits) causaba truncamiento.

**En MQL4/MQL5**: Importar handles como `long` (válido en 32 y 64 bits):
```mql4
#import "echo_pipe_x64.dll"
   long ConnectPipe(string pipeName);  // long, NO int
   int  WritePipeW(long handle, string data);
   void ClosePipe(long handle);
#import
```

---

#### 2. Agregado WritePipeW para UTF-16 → UTF-8

**Problema**: MQL4/MQL5 usa strings UTF-16, pero `WritePipe(const char*)` esperaba UTF-8.

**Solución**: Nueva función `WritePipeW` que:
1. Recibe `wchar_t*` desde MQL (UTF-16)
2. Convierte automáticamente a UTF-8 usando `WideCharToMultiByte`
3. Escribe UTF-8 al pipe

**Código**:
```cpp
extern "C" __declspec(dllexport) int __stdcall WritePipeW(INT_PTR handle, const wchar_t* wdata)
{
    // Conversión UTF-16 → UTF-8
    int bytesNeeded = WideCharToMultiByte(CP_UTF8, 0, wdata, -1, NULL, 0, NULL, NULL);
    char* utf8Buffer = (char*)HeapAlloc(GetProcessHeap(), 0, bytesNeeded);
    WideCharToMultiByte(CP_UTF8, 0, wdata, -1, utf8Buffer, bytesNeeded, NULL, NULL);
    
    // Escribir al pipe
    WriteFile((HANDLE)handle, utf8Buffer, strlen(utf8Buffer), &bytesWritten, NULL);
    HeapFree(GetProcessHeap(), 0, utf8Buffer);
    
    return bytesWritten;
}
```

**Uso en MQL**:
```mql4
string json = "{\"type\":\"handshake\"}\n";
int bytes = WritePipeW(handle, json);  // Conversión automática
```

---

#### 3. ReadPipeLine Ahora es NO Bloqueante

**Antes**: Bloqueaba hasta leer `\n` o error (problema para EAs)

**Ahora**: Usa `PeekNamedPipe` para verificar datos antes de leer:
```cpp
DWORD bytesAvailable = 0;
if (!PeekNamedPipe(hPipe, NULL, 0, NULL, &bytesAvailable, NULL)) {
    return -1;  // Error
}

if (bytesAvailable == 0) {
    return 0;  // No hay datos, retornar inmediatamente
}

// Solo leer si hay datos disponibles
ReadFile(hPipe, &byte, 1, &bytesRead, NULL);
```

**Uso en EA**:
```mql4
void OnTimer() {
    // Llamar periódicamente (cada 1s)
    char buffer[8192];
    int bytesRead = ReadPipeLine(handle, buffer, 8192);
    
    if (bytesRead > 0) {
        // Procesar mensaje
    } else if (bytesRead == 0) {
        // No hay datos (normal)
    } else {
        // Error
    }
}
```

---

#### 4. Validación Robusta de Handles

**Antes**:
```cpp
if (handle <= 0) return -1;
```

**Ahora**:
```cpp
if (handle == 0 || handle == (INT_PTR)INVALID_HANDLE_VALUE) {
    return -1;
}
```

**Motivo**: `INVALID_HANDLE_VALUE` es `-1` en Windows, pero debe compararse correctamente con `INT_PTR`.

---

#### 5. Verificación de SetNamedPipeHandleState

**Antes**: Ignoraba el retorno de `SetNamedPipeHandleState`

**Ahora**: Valida y cierra el pipe si falla:
```cpp
DWORD mode = PIPE_READMODE_BYTE;
BOOL result = SetNamedPipeHandleState(hPipe, &mode, NULL, NULL);

if (!result) {
    CloseHandle(hPipe);
    return (INT_PTR)INVALID_HANDLE_VALUE;
}
```

---

#### 6. Archivos .def para MSVC (Exports Limpios)

**Agregados**:
- `echo_pipe_x64.def` (x64)
- `echo_pipe_x86.def` (x86)

**Contenido**:
```
LIBRARY echo_pipe_x64
EXPORTS
    ConnectPipe
    WritePipeW
    WritePipe
    ReadPipeLine
    ClosePipe
```

**Compilación MSVC**:
```cmd
cl /LD /O2 /EHsc echo_pipe.cpp /Fe:echo_pipe_x64.dll /DEF:echo_pipe_x64.def
```

---

### 📦 Funciones Exportadas (v1.1.0)

| Función | Propósito | Recomendación |
|---------|-----------|---------------|
| `ConnectPipe` | Conectar al pipe | ✅ Usar siempre |
| `WritePipeW` | Escribir con conversión UTF-16→UTF-8 | ✅ **RECOMENDADA para MQL** |
| `WritePipe` | Escribir UTF-8 directamente | ⚠️ Legacy (solo para C/C++) |
| `ReadPipeLine` | Leer línea (NO bloqueante) | ✅ Usar en OnTimer |
| `ClosePipe` | Cerrar handle | ✅ Usar en OnDeinit |

---

### 🔄 Migración desde v1.0.0

#### En MQL4/MQL5:

**Cambiar**:
```mql4
// v1.0.0 (INCORRECTO)
#import "echo_pipe_x64.dll"
   int ConnectPipe(string pipeName);       // Trunca en x64
   int WritePipe(int handle, string data); // UTF-16 sin convertir
#import
```

**Por**:
```mql4
// v1.1.0 (CORRECTO)
#import "echo_pipe_x64.dll"
   long ConnectPipe(string pipeName);          // long, no int
   int  WritePipeW(long handle, string data);  // WritePipeW, no WritePipe
   void ClosePipe(long handle);
#import
```

#### En C++ (test programs):

**Cambiar**:
```cpp
// v1.0.0
typedef int (__stdcall *ConnectPipeFunc)(const wchar_t*);
int handle = ConnectPipe(L"\\\\.\\pipe\\...");
```

**Por**:
```cpp
// v1.1.0
typedef INT_PTR (__stdcall *ConnectPipeFunc)(const wchar_t*);
INT_PTR handle = ConnectPipe(L"\\\\.\\pipe\\...");
```

---

### 📚 Nuevos Archivos

- ✅ `echo_pipe_x64.def` - Definiciones de exports para MSVC x64
- ✅ `echo_pipe_x86.def` - Definiciones de exports para MSVC x86
- ✅ `MQL4_USAGE_EXAMPLE.mq4` - Ejemplo completo de uso en MQL4
- ✅ `CHANGELOG.md` - Este archivo

---

### 🧪 Testing

El programa `test_pipe.cpp` ha sido actualizado para probar:
- ✅ INT_PTR handles
- ✅ WritePipeW con UTF-16
- ✅ ReadPipeLine no bloqueante
- ✅ 5 funciones exportadas (incluyendo WritePipeW)

**Ejecutar**:
```bash
cd bin
wine test_pipe_x64.exe
```

---

### 🎯 Checklist de Verificación

Para confirmar que la migración fue exitosa:

- [ ] DLL compilada con nuevas firmas (INT_PTR)
- [ ] Exports visibles: `ConnectPipe`, `WritePipeW`, `WritePipe`, `ReadPipeLine`, `ClosePipe`
- [ ] EA actualizado: imports usan `long` para handles
- [ ] EA actualizado: usa `WritePipeW` en lugar de `WritePipe`
- [ ] ReadPipeLine retorna 0 cuando no hay datos (no bloqueante)
- [ ] Probado en x64 y x86
- [ ] Sin warnings de compilación
- [ ] Test suite pasa todos los tests

---

### 📖 Referencias

- [RFC-002 Sección 4.1](../docs/rfcs/RFC-002-iteration-0-implementation.md#41-especificación-completa-echo_pipedll)
- [MQL4_USAGE_EXAMPLE.mq4](MQL4_USAGE_EXAMPLE.mq4) - Ejemplo completo
- [README.md](README.md) - Documentación completa
- [INSTALL.md](INSTALL.md) - Guía de instalación

---

## v1.0.0 (2025-10-24) - Initial Release

- ✅ ConnectPipe, WritePipe, ReadPipeLine, ClosePipe
- ✅ x86 y x64 builds
- ✅ Static linking
- ⚠️ Handles como `int` (problema en x64)
- ⚠️ WritePipe sin conversión UTF-16 → UTF-8
- ⚠️ ReadPipeLine bloqueante

---

**Para más información, ver [README.md](README.md)**

