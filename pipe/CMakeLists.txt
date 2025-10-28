cmake_minimum_required(VERSION 3.10)
project(EchoPipe VERSION 1.0.0 LANGUAGES CXX)

# ============================================================================
# Configuration
# ============================================================================

set(CMAKE_CXX_STANDARD 11)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Detect architecture
if(CMAKE_SIZEOF_VOID_P EQUAL 8)
    set(ARCH_SUFFIX "_x64")
    message(STATUS "Building for x64 architecture")
else()
    set(ARCH_SUFFIX "_x86")
    message(STATUS "Building for x86 architecture")
endif()

# ============================================================================
# DLL Target: echo_pipe
# ============================================================================

add_library(echo_pipe SHARED
    echo_pipe.cpp
)

# Set DLL name with architecture suffix
set_target_properties(echo_pipe PROPERTIES
    OUTPUT_NAME "echo_pipe${ARCH_SUFFIX}"
    PREFIX ""  # No 'lib' prefix on Windows
)

# Windows-specific settings
if(WIN32)
    target_compile_definitions(echo_pipe PRIVATE _WIN32)
endif()

# MinGW-specific settings (cross-compile from Linux)
if(MINGW)
    message(STATUS "Using MinGW compiler")
    
    # Static linking to avoid runtime DLL dependencies
    target_link_options(echo_pipe PRIVATE
        -static-libgcc
        -static-libstdc++
        -Wl,--add-stdcall-alias  # Add stdcall aliases for MQL4 compatibility
    )
    
    # Optimization flags
    target_compile_options(echo_pipe PRIVATE
        -O2
        -Wall
        -Wextra
    )
endif()

# MSVC-specific settings
if(MSVC)
    message(STATUS "Using MSVC compiler")
    
    # Optimization flags
    target_compile_options(echo_pipe PRIVATE
        /O2
        /W4
        /EHsc
    )
endif()

# ============================================================================
# Test Executable Target: test_pipe
# ============================================================================

add_executable(test_pipe
    test_pipe.cpp
)

# Set executable name with architecture suffix
set_target_properties(test_pipe PROPERTIES
    OUTPUT_NAME "test_pipe${ARCH_SUFFIX}"
)

# MinGW-specific settings for test executable
if(MINGW)
    target_link_options(test_pipe PRIVATE
        -static-libgcc
        -static-libstdc++
    )
endif()

# ============================================================================
# Installation
# ============================================================================

# Install DLL to bin directory
install(TARGETS echo_pipe
    LIBRARY DESTINATION bin
    RUNTIME DESTINATION bin
)

# Install test executable
install(TARGETS test_pipe
    RUNTIME DESTINATION bin
)

# Install README and documentation
install(FILES
    README.md
    DESTINATION docs
    OPTIONAL
)

# ============================================================================
# Custom Targets
# ============================================================================

# Custom target to display exports (Windows only, requires dumpbin)
if(WIN32 AND NOT MINGW)
    add_custom_target(show_exports
        COMMAND dumpbin /exports $<TARGET_FILE:echo_pipe>
        DEPENDS echo_pipe
        COMMENT "Displaying DLL exports"
    )
endif()

# Custom target to display exports (MinGW, requires objdump)
if(MINGW)
    add_custom_target(show_exports
        COMMAND x86_64-w64-mingw32-objdump -p $<TARGET_FILE:echo_pipe> | grep "Export"
        DEPENDS echo_pipe
        COMMENT "Displaying DLL exports"
    )
endif()

# ============================================================================
# Print Configuration Summary
# ============================================================================

message(STATUS "")
message(STATUS "==================================================")
message(STATUS "Echo Pipe DLL Configuration Summary")
message(STATUS "==================================================")
message(STATUS "Version:           ${PROJECT_VERSION}")
message(STATUS "C++ Standard:      ${CMAKE_CXX_STANDARD}")
message(STATUS "Build Type:        ${CMAKE_BUILD_TYPE}")
message(STATUS "Architecture:      ${CMAKE_SIZEOF_VOID_P} bytes (${ARCH_SUFFIX})")
message(STATUS "Compiler:          ${CMAKE_CXX_COMPILER_ID}")
message(STATUS "DLL Output:        echo_pipe${ARCH_SUFFIX}.dll")
message(STATUS "Test Output:       test_pipe${ARCH_SUFFIX}.exe")
message(STATUS "==================================================")
message(STATUS "")

