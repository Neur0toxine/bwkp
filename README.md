# bwkp

`bwkp` transfers records in both directions between Bitwarden/Vaultwarden and
modern KeePassXC databases. It performs a fresh login for each export or
import, does not use the Bitwarden CLI, does not keep a session, and never
writes an intermediate plaintext export.

The native compatibility boundary is deliberately narrow:

- Bitwarden access and decryption use the official Rust SDK 3.0.0, pinned to
  commit `7fd530e4852639d7391d062760891631ee9c15c1`. Requests use the official
  Bitwarden CLI 2026.6.0 client identity and platform-specific user agent.
- KDBX reading, writing, and verification use the statically linked KeePassXC 2.7.12
  C++ core. There is no independent KDBX implementation.

Run `bwkp version` to see both upstream versions compiled into a binary.

## Installation

Release binaries dynamically use Qt, Botan, Argon2, minizip, QRencode, zlib,
and the platform C/C++ runtime. Windows archives include the required DLLs and
Homebrew installs the macOS libraries automatically; mise and manual Linux or
Android installs require the matching runtime packages described below.

### Windows

Download `windows-386` for 32-bit x86, `windows-amd64` for x86-64, or
`windows-arm64` for ARM64 from [GitHub Releases](https://github.com/Neur0toxine/bwkp/releases).
Download `SHA256SUMS` from the same release, verify the archive in PowerShell,
then extract it with the Windows `tar` command:

```powershell
$archive = "bwkp_v1.2.3_windows-amd64.tar.gz"
$expected = ((Select-String -Path SHA256SUMS -SimpleMatch $archive).Line -split '\s+')[0]
$actual = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLower()
if ($actual -ne $expected) { throw "Checksum verification failed" }
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\Programs\bwkp"
tar -xzf $archive -C "$env:LOCALAPPDATA\Programs\bwkp"
& "$env:LOCALAPPDATA\Programs\bwkp\bwkp.exe" version
$path = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", "$path;$env:LOCALAPPDATA\Programs\bwkp", "User")
```

Keep `bwkp.exe` and the bundled DLLs together. Add that directory to the user
`PATH` to invoke `bwkp` from new terminals. Git Bash users can instead run the
manual installer below; it detects all three supported Windows architectures.
There is no Chocolatey package yet.

### Linux

The recommended installer is [mise](https://mise.jdx.dev/). Its GitHub backend
selects the matching Linux architecture directly from GitHub Releases and adds
`bwkp` to the managed `PATH`:

```text
mise use --global github:Neur0toxine/bwkp
bwkp version
```

Install the runtime library family through the distribution package manager
first. Debian and Ubuntu builds need the Qt 5 Core, Concurrent, DBus, GUI,
Network, SVG, and Widgets libraries plus Botan 2, Argon2, minizip, QRencode,
zlib, and the C++ runtime. [asdf](https://asdf-vm.com/) can also manage release
tools, but it requires a tool-specific plugin; this project does not publish an
asdf plugin, so mise is the direct, plugin-free option.

### macOS

Homebrew selects a native Intel or Apple-silicon archive and installs its
runtime dependencies:

```text
brew tap Neur0toxine/bwkp https://github.com/Neur0toxine/bwkp
brew install --cask Neur0toxine/bwkp/bwkp
bwkp version
```

Intel Macs use the legacy `macos-amd64` build. M-series Macs use the native
`macos-arm64` build without Rosetta.

### Android

Android needs the dedicated `android-arm64` or `android-armv7` Termux binary;
the Linux ARM archives use a different C library and will not work. Install a
current Termux release and its runtime packages, then use the manual installer
below. See the [Termux instructions](docs/user/usage.md#android-and-termux) for
the required `pkg` commands.

### iOS

iOS is untested and has no native binary. Advanced users may experiment with
Alpine Linux in [iSH](https://ish.app/) or a 64-bit Linux virtual machine in
[UTM](https://mac.getutm.app/), but the published glibc binaries are not native
Alpine packages and iSH commonly emulates an unsupported 32-bit architecture.
UTM running a supported x86-64 or ARM64 Linux guest is the more plausible path.

### Manual installation

The installer defaults to the latest release, detects Windows (under Git Bash),
Linux, macOS, or Termux,
and verifies the selected archive against the release `SHA256SUMS`. It installs
to `./bin` unless `-b` is provided:

```text
curl -sSfL https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh | bash
wget -O- -q https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh | bash
```

Choose another directory or an exact release by passing installer arguments:

```text
curl -sSfL https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh |
  bash -s -- -b "$HOME/.local/bin" v1.2.3
```

Alternatively, download the archive for the operating system and architecture
from [GitHub Releases](https://github.com/Neur0toxine/bwkp/releases), verify it
with `SHA256SUMS`, extract it, and place `bwkp` in a directory on `PATH`.

## Usage

After installation, run either:

```text
bwkp export --region us --email alice@example.com --output vault.kdbx
bwkp export --server https://vault.example.com --email alice@example.com --output vault.kdbx
bwkp import --region us --email alice@example.com --input vault.kdbx
bwkp import --server https://vault.example.com --email alice@example.com --input vault.kdbx
```

The program prompts for the master password, authenticator code when required,
and target database password. For automation, each secret can be supplied from
a permission-restricted file. Run `bwkp export --help` or `bwkp import --help`
for complete command options and examples. See [the user guide](docs/user/usage.md)
for the same reference with additional explanations.

## Build and test

The supported build entry point is Mage:

```text
go tool mage build
go tool mage image
go tool mage android:arm64
go tool mage android:armv7
go tool mage test:unit
go tool mage test:native
go tool mage test:e2e
go tool mage lint
```

`go tool mage build` downloads the pinned KeePassXC source and builds its core
as a static C++ archive before linking `bwkp`. The host needs Go 1.26.5, Rust
1.93.1, CMake, a C++20 compiler, and KeePassXC 2.7.12's Qt/Botan/Argon2 build
dependencies. The Dockerfile provides a contained build environment.

The Android targets use the pinned Termux package toolchain in Docker and place
Termux-compatible binaries in `dist/`. See [the user guide](docs/user/usage.md#android-and-termux)
for installation and runtime dependencies.

`go tool mage image` builds the runtime image as `bwkp:dev` by default. It uses
Podman when available; otherwise it uses Docker Buildx with `--load`, falling
back to classic `docker build` when Buildx is unavailable. Set `BWKP_IMAGE` and
`VERSION` to select another image reference and version. The target fails when
neither Podman nor Docker is installed.

See the detailed guides for [usage](docs/user/usage.md),
[troubleshooting](docs/user/troubleshooting.md),
[architecture](docs/tech/architecture.md), [data mapping](docs/tech/mapping.md),
[security](docs/tech/security.md), [building and testing](docs/tech/building.md),
and the [release lifecycle](docs/tech/release.md).

## License

GPL-3.0. See [LICENSE.md](LICENSE.md).
