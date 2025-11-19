//+------------------------------------------------------------------+
//|                                                  Persistence.mqh |
//|                                        Echo Trade Copier · i2b   |
//|                  Manejo robusto de persistencia binaria y CSV    |
//+------------------------------------------------------------------+
#property strict

// Estructura para el Master (Mapeo Ticket -> UUID)
struct TradeMapRecord
{
   int      ticket;
   uchar    trade_id[40]; // UUIDv7 fijo
   long     timestamp;    // Cuándo se registró
};

// Estructura para el Slave (Historial de Comandos)
struct CommandRecord
{
   uchar    command_id[40];
   uchar    trade_id[40];
   long     ticket;
   double   price;
   int      error_code;
   int      cmd_type;
   long     timestamp;
};

//+------------------------------------------------------------------+
//| Clase TradeMapper (Para el Master)                               |
//| Responsabilidad: Mantener consistencia Ticket <-> UUID           |
//+------------------------------------------------------------------+
class TradeMapper
{
private:
   string         m_Filename;
   string         m_ArchiveCsv;
   int            m_Tickets[];
   string         m_UUIDs[];
   int            m_Count;

   void Save()
   {
      int handle = FileOpen(m_Filename, FILE_BIN | FILE_WRITE);
      if(handle == INVALID_HANDLE) return;

      for(int i = 0; i < m_Count; i++)
      {
         TradeMapRecord rec;
         rec.ticket = m_Tickets[i];
         StringToCharArray(m_UUIDs[i], rec.trade_id, 0, 39);
         rec.timestamp = (long)TimeCurrent();
         FileWriteStruct(handle, rec);
      }
      FileClose(handle);
   }

   void ArchiveOrphan(int ticket, string uuid)
   {
      int handle = FileOpen(m_ArchiveCsv, FILE_CSV | FILE_READ | FILE_WRITE | FILE_COMMON);
      if(handle != INVALID_HANDLE)
      {
         FileSeek(handle, 0, SEEK_END);
         string line = IntegerToString(ticket) + "," + uuid + "," + TimeToString(TimeCurrent(), TIME_DATE|TIME_SECONDS) + ",CLOSED_WHILE_OFFLINE";
         FileWrite(handle, line);
         FileClose(handle);
      }
   }

public:
   TradeMapper(string filename, string archiveCsv)
   {
      m_Filename = filename;
      m_ArchiveCsv = archiveCsv;
      m_Count = 0;
   }

   void Load()
   {
      int handle = FileOpen(m_Filename, FILE_BIN | FILE_READ);
      if(handle == INVALID_HANDLE) return;

      m_Count = 0;
      ArrayResize(m_Tickets, 0);
      ArrayResize(m_UUIDs, 0);

      while(!FileIsEnding(handle))
      {
         TradeMapRecord rec;
         if(FileReadStruct(handle, rec) < sizeof(TradeMapRecord)) break;

         m_Count++;
         ArrayResize(m_Tickets, m_Count);
         ArrayResize(m_UUIDs, m_Count);
         m_Tickets[m_Count - 1] = rec.ticket;
         m_UUIDs[m_Count - 1] = CharArrayToString(rec.trade_id);
      }
      FileClose(handle);
   }

   // Reconciliación: Limpia del binario lo que no esté en activeTickets
   void Reconcile(int &activeTickets[])
   {
      int newCount = 0;
      int keptTickets[];
      string keptUUIDs[];
      bool changed = false;

      for(int i = 0; i < m_Count; i++)
      {
         bool isActive = false;
         for(int j = 0; j < ArraySize(activeTickets); j++)
         {
            if(m_Tickets[i] == activeTickets[j])
            {
               isActive = true;
               break;
            }
         }

         if(isActive)
         {
            newCount++;
            ArrayResize(keptTickets, newCount);
            ArrayResize(keptUUIDs, newCount);
            keptTickets[newCount - 1] = m_Tickets[i];
            keptUUIDs[newCount - 1] = m_UUIDs[i];
         }
         else
         {
            // El ticket estaba en disco pero ya no está en MT4 -> Archivar
            ArchiveOrphan(m_Tickets[i], m_UUIDs[i]);
            changed = true;
         }
      }

      if(changed)
      {
         m_Count = newCount;
         ArrayCopy(m_Tickets, keptTickets);
         ArrayCopy(m_UUIDs, keptUUIDs);
         Save(); // Reescribir binario limpio
      }
   }

   void Add(int ticket, string uuid)
   {
      // Verificar si ya existe para evitar duplicados
      for(int i=0; i<m_Count; i++) if(m_Tickets[i] == ticket) return;

      m_Count++;
      ArrayResize(m_Tickets, m_Count);
      ArrayResize(m_UUIDs, m_Count);
      m_Tickets[m_Count - 1] = ticket;
      m_UUIDs[m_Count - 1] = uuid;
      Save(); // Persistencia inmediata (Insert)
   }

   void Remove(int ticket)
   {
      int index = -1;
      for(int i=0; i<m_Count; i++) if(m_Tickets[i] == ticket) { index = i; break; }

      if(index >= 0)
      {
         // Desplazar array
         for(int i = index; i < m_Count - 1; i++)
         {
            m_Tickets[i] = m_Tickets[i + 1];
            m_UUIDs[i] = m_UUIDs[i + 1];
         }
         m_Count--;
         ArrayResize(m_Tickets, m_Count);
         ArrayResize(m_UUIDs, m_Count);
         Save(); // Persistencia inmediata (Delete)
      }
   }

   string GetUUID(int ticket)
   {
      for(int i = 0; i < m_Count; i++)
      {
         if(m_Tickets[i] == ticket) return m_UUIDs[i];
      }
      return "";
   }
   
   int GetCount() { return m_Count; }
   int GetTicketAt(int idx) { if(idx>=0 && idx<m_Count) return m_Tickets[idx]; return -1; }
};

//+------------------------------------------------------------------+
//| Clase CommandJournal (Para el Slave)                             |
//| Responsabilidad: Historial de idempotencia con Rolling Window    |
//+------------------------------------------------------------------+
class CommandJournal
{
private:
   string         m_Filename;
   string         m_ArchiveCsv;
   CommandRecord  m_History[];
   int            m_Count;
   int            m_MaxKeep;

   void Rewrite()
   {
      int handle = FileOpen(m_Filename, FILE_BIN | FILE_WRITE);
      if(handle == INVALID_HANDLE) return;

      for(int i = 0; i < m_Count; i++)
         FileWriteStruct(handle, m_History[i]);
      
      FileClose(handle);
   }

   void Archive(CommandRecord &rec)
   {
      int handle = FileOpen(m_ArchiveCsv, FILE_CSV | FILE_READ | FILE_WRITE | FILE_COMMON);
      if(handle != INVALID_HANDLE)
      {
         FileSeek(handle, 0, SEEK_END);
         string line = CharArrayToString(rec.command_id) + "," + 
                       CharArrayToString(rec.trade_id) + "," + 
                       IntegerToString(rec.ticket) + "," + 
                       DoubleToString(rec.price, 5) + "," + 
                       IntegerToString(rec.error_code) + "," + 
                       TimeToString(rec.timestamp, TIME_DATE|TIME_SECONDS);
         FileWrite(handle, line);
         FileClose(handle);
      }
   }

public:
   CommandJournal(string filename, string archiveCsv, int maxKeep = 500)
   {
      m_Filename = filename;
      m_ArchiveCsv = archiveCsv;
      m_MaxKeep = maxKeep;
      m_Count = 0;
   }

   void Load()
   {
      int handle = FileOpen(m_Filename, FILE_BIN | FILE_READ);
      if(handle == INVALID_HANDLE) return;

      // Cargar todo a temporal
      CommandRecord temp[];
      int total = 0;
      while(!FileIsEnding(handle))
      {
         CommandRecord rec;
         if(FileReadStruct(handle, rec) < sizeof(CommandRecord)) break;
         total++;
         ArrayResize(temp, total);
         temp[total-1] = rec;
      }
      FileClose(handle);

      // Aplicar Rolling Window si excede
      int start = 0;
      if(total > m_MaxKeep)
      {
         start = total - m_MaxKeep;
         
         // Archivar los que vamos a borrar
         for(int i=0; i<start; i++)
            Archive(temp[i]);

         // Reescribir archivo truncado
         int hWrite = FileOpen(m_Filename, FILE_BIN | FILE_WRITE);
         if(hWrite != INVALID_HANDLE)
         {
            for(int i=start; i<total; i++) FileWriteStruct(hWrite, temp[i]);
            FileClose(hWrite);
         }
      }

      // Cargar en memoria
      m_Count = 0;
      ArrayResize(m_History, 0);
      for(int i=start; i<total; i++)
      {
         m_Count++;
         ArrayResize(m_History, m_Count);
         m_History[m_Count-1] = temp[i];
      }
   }

   void Add(string commandId, string tradeId, long ticket, double price, int error, int type)
   {
      CommandRecord rec;
      StringToCharArray(commandId, rec.command_id, 0, 39);
      StringToCharArray(tradeId, rec.trade_id, 0, 39);
      rec.ticket = ticket;
      rec.price = price;
      rec.error_code = error;
      rec.cmd_type = type;
      rec.timestamp = (long)TimeCurrent();

      // Append a memoria
      m_Count++;
      ArrayResize(m_History, m_Count);
      m_History[m_Count-1] = rec;

      // Append a disco (rápido)
      int handle = FileOpen(m_Filename, FILE_BIN | FILE_READ | FILE_WRITE);
      if(handle != INVALID_HANDLE)
      {
         FileSeek(handle, 0, SEEK_END);
         FileWriteStruct(handle, rec);
         FileClose(handle);
      }
      
      // Check rotación en runtime (opcional, pero bueno para long-running)
      if(m_Count > m_MaxKeep + 50) // Buffer de 50 para no reescribir a cada rato
      {
         // Archivar los que vamos a borrar
         int remove = m_Count - m_MaxKeep;
         for(int i=0; i<remove; i++)
            Archive(m_History[i]);

         // Podar memoria (Manual shift para compatibilidad)
         if(remove > 0)
         {
            for(int i = 0; i < m_Count - remove; i++)
               m_History[i] = m_History[i + remove];
            
            ArrayResize(m_History, m_Count - remove);
            m_Count = ArraySize(m_History);
         }
         
         Rewrite(); // Reescribir disco limpio
      }
   }

   int FindIndex(string commandId)
   {
      for(int i = 0; i < m_Count; i++)
      {
         if(CharArrayToString(m_History[i].command_id) == commandId) return i;
      }
      return -1;
   }

   CommandRecord GetRecord(int idx)
   {
      return m_History[idx];
   }
};
