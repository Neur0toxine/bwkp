#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
prefix=${BWKP_STATIC_PREFIX:-"$root/target/static"}
downloads="$root/target/downloads"
sources="$root/target/static-sources"
jobs=${CMAKE_BUILD_PARALLEL_LEVEL:-$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu)}

qt_version=5.15.18
botan_version=3.11.1
argon2_version=20190702
zlib_version=1.3.1

mkdir -p "$downloads" "$sources" "$prefix"

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

fetch() {
  local name=$1 url=$2 expected=$3 archive
  archive="$downloads/$name"
  if [[ ! -f "$archive" || $(hash_file "$archive") != "$expected" ]]; then
    rm -f "$archive"
    curl --fail --location --retry 3 --output "$archive" "$url"
  fi
  [[ $(hash_file "$archive") == "$expected" ]] || {
    echo "checksum mismatch for $name" >&2
    return 1
  }
}

extract() {
  local archive=$1 destination=$2 marker=$3
  if [[ ! -f "$destination/$marker" ]]; then
    rm -rf "$destination"
    mkdir -p "$destination"
    local force_local=()
    if [[ -n "${MSYSTEM:-}" ]]; then
      # MSYS2's GNU tar otherwise treats the drive-letter colon as a remote host.
      force_local=(--force-local)
    fi
    tar ${force_local[@]+"${force_local[@]}"} --extract --file "$archive" --directory "$destination" --strip-components=1
  fi
}

build_zlib() {
  [[ -f "$prefix/lib/libz.a" ]] && return
  fetch "zlib-$zlib_version.tar.gz" \
    "https://github.com/madler/zlib/releases/download/v$zlib_version/zlib-$zlib_version.tar.gz" \
    9a93b2b7dfdac77ceba5a558a580e74667dd6fede4585b91eefb60f03b72df23
  local source="$sources/zlib-$zlib_version"
  extract "$downloads/zlib-$zlib_version.tar.gz" "$source" CMakeLists.txt
  cmake -S "$source" -B "$source/build" -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX="$prefix" -DBUILD_SHARED_LIBS=OFF -DZLIB_BUILD_EXAMPLES=OFF
  cmake --build "$source/build" --parallel "$jobs"
  cmake --install "$source/build"
  if [[ -f "$prefix/lib/libzlibstatic.a" && ! -f "$prefix/lib/libz.a" ]]; then
    cp "$prefix/lib/libzlibstatic.a" "$prefix/lib/libz.a"
  fi
  rm -f \
    "$prefix/lib/libz.so" "$prefix/lib/libz.so.1" "$prefix/lib/libz.so.1.3.1" \
    "$prefix/lib/libz.dylib" "$prefix/lib/libz.1.dylib" "$prefix/lib/libz.1.3.1.dylib" \
    "$prefix/lib/libz.dll.a" "$prefix/bin/zlib1.dll"
}

build_argon2() {
  [[ -f "$prefix/lib/libargon2.a" ]] && return
  fetch "argon2-$argon2_version.tar.gz" \
    "https://github.com/P-H-C/phc-winner-argon2/archive/refs/tags/$argon2_version.tar.gz" \
    daf972a89577f8772602bf2eb38b6a3dd3d922bf5724d45e7f9589b5e830442c
  local source="$sources/argon2-$argon2_version"
  extract "$downloads/argon2-$argon2_version.tar.gz" "$source" Makefile
  make -C "$source" -j"$jobs" OPTTARGET=none LIBRARY_REL=lib LIBRARY=libargon2.a
  install -m 0644 "$source/libargon2.a" "$prefix/lib/libargon2.a"
  install -m 0644 "$source/include/argon2.h" "$prefix/include/argon2.h"
  mkdir -p "$prefix/lib/pkgconfig"
  sed -e "s|@UPSTREAM_VER@|$argon2_version|g" \
    -e "s|@PREFIX@|$prefix|g" "$root/build/argon2.pc.in" > "$prefix/lib/pkgconfig/libargon2.pc"
}

build_botan() {
  [[ -f "$prefix/lib/libbotan-3.a" ]] && return
  fetch "Botan-$botan_version.tar.xz" \
    "https://botan.randombit.net/releases/Botan-$botan_version.tar.xz" \
    c1cd7152519f4188591fa4f6ddeb116bc1004491f5f3c58aa99b00582eb8a137
  local source="$sources/Botan-$botan_version"
  extract "$downloads/Botan-$botan_version.tar.xz" "$source" configure.py
  pushd "$source" >/dev/null
  local target=()
  if [[ -n "${TERMUX_ARCH:-}" ]]; then
    local cpu
    case "$TERMUX_ARCH" in
      aarch64) cpu=arm64 ;;
      arm) cpu=arm32 ;;
      *) echo "unsupported Botan Android architecture: $TERMUX_ARCH" >&2; return 1 ;;
    esac
    target=(--os=android --cpu="$cpu" --cc=clang --cc-bin="$CXX")
  elif [[ -n "${MSYSTEM:-}" ]]; then
    case "$MSYSTEM" in
      MINGW32) target=(--os=mingw --cpu=x86_32 --cc=gcc) ;;
      MINGW64) target=(--os=mingw --cpu=x86_64 --cc=gcc) ;;
      CLANGARM64) target=(--os=mingw --cpu=arm64 --cc=clang) ;;
      *) echo "unsupported Botan MSYS2 environment: $MSYSTEM" >&2; return 1 ;;
    esac
  fi
  python3 configure.py --prefix="$prefix" --build-targets=static \
    --without-documentation --minimized-build \
    ${target[@]+"${target[@]}"} \
    --enable-modules=aes,auto_rng,cbc,chacha,ctr,gcm,hmac,salsa20,sha2_32,sha2_64,system_rng,twofish
  make -j"$jobs"
  make install
  popd >/dev/null
}

build_qt() {
  if [[ -n "${MINGW_PREFIX:-}" && -f "$MINGW_PREFIX/qt5-static/lib/libQt5Core.a" ]]; then
    return
  fi
  [[ -f "$prefix/lib/libQt5Core.a" && -f "$prefix/lib/libQt5Concurrent.a" ]] && return
  fetch "qtbase-$qt_version.tar.xz" \
    "https://download.qt.io/archive/qt/5.15/$qt_version/submodules/qtbase-everywhere-opensource-src-$qt_version.tar.xz" \
    7b632550ea1048fc10c741e46e2e3b093e5ca94dfa6209e9e0848800e247023b
  local source="$sources/qtbase-$qt_version"
  extract "$downloads/qtbase-$qt_version.tar.xz" "$source" configure
  mkdir -p "$source/build"
  pushd "$source/build" >/dev/null
  local platform=()
  if [[ -n "${MSYSTEM:-}" ]]; then
    platform=(-platform win32-g++)
  elif [[ -n "${TERMUX_ARCH:-}" ]]; then
    cp -R "$TERMUX_PREFIX/lib/qt/mkspecs/termux-cross" "$source/mkspecs/"
    platform=(-xplatform termux-cross -hostprefix "$source/host")
  fi
  ../configure -prefix "$prefix" ${platform[@]+"${platform[@]}"} -opensource -confirm-license -release -static \
    -nomake examples -nomake tests -no-gui -no-widgets -no-dbus -no-glib \
    -no-icu -no-openssl -no-cups -no-feature-network -no-feature-zstd \
    -qt-doubleconversion -qt-pcre -system-zlib \
    -I "$prefix/include" -L "$prefix/lib"
  make -j"$jobs" sub-src
  make install
  popd >/dev/null
}

build_zlib
build_argon2
build_botan
build_qt

printf '%s\n' "$prefix"
