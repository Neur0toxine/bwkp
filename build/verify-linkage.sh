#!/usr/bin/env bash
set -euo pipefail

binary=${1:?usage: verify-linkage.sh BINARY}
platform=${2:-host}

case "$(uname -s)" in
  Linux)
    if file "$binary" | grep -q 'ELF'; then
      needed=$(readelf -d "$binary" 2>/dev/null | sed -n 's/.*Shared library: \[\([^]]*\)\].*/\1/p')
      if [[ "$platform" == android ]]; then
        unexpected=$(printf '%s\n' "$needed" | grep -Ev '^(libc|libdl|liblog|libm)\.so$' || true)
      else
        unexpected=$needed
      fi
      if [[ -n "$unexpected" ]]; then
        echo "unexpected dynamic ELF dependencies:" >&2
        printf '%s\n' "$unexpected" | sed 's/^/  /' >&2
        exit 1
      fi
      if [[ "$platform" != android ]] && readelf -l "$binary" | grep -q 'INTERP'; then
        echo "ELF binary contains a dynamic interpreter" >&2
        exit 1
      fi
    fi
    ;;
  Darwin)
    unexpected=$(otool -L "$binary" | tail -n +2 | awk '{print $1}' | \
      grep -Ev '^(/usr/lib/|/System/Library/)' || true)
    if [[ -n "$unexpected" ]]; then
      echo "unexpected non-system macOS dependencies:" >&2
      printf '%s\n' "$unexpected" >&2
      exit 1
    fi
    ;;
  MINGW*|MSYS*)
    allowed='^(ADVAPI32|bcrypt|CRYPT32|DNSAPI|IPHLPAPI|KERNEL32|msvcrt|mswsock|ntdll|ole32|OLEAUT32|POWRPROF|RPCRT4|SHELL32|ucrtbase|USER32|USERENV|VERSION|WINMM|WS2_32|api-ms-win-[A-Za-z0-9-]+)\.dll$'
    unexpected=$(objdump -p "$binary" | sed -n 's/^\s*DLL Name: //p' | grep -Eiv "$allowed" || true)
    if [[ -n "$unexpected" ]]; then
      echo "unexpected non-system Windows dependencies:" >&2
      printf '%s\n' "$unexpected" | sed 's/^/  /' >&2
      exit 1
    fi
    ;;
esac
