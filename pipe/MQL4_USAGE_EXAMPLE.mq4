//+------------------------------------------------------------------+
//|                                        MQL4_USAGE_EXAMPLE.mq4    |
//|                                  Ejemplo de uso de echo_pipe.dll |
//|                                        Echo Trade Copier v1.1.0  |
//+------------------------------------------------------------------+
#property copyright "Aranea Labs"
#property link      "https://github.com/xKoRx/echo"
#property version   "1.10"
#property strict

// ============================================================================
// IMPORT CORRECTO de echo_pipe.dll para MT4 (x86)
// ============================================================================
// IMPORTANTE:
// - Usar 'long' para handles (válido en 32 y 64 bits)
// - Usar 'string' para pasar strings (MQL convierte a wchar_t* automáticamente)
// - Usar WritePipeW (con W) para conversión UTF-16 → UTF-8
// ============================================================================

#import "echo_pipe_x86.dll"
   long ConnectPipe(string pipeName);           // Retorna handle o -1
   int  WritePipeW(long handle, string data);   // Convierte UTF-16 → UTF-8
   int  ReadPipeLine(long handle, char &buffer[], int size); // No bloqueante
   void ClosePipe(long handle);                 // Siempre llamar en OnDeinit
#import

// Variables globales
long g_PipeHandle = -1;  // Handle del pipe (usar long, no int)

//+------------------------------------------------------------------+
//| Expert initialization function                                   |
//+------------------------------------------------------------------+
int OnInit()
{
    Print("=== Echo Pipe DLL - Ejemplo de Uso ===");
    Print("Versión DLL: 1.1.0");
    Print("Account: ", AccountNumber());
    
    // Construir nombre del pipe: \\.\pipe\echo_master_<account_id>
    string pipeName = "\\\\.\\pipe\\echo_master_" + IntegerToString(AccountNumber());
    Print("Intentando conectar a: ", pipeName);
    
    // Conectar al pipe
    g_PipeHandle = ConnectPipe(pipeName);
    
    if (g_PipeHandle > 0) {
        Print("✓ Conectado exitosamente. Handle: ", g_PipeHandle);
        
        // Enviar handshake (JSON line-delimited, debe terminar en \n)
        string handshake = 
            "{\"type\":\"handshake\"," +
            "\"timestamp_ms\":" + IntegerToString(GetTickCount()) + "," +
            "\"payload\":{" +
                "\"client_id\":\"master_" + IntegerToString(AccountNumber()) + "\"," +
                "\"account_id\":\"" + IntegerToString(AccountNumber()) + "\"," +
                "\"broker\":\"" + AccountCompany() + "\"," +
                "\"role\":\"master\"," +
                "\"symbol\":\"XAUUSD\"," +
                "\"version\":\"1.1.0\"" +
            "}}\n";  // IMPORTANTE: terminar con \n
        
        int written = WritePipeW(g_PipeHandle, handshake);
        
        if (written > 0) {
            Print("✓ Handshake enviado: ", written, " bytes (UTF-8)");
        } else {
            Print("✗ Error enviando handshake");
        }
        
        // Configurar timer para polling de lectura (1 segundo)
        EventSetTimer(1);
        
    } else {
        Print("✗ Error conectando al pipe");
        Print("  Verificar que el Agent esté corriendo");
        Print("  Verificar nombre del pipe");
        return INIT_FAILED;
    }
    
    return INIT_SUCCEEDED;
}

//+------------------------------------------------------------------+
//| Expert deinitialization function                                 |
//+------------------------------------------------------------------+
void OnDeinit(const int reason)
{
    // CRÍTICO: Siempre cerrar el pipe para evitar resource leaks
    if (g_PipeHandle > 0) {
        ClosePipe(g_PipeHandle);
        Print("✓ Pipe cerrado");
        g_PipeHandle = -1;
    }
    
    EventKillTimer();
    
    Print("=== EA Detenido ===");
}

//+------------------------------------------------------------------+
//| Timer function (polling para leer del pipe)                      |
//+------------------------------------------------------------------+
void OnTimer()
{
    // ReadPipeLine es NO bloqueante: retorna 0 si no hay datos
    // Debemos llamarla periódicamente para leer mensajes del Agent
    
    if (g_PipeHandle <= 0) return;
    
    char buffer[8192];  // Buffer para leer (8KB es suficiente para i0)
    ArrayInitialize(buffer, 0);
    
    int bytesRead = ReadPipeLine(g_PipeHandle, buffer, 8192);
    
    if (bytesRead > 0) {
        // Convertir buffer a string
        string message = CharArrayToString(buffer, 0, bytesRead);
        Print("✓ Mensaje recibido (", bytesRead, " bytes): ", message);
        
        // Aquí procesar el mensaje JSON
        // En un EA real, parsear JSON y ejecutar acciones
        
    } else if (bytesRead == 0) {
        // No hay datos (normal, no hacer nada)
        
    } else {
        // bytesRead == -1: error
        Print("✗ Error leyendo del pipe. Reconectando...");
        
        // Intentar reconectar
        ClosePipe(g_PipeHandle);
        g_PipeHandle = -1;
        
        Sleep(5000); // Esperar 5s antes de reconectar
        
        string pipeName = "\\\\.\\pipe\\echo_master_" + IntegerToString(AccountNumber());
        g_PipeHandle = ConnectPipe(pipeName);
        
        if (g_PipeHandle > 0) {
            Print("✓ Reconectado");
        } else {
            Print("✗ Falló reconexión. Reintentando en próximo timer...");
        }
    }
}

//+------------------------------------------------------------------+
//| Ejemplo: Enviar TradeIntent                                      |
//+------------------------------------------------------------------+
void SendTradeIntent(int ticket, string symbol, string side, double lots, double price, int magic)
{
    if (g_PipeHandle <= 0) {
        Print("✗ No hay conexión al pipe");
        return;
    }
    
    // Generar UUID simple (en EA real, usar UUIDv7 del RFC)
    string trade_id = "01HKQV8Y-" + IntegerToString(GetTickCount()) + "-" + IntegerToString(magic);
    
    // Construir JSON line-delimited
    string json = 
        "{\"type\":\"trade_intent\"," +
        "\"timestamp_ms\":" + IntegerToString(GetTickCount()) + "," +
        "\"payload\":{" +
            "\"trade_id\":\"" + trade_id + "\"," +
            "\"client_id\":\"master_" + IntegerToString(AccountNumber()) + "\"," +
            "\"account_id\":\"" + IntegerToString(AccountNumber()) + "\"," +
            "\"symbol\":\"" + symbol + "\"," +
            "\"order_side\":\"" + side + "\"," +
            "\"lot_size\":" + DoubleToString(lots, 2) + "," +
            "\"price\":" + DoubleToString(price, Digits) + "," +
            "\"magic_number\":" + IntegerToString(magic) + "," +
            "\"ticket\":" + IntegerToString(ticket) + "," +
            "\"timestamps\":{" +
                "\"t0_master_ea_ms\":" + IntegerToString(GetTickCount()) +
            "}" +
        "}}\n";  // IMPORTANTE: terminar con \n
    
    // Enviar usando WritePipeW (convierte UTF-16 → UTF-8 automáticamente)
    int written = WritePipeW(g_PipeHandle, json);
    
    if (written > 0) {
        Print("✓ TradeIntent enviado: trade_id=", trade_id, ", bytes=", written);
    } else {
        Print("✗ Error enviando TradeIntent");
    }
}

//+------------------------------------------------------------------+
//| Tick function (ejemplo de uso)                                   |
//+------------------------------------------------------------------+
void OnTick()
{
    // Ejemplo: Detectar nueva orden y enviar TradeIntent
    // En un EA real, mantener array de tickets y detectar nuevas órdenes
    
    // Este es solo un ejemplo ilustrativo
    // NO usar en producción tal cual
}

//+------------------------------------------------------------------+
//| NOTAS IMPORTANTES                                                |
//+------------------------------------------------------------------+
// 1. Usar 'long' para handles, NO 'int' (evita truncamiento en x64)
// 2. Usar 'WritePipeW' (con W), NO 'WritePipe' (conversión UTF-16 → UTF-8)
// 3. Todos los mensajes JSON deben terminar en '\n' (line-delimited)
// 4. ReadPipeLine es NO bloqueante, llamar en OnTimer periódicamente
// 5. Siempre cerrar el pipe en OnDeinit() (ClosePipe)
// 6. Verificar handle > 0 antes de usarlo
// 7. Reconectar automáticamente si hay errores
//+------------------------------------------------------------------+

