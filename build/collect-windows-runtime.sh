#!/usr/bin/env bash
set -euo pipefail

binary=${1:?usage: collect-windows-runtime.sh BINARY DESTINATION}
destination=${2:?usage: collect-windows-runtime.sh BINARY DESTINATION}

mkdir -p "$destination"
cp "$binary" "$destination/bwkp.exe"

ldd "$binary" |
  awk '{ for (field = 1; field <= NF; field++) if ($field ~ /^\//) print $field }' |
  sort -u |
  while IFS= read -r library; do
    case "$library" in
      /mingw32/bin/*|/mingw64/bin/*|/clangarm64/bin/*)
        cp "$library" "$destination/"
        ;;
    esac
  done

# The archived MINGW32 dependencies require the GCC 14 runtime used to compile
# that target. Override ldd's active-repository resolution when CI supplies the
# isolated runtime directory.
if [[ -n "${BWKP_WINDOWS_GCC_RUNTIME_DIR:-}" ]]; then
  install -m 0755 \
    "$BWKP_WINDOWS_GCC_RUNTIME_DIR/libgcc_s_dw2-1.dll" \
    "$BWKP_WINDOWS_GCC_RUNTIME_DIR/libstdc++-6.dll" \
    "$destination"
fi
