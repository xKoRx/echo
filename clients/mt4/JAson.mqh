//+------------------------------------------------------------------+
//|                                                        JAson.mqh |
//| Simplified JSON parser for Echo Trade Copier                     |
//| Based on JSON parsing best practices for MQL4/MQL5               |
//+------------------------------------------------------------------+
#property strict

//+------------------------------------------------------------------+
//| JSON Value Types                                                  |
//+------------------------------------------------------------------+
enum ENUM_JSON_TYPE
{
   JSON_NULL,
   JSON_BOOL,
   JSON_NUMBER,
   JSON_STRING,
   JSON_OBJECT,
   JSON_ARRAY
};

//+------------------------------------------------------------------+
//| JSON Parser Class                                                 |
//+------------------------------------------------------------------+
class CJAVal
{
private:
   ENUM_JSON_TYPE m_type;
   string         m_key;
   string         m_string_value;
   double         m_number_value;
   bool           m_bool_value;
   CJAVal*        m_items[];
   int            m_items_total;
   
   void Clear()
   {
      for(int i = 0; i < m_items_total; i++)
      {
         if(CheckPointer(m_items[i]) == POINTER_DYNAMIC)
            delete m_items[i];
      }
      ArrayResize(m_items, 0);
      m_items_total = 0;
   }
   
public:
   CJAVal() : m_type(JSON_NULL), m_key(""), m_string_value(""), m_number_value(0), m_bool_value(false), m_items_total(0) {}
   
   ~CJAVal() { Clear(); }
   
   // Setters
   void SetKey(string key) { m_key = key; }
   void SetString(string val) { m_type = JSON_STRING; m_string_value = val; }
   void SetNumber(double val) { m_type = JSON_NUMBER; m_number_value = val; }
   void SetBool(bool val) { m_type = JSON_BOOL; m_bool_value = val; }
   void SetNull() { m_type = JSON_NULL; }
   
   // Getters
   string GetKey() { return m_key; }
   ENUM_JSON_TYPE GetType() { return m_type; }
   
   string ToStr()
   {
      switch(m_type)
      {
         case JSON_STRING: return m_string_value;
         case JSON_NUMBER: return DoubleToString(m_number_value, 8);
         case JSON_BOOL:   return m_bool_value ? "true" : "false";
         case JSON_NULL:   return "null";
         default:          return "";
      }
   }
   
   double ToDouble()
   {
      if(m_type == JSON_NUMBER) return m_number_value;
      if(m_type == JSON_STRING) return StringToDouble(m_string_value);
      return 0.0;
   }
   
   int ToInt()
   {
      if(m_type == JSON_NUMBER) return (int)m_number_value;
      if(m_type == JSON_STRING) return (int)StringToInteger(m_string_value);
      return 0;
   }
   
   long ToLong()
   {
      if(m_type == JSON_NUMBER) return (long)m_number_value;
      if(m_type == JSON_STRING) return StringToInteger(m_string_value);
      return 0;
   }
   
   bool ToBool()
   {
      if(m_type == JSON_BOOL) return m_bool_value;
      if(m_type == JSON_STRING) return (m_string_value == "true" || m_string_value == "1");
      if(m_type == JSON_NUMBER) return m_number_value != 0;
      return false;
   }
   
   // Array/Object management
   void Add(CJAVal* item)
   {
      if(m_type != JSON_OBJECT && m_type != JSON_ARRAY)
         m_type = JSON_OBJECT;
      
      int size = ArraySize(m_items);
      ArrayResize(m_items, size + 1);
      m_items[size] = item;
      m_items_total++;
   }
   
   int Size() { return m_items_total; }
   
   CJAVal* GetItem(int index)
   {
      if(index >= 0 && index < m_items_total)
         return m_items[index];
      return NULL;
   }
   
   CJAVal* FindKey(string key)
   {
      for(int i = 0; i < m_items_total; i++)
      {
         if(m_items[i].GetKey() == key)
            return m_items[i];
      }
      return NULL;
   }
   
   // Parsing
   bool Deserialize(string json_string)
   {
      Clear();
      string trimmed = Trim(json_string);
      if(StringLen(trimmed) == 0) return false;
      
      int pos = 0;
      return ParseValue(trimmed, pos);
   }
   
private:
   string Trim(string s)
   {
      int i = 0, j = StringLen(s) - 1;
      while(i <= j && StringGetChar(s, i) <= 32) i++;
      while(j >= i && StringGetChar(s, j) <= 32) j--;
      if(j < i) return "";
      return StringSubstr(s, i, j - i + 1);
   }
   
   void SkipWhitespace(string str, int &pos)
   {
      int len = StringLen(str);
      while(pos < len && StringGetChar(str, pos) <= 32)
         pos++;
   }
   
   bool ParseValue(string str, int &pos)
   {
      SkipWhitespace(str, pos);
      if(pos >= StringLen(str)) return false;
      
      int ch = StringGetChar(str, pos);
      
      if(ch == '{') return ParseObject(str, pos);
      if(ch == '[') return ParseArray(str, pos);
      if(ch == '\"') return ParseString(str, pos);
      if(ch == 't' || ch == 'f') return ParseBool(str, pos);
      if(ch == 'n') return ParseNull(str, pos);
      if((ch >= '0' && ch <= '9') || ch == '-' || ch == '+') return ParseNumber(str, pos);
      
      return false;
   }
   
   bool ParseObject(string str, int &pos)
   {
      m_type = JSON_OBJECT;
      pos++; // Skip '{'
      
      while(pos < StringLen(str))
      {
         SkipWhitespace(str, pos);
         if(StringGetChar(str, pos) == '}')
         {
            pos++;
            return true;
         }
         
         // Parse key
         if(StringGetChar(str, pos) != '\"')
            return false;
         
         string key;
         pos++;
         int start = pos;
         while(pos < StringLen(str) && StringGetChar(str, pos) != '\"')
            pos++;
         key = StringSubstr(str, start, pos - start);
         pos++; // Skip closing quote
         
         SkipWhitespace(str, pos);
         if(StringGetChar(str, pos) != ':')
            return false;
         pos++; // Skip ':'
         
         // Parse value
         CJAVal* item = new CJAVal();
         item.SetKey(key);
         if(!item.ParseValue(str, pos))
         {
            delete item;
            return false;
         }
         Add(item);
         
         SkipWhitespace(str, pos);
         int ch = StringGetChar(str, pos);
         if(ch == ',')
         {
            pos++;
            continue;
         }
         if(ch == '}')
         {
            pos++;
            return true;
         }
         return false;
      }
      return false;
   }
   
   bool ParseArray(string str, int &pos)
   {
      m_type = JSON_ARRAY;
      pos++; // Skip '['
      
      while(pos < StringLen(str))
      {
         SkipWhitespace(str, pos);
         if(StringGetChar(str, pos) == ']')
         {
            pos++;
            return true;
         }
         
         CJAVal* item = new CJAVal();
         if(!item.ParseValue(str, pos))
         {
            delete item;
            return false;
         }
         Add(item);
         
         SkipWhitespace(str, pos);
         int ch = StringGetChar(str, pos);
         if(ch == ',')
         {
            pos++;
            continue;
         }
         if(ch == ']')
         {
            pos++;
            return true;
         }
         return false;
      }
      return false;
   }
   
   bool ParseString(string str, int &pos)
   {
      pos++; // Skip opening quote
      int start = pos;
      string result = "";
      
      while(pos < StringLen(str))
      {
         int ch = StringGetChar(str, pos);
         if(ch == '\\')
         {
            pos++;
            if(pos >= StringLen(str)) return false;
            int escaped = StringGetChar(str, pos);
            switch(escaped)
            {
               case 'n': result += "\n"; break;
               case 't': result += "\t"; break;
               case 'r': result += "\r"; break;
               case '\\': result += "\\"; break;
               case '\"': result += "\""; break;
               default: result += CharToString((ushort)escaped); break;
            }
            pos++;
         }
         else if(ch == '\"')
         {
            SetString(result);
            pos++;
            return true;
         }
         else
         {
            result += CharToString((ushort)ch);
            pos++;
         }
      }
      return false;
   }
   
   bool ParseNumber(string str, int &pos)
   {
      int start = pos;
      bool has_dot = false;
      
      if(StringGetChar(str, pos) == '-' || StringGetChar(str, pos) == '+')
         pos++;
      
      while(pos < StringLen(str))
      {
         int ch = StringGetChar(str, pos);
         if(ch >= '0' && ch <= '9')
         {
            pos++;
         }
         else if(ch == '.' && !has_dot)
         {
            has_dot = true;
            pos++;
         }
         else if(ch == 'e' || ch == 'E')
         {
            pos++;
            if(pos < StringLen(str) && (StringGetChar(str, pos) == '+' || StringGetChar(str, pos) == '-'))
               pos++;
            while(pos < StringLen(str) && StringGetChar(str, pos) >= '0' && StringGetChar(str, pos) <= '9')
               pos++;
            break;
         }
         else
            break;
      }
      
      string num_str = StringSubstr(str, start, pos - start);
      SetNumber(StringToDouble(num_str));
      return true;
   }
   
   bool ParseBool(string str, int &pos)
   {
      if(StringSubstr(str, pos, 4) == "true")
      {
         SetBool(true);
         pos += 4;
         return true;
      }
      if(StringSubstr(str, pos, 5) == "false")
      {
         SetBool(false);
         pos += 5;
         return true;
      }
      return false;
   }
   
   bool ParseNull(string str, int &pos)
   {
      if(StringSubstr(str, pos, 4) == "null")
      {
         SetNull();
         pos += 4;
         return true;
      }
      return false;
   }
};

//+------------------------------------------------------------------+
//| Helper functions for easy access                                 |
//+------------------------------------------------------------------+
string JAson_GetString(CJAVal* json, string key, string default_value = "")
{
   CJAVal* item = json.FindKey(key);
   if(item == NULL) return default_value;
   return item.ToStr();
}

double JAson_GetDouble(CJAVal* json, string key, double default_value = 0.0)
{
   CJAVal* item = json.FindKey(key);
   if(item == NULL) return default_value;
   return item.ToDouble();
}

int JAson_GetInt(CJAVal* json, string key, int default_value = 0)
{
   CJAVal* item = json.FindKey(key);
   if(item == NULL) return default_value;
   return item.ToInt();
}

long JAson_GetLong(CJAVal* json, string key, long default_value = 0)
{
   CJAVal* item = json.FindKey(key);
   if(item == NULL) return default_value;
   return item.ToLong();
}

bool JAson_GetBool(CJAVal* json, string key, bool default_value = false)
{
   CJAVal* item = json.FindKey(key);
   if(item == NULL) return default_value;
   return item.ToBool();
}

CJAVal* JAson_GetObject(CJAVal* json, string key)
{
   return json.FindKey(key);
}
//+------------------------------------------------------------------+

