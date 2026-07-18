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
