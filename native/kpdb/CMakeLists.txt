cmake_minimum_required(VERSION 3.21)
project(bwkp_kpdb LANGUAGES C CXX)

include(FetchContent)

set(KEEPASSXC_VERSION "2.7.12" CACHE STRING "Pinned KeePassXC compatibility version")
set(CMAKE_POSITION_INDEPENDENT_CODE ON)
set(CMAKE_AUTOMOC ON)
set(CMAKE_ARCHIVE_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/lib")

FetchContent_Declare(
    keepassxc
    GIT_REPOSITORY https://github.com/keepassxreboot/keepassxc.git
    GIT_TAG "${KEEPASSXC_VERSION}"
    GIT_SHALLOW TRUE
    GIT_PROGRESS TRUE
    PATCH_COMMAND
        "${CMAKE_COMMAND}"
        -DANDROID_BUILD=${ANDROID}
        -DSOURCE_DIR=<SOURCE_DIR>
        -P "${CMAKE_CURRENT_SOURCE_DIR}/cmake/PatchKeePassXC.cmake"
)

# KeePassXC 2.7.12 does not expose its KDBX implementation as an independent
# target. Populate the pinned source without configuring its GUI application,
# then build only the source files used for KDBX reading and writing.
if(POLICY CMP0169)
    cmake_policy(SET CMP0169 OLD)
endif()
FetchContent_GetProperties(keepassxc)
if(NOT keepassxc_POPULATED)
    FetchContent_Populate(keepassxc)
endif()

list(PREPEND CMAKE_MODULE_PATH "${keepassxc_SOURCE_DIR}/cmake")
find_package(Qt5 5.12 REQUIRED COMPONENTS Core Concurrent)
find_package(Botan REQUIRED)
find_package(ZLIB REQUIRED)
find_library(ARGON2_LIBRARY NAMES argon2 REQUIRED)
find_path(ARGON2_INCLUDE_DIR NAMES argon2.h REQUIRED)

if(BOTAN_VERSION VERSION_GREATER_EQUAL "3.0.0")
    set(WITH_XC_BOTAN3 TRUE)
elseif(BOTAN_VERSION VERSION_LESS "2.11.0")
    message(FATAL_ERROR "Botan 2.11.0 or newer is required")
endif()

set(KEEPASSXC_BUILD_TYPE Release)
set(KEEPASSXC_BUILD_TYPE_RELEASE ON)
set(PLUGIN_INSTALL_DIR "")
set(DATA_INSTALL_DIR "")
configure_file(
    "${keepassxc_SOURCE_DIR}/src/config-keepassx.h.cmake"
    "${CMAKE_CURRENT_BINARY_DIR}/config-keepassx.h"
)
set(git_info "#define GIT_HEAD \"\"\n#define GIT_BRANCH \"\"\n")
set(git_info_file "${CMAKE_CURRENT_BINARY_DIR}/git-info.h")
if(EXISTS "${git_info_file}")
    file(READ "${git_info_file}" existing_git_info)
endif()
if(NOT existing_git_info STREQUAL git_info)
    file(WRITE "${git_info_file}" "${git_info}")
endif()

set(keepassx_kdbx_SOURCES
    core/Alloc.cpp
    core/AutoTypeAssociations.cpp
    core/Base32.cpp
    core/Clock.cpp
    core/Config.cpp
    core/CustomData.cpp
    core/Database.cpp
    core/Entry.cpp
    core/EntryAttachments.cpp
    core/EntryAttributes.cpp
    core/FileWatcher.cpp
    core/Group.cpp
    core/Metadata.cpp
    core/ModifiableObject.cpp
    core/PasswordHealth.cpp
    core/SignalMultiplexer.cpp
    core/TimeDelta.cpp
    core/TimeInfo.cpp
    core/Tools.cpp
    core/Totp.cpp
    crypto/Crypto.cpp
    crypto/CryptoHash.cpp
    crypto/Random.cpp
    crypto/SymmetricCipher.cpp
    crypto/kdf/AesKdf.cpp
    crypto/kdf/Argon2Kdf.cpp
    crypto/kdf/Kdf.cpp
    format/Kdbx3Reader.cpp
    format/Kdbx3Writer.cpp
    format/Kdbx4Reader.cpp
    format/Kdbx4Writer.cpp
    format/KdbxReader.cpp
    format/KdbxWriter.cpp
    format/KdbxXmlReader.cpp
    format/KdbxXmlWriter.cpp
    format/KeePass2.cpp
    format/KeePass2RandomStream.cpp
    format/KeePass2Reader.cpp
    format/KeePass2Writer.cpp
    keys/ChallengeResponseKey.cpp
    keys/CompositeKey.cpp
    keys/FileKey.cpp
    keys/PasswordKey.cpp
    keys/drivers/YubiKey.h
    keys/drivers/YubiKeyStub.cpp
    streams/HashedBlockStream.cpp
    streams/HashingStream.cpp
    streams/HmacBlockStream.cpp
    streams/LayeredStream.cpp
    streams/qtiocompressor.cpp
    streams/StoreDataStream.cpp
    streams/SymmetricCipherStream.cpp
    thirdparty/zxcvbn/zxcvbn.c
)
list(TRANSFORM keepassx_kdbx_SOURCES PREPEND "${keepassxc_SOURCE_DIR}/src/")

add_library(keepassx_core STATIC ${keepassx_kdbx_SOURCES})
target_compile_features(keepassx_core PRIVATE cxx_std_20)
target_compile_definitions(keepassx_core PRIVATE
    BWKP_KDBX_ONLY
    KEEPASSX_BUILDING_CORE
    QT_NO_CAST_TO_ASCII
    QT_NO_DEBUG_OUTPUT
    QT_NO_EXCEPTIONS
    QT_STRICT_ITERATORS
)
target_compile_options(keepassx_core PRIVATE
    $<$<COMPILE_LANGUAGE:C,CXX>:-ffunction-sections>
    $<$<COMPILE_LANGUAGE:C,CXX>:-fdata-sections>
)
target_include_directories(keepassx_core
    PUBLIC
        "${keepassxc_SOURCE_DIR}/src"
        "${CMAKE_CURRENT_BINARY_DIR}"
    PRIVATE
        "${BOTAN_INCLUDE_DIR}"
        "${ARGON2_INCLUDE_DIR}"
        "${keepassxc_SOURCE_DIR}/src/thirdparty/zxcvbn"
)
target_link_libraries(keepassx_core PRIVATE
    Qt5::Core
    Qt5::Concurrent
    ${BOTAN_LIBRARIES}
    ${ARGON2_LIBRARY}
    ZLIB::ZLIB
)

# Give cgo a stable Botan linker name across systems with Botan 2 or 3.
if(UNIX AND NOT APPLE)
    set(botan_linker_script "INPUT(${BOTAN_LIBRARY})\n")
    set(botan_linker_script_file "${CMAKE_ARCHIVE_OUTPUT_DIRECTORY}/libbwkp_botan.a")
    if(EXISTS "${botan_linker_script_file}")
        file(READ "${botan_linker_script_file}" existing_botan_linker_script)
    endif()
    if(NOT existing_botan_linker_script STREQUAL botan_linker_script)
        file(WRITE "${botan_linker_script_file}" "${botan_linker_script}")
    endif()
endif()

add_library(bwkp_kpdb STATIC bridge.cpp)
target_compile_features(bwkp_kpdb PRIVATE cxx_std_20)
target_compile_definitions(bwkp_kpdb PRIVATE BWKP_KEEPASSXC_VERSION="${KEEPASSXC_VERSION}")
target_include_directories(bwkp_kpdb PRIVATE
    "${keepassxc_SOURCE_DIR}/src"
    "${CMAKE_CURRENT_BINARY_DIR}"
    "${BOTAN_INCLUDE_DIR}"
)
target_link_libraries(bwkp_kpdb PRIVATE keepassx_core Qt5::Core)
