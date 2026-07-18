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
  "$repository/mingw-w64-i686-boost-1.90.0-3-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-qt5-base-5.15.16%2Bkde%2Br130-2-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-qt5-tools-5.15.16-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-qt5-svg-5.15.16%2Bkde%2Br5-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-libbotan-3.11.1-1-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-argon2-20190702-2-any.pkg.tar.zst" \
  "$repository/mingw-w64-i686-qrencode-4.1.1-2-any.pkg.tar.zst"
