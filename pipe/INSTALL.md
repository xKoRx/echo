# Installation and Setup Guide

## 📋 Prerequisites

### Linux (Ubuntu/Debian)

1. **Install MinGW-w64 cross-compiler**:
   ```bash
   sudo apt-get update
   sudo apt-get install mingw-w64
   ```

2. **Verify installation**:
   ```bash
   x86_64-w64-mingw32-g++ --version
   i686-w64-mingw32-g++ --version
   ```

3. **(Optional) Install CMake**:
   ```bash
   sudo apt-get install cmake
   ```

### Other Linux Distributions

- **Fedora/RHEL/CentOS**:
  ```bash
  sudo dnf install mingw64-gcc-c++ mingw32-gcc-c++
  ```

- **Arch Linux**:
  ```bash
  sudo pacman -S mingw-w64-gcc
  ```

### Windows

1. **Option A: MinGW-w64 for Windows**:
   - Download from: https://sourceforge.net/projects/mingw-w64/
   - Install to `C:\mingw-w64\`
   - Add to PATH: `C:\mingw-w64\mingw64\bin`

2. **Option B: Visual Studio 2019+**:
   - Download Community Edition (free)
   - Install "Desktop development with C++"
   - Use Developer Command Prompt

---

## 🔨 Building the DLL

### Method 1: Using build.sh (Recommended for Linux)

```bash
cd /home/kor/go/src/github.com/xKoRx/echo/pipe

# Make executable if not already
chmod +x build.sh

# Build both x86 and x64
./build.sh

# Output: bin/echo_pipe_x64.dll, bin/echo_pipe_x86.dll
```

### Method 2: Using Makefile

```bash
cd /home/kor/go/src/github.com/xKoRx/echo/pipe

# Build both architectures
make all

# Or build specific architecture
make x64
make x86

# Show DLL exports
make test

# Clean build artifacts
make clean
```

### Method 3: Using CMake

```bash
cd /home/kor/go/src/github.com/xKoRx/echo/pipe

# Build with CMake
./build.sh cmake

# Or manually:
mkdir -p build/x64
cd build/x64
cmake ../.. -DCMAKE_TOOLCHAIN_FILE=../../toolchain-mingw-x64.cmake
make
```

### Method 4: Manual Compilation

```bash
# x64 DLL
x86_64-w64-mingw32-g++ -shared -o echo_pipe_x64.dll echo_pipe.cpp \
    -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias -O2

# x86 DLL
i686-w64-mingw32-g++ -shared -o echo_pipe_x86.dll echo_pipe.cpp \
    -static-libgcc -static-libstdc++ -Wl,--add-stdcall-alias -O2

# x64 Test
x86_64-w64-mingw32-g++ -o test_pipe_x64.exe test_pipe.cpp \
    -static-libgcc -static-libstdc++ -O2

# x86 Test
i686-w64-mingw32-g++ -o test_pipe_x86.exe test_pipe.cpp \
    -static-libgcc -static-libstdc++ -O2
```

---

## ✅ Verification

### 1. Check Build Output

```bash
ls -lh bin/

# Expected output:
# echo_pipe_x64.dll  (~50-100 KB)
# echo_pipe_x86.dll  (~50-100 KB)
# test_pipe_x64.exe
# test_pipe_x86.exe
```

### 2. Verify Exports

```bash
# x64
x86_64-w64-mingw32-objdump -p bin/echo_pipe_x64.dll | grep "Export"

# x86
i686-w64-mingw32-objdump -p bin/echo_pipe_x86.dll | grep "Export"
```

Expected functions:
- `ConnectPipe`
- `WritePipe`
- `ReadPipeLine`
- `ClosePipe`

### 3. Run Test Executable (Windows or Wine)

#### On Windows:

```cmd
cd bin
test_pipe_x64.exe
```

#### On Linux with Wine:

```bash
# Install Wine if not already
sudo apt-get install wine wine64

# Run test
cd bin
wine test_pipe_x64.exe
```

Expected output:
```
================================================================
Echo Pipe DLL Test Suite
Version: 1.0.0
================================================================
[INFO] Architecture: x64
...
[OK] Basic DLL functionality verified!
```

---

## 📥 Installation in MetaTrader

### Step 1: Locate MT4/MT5 Data Folder

1. Open MetaTrader 4 or 5
2. Menu: **File → Open Data Folder**
3. Navigate to:
   - MT4: `MQL4/Libraries/`
   - MT5: `MQL5/Libraries/`

### Step 2: Copy DLL

1. Copy the appropriate DLL:
   - **32-bit MT4/MT5**: `bin/echo_pipe_x86.dll`
   - **64-bit MT4/MT5**: `bin/echo_pipe_x64.dll`

2. **Important**: Rename to `echo_pipe.dll` (remove architecture suffix)

3. Example:
   ```bash
   # From Linux
   cp bin/echo_pipe_x64.dll /path/to/MT4/MQL4/Libraries/echo_pipe.dll
   ```

### Step 3: Enable DLL Imports

1. In MT4/MT5: **Tools → Options → Expert Advisors**
2. Check: ✅ **Allow DLL imports**
3. Click **OK**

### Step 4: Verify Installation

Create a test EA (`TestEchoPipe.mq4`):

```mql4
#property strict

#import "echo_pipe.dll"
   int ConnectPipe(string pipeName);
   void ClosePipe(int handle);
#import

void OnInit() {
    Print("=== Testing echo_pipe.dll ===");
    
    int handle = ConnectPipe("\\\\.\\pipe\\echo_test");
    
    if (handle > 0) {
        Print("SUCCESS: DLL loaded and connected, handle=", handle);
        ClosePipe(handle);
    } else {
        Print("INFO: DLL loaded (pipe not found is expected if Agent not running)");
    }
    
    Print("Test completed!");
}
```

Compile and run. Check "Experts" tab in Terminal.

---

## 🐛 Troubleshooting

### Error: "mingw-w64: command not found"

**Solution**: Install MinGW-w64:
```bash
sudo apt-get install mingw-w64
```

### Error: "Cannot open output file ... Permission denied"

**Solution**: Clean and rebuild:
```bash
make clean
make all
```

### Error: MT4 can't load DLL

**Cause**: Architecture mismatch

**Solution**:
1. Check MT4 architecture in Task Manager
2. Use matching DLL (x86 or x64)
3. Ensure DLL is in correct Libraries folder

### Error: "The specified module could not be found"

**Cause**: Missing runtime dependencies

**Solution**: Ensure static linking was used (should be default with provided build scripts)

---

## 📦 Distribution

To distribute the DLLs to other users:

1. **Package both architectures**:
   ```bash
   cd bin
   zip echo_pipe_v1.0.0.zip echo_pipe_x64.dll echo_pipe_x86.dll
   ```

2. **Include README.md** with installation instructions

3. **Test on clean Windows installation** before releasing

---

## 🔄 Updating

To rebuild after code changes:

```bash
# Clean old builds
make clean

# Rebuild
make all

# Or with build.sh
./build.sh
```

---

## 📞 Support

If you encounter issues:

1. Check this document first
2. Verify MinGW is installed correctly
3. Check build logs for errors
4. Review [README.md](README.md) for API usage
5. Check MT4/MT5 logs in Experts tab

---

## ✨ Next Steps

After successful build:

1. ✅ DLLs compiled
2. → Install in MetaTrader (see Step 2 above)
3. → Test with Master/Slave EAs
4. → Integrate with Echo Agent

For full integration guide, see [RFC-002](../docs/rfcs/RFC-002-iteration-0-implementation.md).

---

**Good luck with your build! 🚀**

