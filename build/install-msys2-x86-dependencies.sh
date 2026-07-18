#!/usr/bin/env bash
set -euo pipefail

# MSYS2 retains these 32-bit packages in its archive after removing them from
# the active mingw32 package index. Install them together so pacman can resolve
# dependencies between the archived packages.
repository=https://repo.msys2.org/mingw/mingw32
pacman --noconfirm -U \
  "$repository/mingw-w64-i686-dbus-1.16.2-3-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-double-conversion-3.3.1-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-icu-76.1-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-md4c-0.5.2-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-boost-libs-1.90.0-3-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-boost-1.90.0-3-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-qt5-base-5.15.16%2Bkde%2Br130-2-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-qt5-tools-5.15.16-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-qt5-svg-5.15.16%2Bkde%2Br5-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-libbotan-3.11.1-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-argon2-20190702-2-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-qrencode-4.1.1-2-any.pkg.tar.zst"

# The archived Qt tools were built with GCC 14 and do not start with the GCC 16
# runtime from the active repository. Give only the host-side Qt generators a
# matching private runtime; keep the current compiler and target runtime intact.
qt_tools=/mingw32/qt5-host-tools
gcc_runtime=$(mktemp -d)
trap 'rm -rf "$gcc_runtime"' EXIT
curl --fail --location --silent --show-error \
  "$repository/mingw-w64-i686-gcc-libs-14.2.0-2-any.pkg.tar.zst" \
  --output "$gcc_runtime/gcc-libs.pkg.tar.zst"
bsdtar --extract --file "$gcc_runtime/gcc-libs.pkg.tar.zst" --directory "$gcc_runtime"

install -d "$qt_tools"
install -m 0755 /mingw32/bin/{moc,qmake,rcc,uic}.exe "$qt_tools"
install -m 0755 \
  /mingw32/bin/Qt5Core.dll \
  /mingw32/bin/libdouble-conversion.dll \
  /mingw32/bin/libicu{dt,in,uc}76.dll \
  /mingw32/bin/libpcre2-16-0.dll \
  /mingw32/bin/libwinpthread-1.dll \
  /mingw32/bin/libzstd.dll \
  /mingw32/bin/zlib1.dll \
  "$gcc_runtime/mingw32/bin/libgcc_s_dw2-1.dll" \
  "$gcc_runtime/mingw32/bin/libstdc++-6.dll" \
  "$qt_tools"

qt_tools_windows=$(cygpath -m "$qt_tools")
sed -i "s|\${_qt5Core_install_prefix}/bin/|${qt_tools_windows}/|g" \
  /mingw32/lib/cmake/Qt5Core/Qt5CoreConfigExtras.cmake
sed -i "s|\${_qt5Widgets_install_prefix}/bin/|${qt_tools_windows}/|g" \
  /mingw32/lib/cmake/Qt5Widgets/Qt5WidgetsConfigExtras.cmake
