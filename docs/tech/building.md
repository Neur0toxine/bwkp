# Building bwkp

The supported interface for local builds and tests is Mage. Run targets as
`go tool mage <target>` from the repository root; this uses the Mage version
recorded in `go.mod` and does not require a separately installed `mage` binary.

## What the build contains

`bwkp` is a Go executable with two native components:

- `native/bw` and `native/ffi` compile as a Rust static library. They wrap the
  pinned official Bitwarden Rust SDK and own authentication, synchronization,
  encryption, mutations, and attachment transfer.
- `native/kpdb` populates the pinned KeePassXC source but compiles only its KDBX
  data model, crypto, key, stream, reader, and writer sources. The small project
  patch removes GUI-only includes from that target; it does not replace any
  KDBX behavior. KeePassXC remains the sole KDBX implementation.
- Mage builds checksum-pinned static QtCore/QtConcurrent, Botan, Argon2, and
  zlib archives, then cgo links the complete native closure into the command.
  Qt GUI, Widgets, Network, DBus, SVG, minizip, and QRencode are not built or
  linked.

The first build downloads Go modules, Cargo crates, the pinned Bitwarden SDK
Git revision, KeePassXC 2.7.12, Qt 5.15.18, Botan 3.11.1, Argon2 20190702, and
zlib 1.3.1. Source archives are checksum verified and generated output is kept
under `target/`. Subsequent builds reuse those inputs and archives. Network
access and Git are therefore required for a clean build.

## Host requirements

Use the exact Go and Rust versions declared by the project:

- Go 1.26.5, including cgo;
- Rust 1.93.1 and Cargo;
- Git, curl, Python 3, Make, CMake 3.21 or newer, pkg-config, a C++20 compiler,
  and the platform inspection tools (`file` plus `readelf`, `otool`, or
  `objdump`).

On Debian or Ubuntu, the native build dependencies used by CI can be installed
with:

```text
sudo apt-get update
sudo apt-get install binutils cmake curl file g++ git make pkg-config python3
```

On macOS with Homebrew:

```text
brew install cmake pkgconf
```

Windows releases use MSYS2's MINGW32, MINGW64, and CLANGARM64 environments for
32-bit x86, x86-64, and ARM64 respectively. Install the matching prefixed
toolchain, CMake, Ninja, and Python packages, then set `GOARCH`, `CC`, `CXX`,
and `CARGO_BUILD_TARGET` as shown in the release workflow. MINGW64 and
CLANGARM64 use MSYS2's `qt5-static` package; Mage source-builds the reduced Qt
target for MINGW32, where that package is unavailable. Botan, Argon2, and zlib
are source-built for all three targets.

Run Windows builds from the matching MSYS2 shell. The architecture settings are:

| MSYS2 environment | `GOARCH` | `CC` / `CXX` | `CARGO_BUILD_TARGET` |
| --- | --- | --- | --- |
| `MINGW32` | `386` | `gcc` / `g++` | `i686-pc-windows-gnu` |
| `MINGW64` | `amd64` | `gcc` / `g++` | `x86_64-pc-windows-gnu` |
| `CLANGARM64` | `arm64` | `clang` / `clang++` | `aarch64-pc-windows-gnullvm` |

For example, an x86-64 shell builds with:

```text
GOOS=windows GOARCH=amd64 CC=gcc CXX=g++ \
  CARGO_BUILD_TARGET=x86_64-pc-windows-gnu go tool mage build
```

Mage copies Cargo's target-specific static library to the stable archive path
used by cgo before linking `dist/bwkp.exe`. The executable is the complete
application artifact: its import table may reference Windows system DLLs, but
not Qt, Botan, MinGW, LLVM-MinGW, or other redistributable DLLs.

Both Intel and Apple-silicon macOS builds are native host builds. The release
and CI matrices use separate `macos-15-intel` Intel and `macos-15` ARM runners, so no
cross-compilation or universal-binary merge is involved. The resulting
`macos-amd64` and `macos-arm64` archives are selected by `Casks/bwkp.rb`. CI
also taps the checked-out repository and runs `brew audit --strict --cask` on
both macOS architectures, so cask metadata changes are checked with the native
builds they describe.

Linux output is a fully static ELF with no interpreter or `DT_NEEDED` entries.
Windows, macOS, and Android retain only their platform ABI: Windows system DLLs,
Apple system frameworks and `/usr/lib`, or Android Bionic libraries. All
third-party runtimes are embedded; a C/C++ runtime supplied as part of the OS
remains part of that platform ABI.

Static glibc must not load an NSS plugin from a host built with a different
glibc. Linux startup therefore selects glibc's built-in `files` and `dns` host
lookups before the SDK performs network resolution. `/etc/hosts` and ordinary
DNS names work; hostnames provided only by optional NSS plugins such as mDNS or
LDAP must also be made available through one of those two mechanisms. This is
a no-op on non-glibc Linux builds.

Additional tools are target-specific:

- Docker or Podman is required for container images and the e2e suite.
- Node.js 24, npm/npx, OpenSSH client tools, SQLite, and
  `keepassxc-cli` are required by `test:e2e`.
- Android builds require a Docker-compatible `docker` command. Mage downloads
  a pinned Termux package-builder snapshot and runs it in its official builder
  image; the Android NDK and Termux sysroot do not need to be installed on the
  host. Docker and Podman are both supported through that command.

## Build on the host

Clone the repository, install the host requirements, and build:

```text
git clone https://github.com/Neur0toxine/bwkp.git
cd bwkp
go tool mage build
```

The executable is written to `dist/bwkp` (`dist/bwkp.exe` on Windows). Supply
release metadata through environment variables when a reproducible version
string is needed:

```text
VERSION=1.2.3 COMMIT="$(git rev-parse HEAD)" \
  BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" go tool mage build
```

`go tool mage build` first creates the pinned static dependency sysroot, then
builds the Bitwarden SDK adapter, reduced KeePassXC core and bridge, and links
the native archives into the command with cgo. It verifies the uncompressed
artifact's dependency table before optional packing. Generated output lives
under `target/` and `dist/`. Linker symbols, debug tables, source paths, and
build identifiers are omitted from release-style binaries. When `upx` is in
`PATH`, Mage also packs Linux and Android executables with its best LZMA mode
and verifies the result. Set `BWKP_UPX=0` to leave a local build unpacked for
profiling or executable inspection. Release and container builds install the
pinned, checksum-verified UPX version automatically; UPX does not support the
macOS Mach-O output format. `build/install-upx.sh` supports x86-64 and ARM64
Linux hosts and installs the pinned executable into a requested directory; the
build still succeeds without UPX and reports that the binary was left unpacked.

Useful verification targets are:

```text
go tool mage test:unit
go tool mage test:native
go tool mage test:e2e
go tool mage lint
go tool mage coverage
go tool mage verify
```

Unit tests do not require a server. `test:native` builds and exercises the real
native bindings. `test:e2e` starts a disposable Vaultwarden service with
Compose and covers authentication, synchronization, KDBX, imports, and
attachments. `verify` runs lint, coverage, unit tests, and native tests; run the
e2e target separately whenever authentication, synchronization, attachments,
native bindings, or KDBX behavior changes.

## Android builds for Termux

Build one or both supported Android architectures with:

```text
go tool mage android:arm64
go tool mage android:armv7
go tool mage android:all
```

Mage checks out the pinned `termux-packages` revision into a temporary
directory, copies the current non-ignored worktree into it, and invokes the
official Termux package builder. Dependencies and toolchains are cached in a
named container volume. The resulting Android API 24 executables are
`dist/bwkp-android-arm64` and `dist/bwkp-android-armv7`. They are Bionic/Termux
binaries, not general GNU/Linux ARM binaries or APKs.

## Build with Docker or Podman

The repository Dockerfile provides a pinned Debian-based build environment and
a `scratch` runtime image. The public Mage target chooses Podman when installed,
otherwise Docker Buildx, and finally classic Docker:

```text
VERSION=dev BWKP_IMAGE=bwkp:dev go tool mage image
```

The equivalent direct commands are:

```text
podman build --target runtime --tag bwkp:dev .
docker buildx build --load --target runtime --tag bwkp:dev .
```

Run the container with arguments after the image name. Mount the destination
directory because the container filesystem is ephemeral:

```text
docker run --rm -it -v "$PWD/output:/output" bwkp:dev \
  export --region us --email alice@example.com --output /output/vault.kdbx
```

The runtime image is `scratch` plus the executable and CA certificate bundle;
it contains no package-manager runtime libraries. Container builds are useful
when CI and local builds should use the same toolchain or when build tools
should not be installed globally.
Host builds are faster for repeated development because compiler caches and
debugging tools are directly available. Containerization does not change the
secret-handling rules: mount only the required secret files and output
directory, and never bake credentials or decrypted vault data into an image.

## Troubleshooting

- A clean native build needs network access to the pinned source archives. A
  checksum error indicates a corrupted cache or changed upstream archive and
  must not be bypassed.
- A cgo linker error after changing native code can be stale output. Re-run
  `go tool mage build`; the target deliberately asks Go for a full relink.
- Docker Compose must be available as `docker compose` for e2e tests. Podman
  installations may provide this through an external Compose provider.
- Android build failures should be reproduced through the Mage Android target,
  because it pins the Termux package definitions and toolchain used by CI.
