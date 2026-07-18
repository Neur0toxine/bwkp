cmake_minimum_required(VERSION 3.21)
project(bwkp_kpdb LANGUAGES CXX)

include(FetchContent)

set(KEEPASSXC_VERSION "2.7.12" CACHE STRING "Pinned KeePassXC compatibility version")
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(WITH_TESTS OFF CACHE BOOL "" FORCE)
set(WITH_GUI_TESTS OFF CACHE BOOL "" FORCE)
set(WITH_X11 OFF CACHE BOOL "" FORCE)
set(WITH_APP_BUNDLE OFF CACHE BOOL "" FORCE)
# KeePassXC 2.7.x option names.
set(WITH_XC_AUTOTYPE OFF CACHE BOOL "" FORCE)
set(WITH_XC_NETWORKING OFF CACHE BOOL "" FORCE)
set(WITH_XC_UPDATECHECK OFF CACHE BOOL "" FORCE)
set(WITH_XC_X11 OFF CACHE BOOL "" FORCE)
set(WITH_XC_DOCS OFF CACHE BOOL "" FORCE)
set(KPXC_MINIMAL ON CACHE BOOL "" FORCE)
set(KPXC_FEATURE_NETWORK OFF CACHE BOOL "" FORCE)
set(KPXC_FEATURE_UPDATES OFF CACHE BOOL "" FORCE)
set(KPXC_FEATURE_DOCS OFF CACHE BOOL "" FORCE)
set(KEEPASSXC_BUILD_TYPE Release CACHE STRING "" FORCE)
set(OVERRIDE_VERSION "${KEEPASSXC_VERSION}" CACHE STRING "" FORCE)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)
set(CMAKE_ARCHIVE_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/lib")
list(PREPEND CMAKE_PREFIX_PATH "${CMAKE_CURRENT_SOURCE_DIR}/cmake")
set(Qt5LinguistTools_DIR "${CMAKE_CURRENT_SOURCE_DIR}/cmake" CACHE PATH "" FORCE)

FetchContent_Declare(
    keepassxc
    GIT_REPOSITORY https://github.com/keepassxreboot/keepassxc.git
    GIT_TAG "${KEEPASSXC_VERSION}"
    GIT_SHALLOW TRUE
    GIT_PROGRESS TRUE
)
FetchContent_MakeAvailable(keepassxc)

# Give CGo a stable linker name across distributions where KeePassXC may use
# Botan 2 or Botan 3. The resulting executable still records Botan's SONAME.
if(UNIX AND NOT APPLE)
    file(WRITE "${CMAKE_ARCHIVE_OUTPUT_DIRECTORY}/libbwkp_botan.so" "INPUT(${BOTAN_LIBRARY})\n")
endif()

add_library(bwkp_kpdb STATIC bridge.cpp)
target_compile_features(bwkp_kpdb PRIVATE cxx_std_20)
target_compile_definitions(bwkp_kpdb PRIVATE BWKP_KEEPASSXC_VERSION="${KEEPASSXC_VERSION}")
target_include_directories(bwkp_kpdb PRIVATE
    "${keepassxc_SOURCE_DIR}/src"
    "${keepassxc_BINARY_DIR}/src"
    "${BOTAN_INCLUDE_DIR}"
)
target_link_libraries(bwkp_kpdb PRIVATE keepassx_core)
