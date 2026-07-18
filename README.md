# bwkp

`bwkp` transfers records between Bitwarden/Vaultwarden and modern KeePassXC
databases. It performs a fresh login for each export or import, does not use the
Bitwarden CLI, does not keep a session, and never writes an intermediate
plaintext export.

The native compatibility boundary is deliberately narrow:

- Bitwarden access and decryption use the official Rust SDK 3.0.0, pinned to
  commit `7fd530e4852639d7391d062760891631ee9c15c1`. Requests use the official
  Bitwarden CLI 2026.6.0 client identity and platform-specific user agent.
- KDBX reading, writing, and verification use the statically linked KeePassXC 2.7.12
  C++ core. There is no independent KDBX implementation.

Run `bwkp version` to see both upstream versions compiled into a binary.

## Usage

Download a Linux, macOS, or Android archive for your architecture from GitHub
Releases, unpack it, then run either:

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

See the detailed guides for [architecture](docs/tech/architecture.md),
[building and testing](docs/tech/building.md), and the
[release lifecycle](docs/tech/release.md).

## License

GPL-3.0. See [LICENSE.md](LICENSE.md).
