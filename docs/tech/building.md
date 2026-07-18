# Building bwkp

The supported interface for local builds and tests is Mage. Run targets as
`go tool mage <target>` from the repository root; this uses the Mage version
recorded in `go.mod` and does not require a separately installed `mage` binary.

## What the build contains

`bwkp` is a Go executable with two native components:

- `native/bw` and `native/ffi` compile as a Rust static library. They wrap the
  pinned official Bitwarden Rust SDK and own authentication, synchronization,
  encryption, mutations, and attachment transfer.
- `native/kpdb` compiles the pinned KeePassXC core and the small project bridge
  as C++ static libraries. KeePassXC owns KDBX reading, writing, and
  verification; the project does not contain a second KDBX implementation.
- cgo links both native components into the Go command. Qt, Botan, Argon2,
  minizip, qrencode, zlib, and the platform C/C++ runtime remain system runtime
  dependencies.

The first build downloads Go modules, Cargo crates and the pinned Bitwarden SDK
Git revision, and KeePassXC 2.7.12 through CMake FetchContent. Subsequent builds
reuse the Go, Cargo, and CMake caches. Network access and Git are therefore
required for a clean build.

## Host requirements

Use the exact Go and Rust versions declared by the project:

- Go 1.26.5, including cgo;
- Rust 1.93.1 and Cargo;
- Git, CMake 3.21 or newer, pkg-config, and a C++20 compiler;
- Qt 5 development files for Core, Concurrent, DBus, GUI, Network, SVG, and
  Widgets;
- Botan 2 or 3, Argon2, minizip, qrencode, and zlib development files.

On Debian or Ubuntu, the native build dependencies used by CI can be installed
with:

```text
sudo apt-get update
sudo apt-get install cmake g++ git pkg-config qtbase5-dev qttools5-dev \
  libqt5svg5-dev libbotan-2-dev libargon2-dev libminizip-dev \
  libqrencode-dev
```

On macOS with Homebrew:

```text
brew install cmake qt@5 botan argon2 minizip qrencode
export PKG_CONFIG_PATH="$(brew --prefix qt@5)/lib/pkgconfig:$(brew --prefix botan)/lib/pkgconfig:$PKG_CONFIG_PATH"
```

Windows releases use MSYS2's MINGW32, MINGW64, and CLANGARM64 environments for
32-bit x86, x86-64, and ARM64 respectively. Install the matching prefixed
toolchain, CMake, Ninja, Qt 5, Botan, Argon2, minizip, QRencode, and zlib
packages, then set `GOARCH` and `CARGO_BUILD_TARGET` as shown in the release
workflow before running `go tool mage build`. MSYS2 has removed several 32-bit
packages from its active index, so x86 builds must first run
`build/install-msys2-x86-dependencies.sh` to install the pinned archived
dependency set used by CI and releases.

Both Intel and Apple-silicon macOS builds are native host builds. The release
and CI matrices use separate `macos-13` Intel and `macos-14` ARM runners, so no
cross-compilation or universal-binary merge is involved. The resulting
`macos-amd64` and `macos-arm64` archives are selected by `Casks/bwkp.rb`.

The runtime machine also needs the corresponding shared Qt, Botan, Argon2,
minizip, qrencode, zlib, and C++ runtime libraries. The release build embeds
the Go code, the official Bitwarden SDK wrapper, and the KeePassXC core, but it
is not a fully static executable.

Additional tools are target-specific:

- Docker or Podman is required for container images and the e2e suite.
- Node.js 24, npm/npx, OpenSSL, OpenSSH client tools, SQLite, and
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

`go tool mage build` performs these phases in order: Cargo builds the pinned
Bitwarden SDK adapter, CMake builds the pinned KeePassXC core and bridge, and Go
links the native archives into the command with cgo. Generated output lives
under `target/` and `dist/`. Linker symbols, debug tables, source paths, and
build identifiers are omitted from release-style binaries. When `upx` is in
`PATH`, Mage also packs Linux and Android executables with its best LZMA mode
and verifies the result. Set `BWKP_UPX=0` to leave a local build unpacked for
profiling or executable inspection. Release and container builds install the
pinned, checksum-verified UPX version automatically; UPX does not support the
macOS Mach-O output format.

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
a smaller runtime image. The public Mage target chooses Podman when installed,
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

Container builds are preferable when the host distribution lacks compatible
Qt/Botan development packages, when CI and local builds should use the same
toolchain, or when native build dependencies should not be installed globally.
Host builds are faster for repeated development because compiler caches and
debugging tools are directly available. Containerization does not change the
secret-handling rules: mount only the required secret files and output
directory, and never bake credentials or decrypted vault data into an image.

## Troubleshooting

- A pkg-config error normally means a Qt or native development package is
  missing, or Homebrew's Qt/Botan pkg-config directories are not in
  `PKG_CONFIG_PATH`.
- A cgo linker error after changing native code can be stale output. Re-run
  `go tool mage build`; the target deliberately asks Go for a full relink.
- Docker Compose must be available as `docker compose` for e2e tests. Podman
  installations may provide this through an external Compose provider.
- Android build failures should be reproduced through the Mage Android target,
  because it pins the Termux package definitions and toolchain used by CI.
