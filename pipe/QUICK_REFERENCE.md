# Quick Reference - Echo Pipe DLL

## 🚀 Quick Start

```bash
# 1. Install MinGW (one-time)
sudo apt-get install mingw-w64

# 2. Build DLLs
cd /home/kor/go/src/github.com/xKoRx/echo/pipe
./build.sh

# 3. Output
ls -lh bin/
# → echo_pipe_x64.dll
# → echo_pipe_x86.dll
```

---

## 🔧 Build Commands

| Command | Description |
|---------|-------------|
| `./build.sh` | Build with MinGW (direct) - **Recommended** |
| `./build.sh cmake` | Build with CMake |
| `./build.sh test` | Show DLL exports |
| `./build.sh clean` | Clean artifacts |
| `make all` | Build with Makefile |
| `make x64` | Build x64 only |
| `make x86` | Build x86 only |
| `make clean` | Clean artifacts |

---

## 📖 API Summary

### ConnectPipe
```cpp
int ConnectPipe(const wchar_t* pipeName)
```
- Returns: `> 0` = handle, `-1` = error

### WritePipe
```cpp
int WritePipe(int handle, const char* data)
```
- Returns: bytes written or `-1`
- **Must** end with `\n`

### ReadPipeLine
```cpp
int ReadPipeLine(int handle, char* buffer, int bufferSize)
```
- Returns: bytes read, `0` = timeout, `-1` = error

### ClosePipe
```cpp
void ClosePipe(int handle)
```
- Always call in `OnDeinit()`

---

## 🎯 MQL4 Usage

```mql4
#import "echo_pipe.dll"
   int ConnectPipe(string pipeName);
   int WritePipe(int handle, string data);
   int ReadPipeLine(int handle, uchar &buffer[], int size);
   void ClosePipe(int handle);
#import

int g_PipeHandle = -1;

void OnInit() {
    g_PipeHandle = ConnectPipe("\\\\.\\pipe\\echo_master_12345");
    if (g_PipeHandle > 0) {
        Print("Connected!");
        string json = "{\"type\":\"handshake\"}\n";
        WritePipe(g_PipeHandle, json);
    }
}

void OnDeinit(const int reason) {
    if (g_PipeHandle > 0) {
        ClosePipe(g_PipeHandle);
    }
}
```

---

## 📦 Installation Checklist

- [ ] Build DLL with `./build.sh`
- [ ] Copy `bin/echo_pipe_x64.dll` or `bin/echo_pipe_x86.dll`
- [ ] Paste to MT4/MT5 → `MQL4/Libraries/`
- [ ] Rename to `echo_pipe.dll` (no suffix)
- [ ] Enable DLL imports: Tools → Options → Expert Advisors
- [ ] Test with simple EA

---

## 🔍 Verification Commands

```bash
# Check exports
x86_64-w64-mingw32-objdump -p bin/echo_pipe_x64.dll | grep "Export"

# Expected output:
# ConnectPipe
# WritePipe
# ReadPipeLine
# ClosePipe

# Run test
wine bin/test_pipe_x64.exe
```

---

## 🐛 Common Issues

| Problem | Solution |
|---------|----------|
| MinGW not found | `sudo apt-get install mingw-w64` |
| MT4 won't load DLL | Check architecture (x86 vs x64) |
| ConnectPipe returns -1 | Agent not running or wrong pipe name |
| Build permission error | `make clean && make all` |

---

## 📁 File Structure

```
pipe/
├── echo_pipe.cpp              # DLL source
├── test_pipe.cpp              # Test program
├── build.sh                   # Build script
├── Makefile                   # Make targets
├── CMakeLists.txt             # CMake config
├── README.md                  # Full docs
├── INSTALL.md                 # Installation guide
├── QUICK_REFERENCE.md         # This file
└── bin/                       # Output
    ├── echo_pipe_x64.dll
    ├── echo_pipe_x86.dll
    ├── test_pipe_x64.exe
    └── test_pipe_x86.exe
```

---

## 🔗 Related Docs

- [README.md](README.md) - Complete documentation
- [INSTALL.md](INSTALL.md) - Detailed installation guide
- [RFC-002](../docs/rfcs/RFC-002-iteration-0-implementation.md) - Implementation spec

---

## 📋 Pipe Name Format

- Master: `\\.\pipe\echo_master_<account_id>`
- Slave: `\\.\pipe\echo_slave_<account_id>`

Example: `\\.\pipe\echo_master_12345`

---

## 🎨 Message Format

JSON line-delimited (each message ends with `\n`):

```json
{"type":"trade_intent","payload":{...}}\n
```

---

**Built for Echo Trade Copier** 🏴‍☠️

