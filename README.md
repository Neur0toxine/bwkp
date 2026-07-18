# bwkp

`bwkp` performs a fresh Bitwarden/Vaultwarden login and exports the decrypted
vault to a modern KeePassXC database. It does not use the Bitwarden CLI, keep a
session, or write an intermediate plaintext export.

The native compatibility boundary is deliberately narrow:

- Bitwarden access and decryption use the official Rust SDK 3.0.0, pinned to
  commit `7fd530e4852639d7391d062760891631ee9c15c1`.
- KDBX writing and verification use the statically linked KeePassXC 2.7.12
  C++ core. There is no independent KDBX implementation.

Run `bwkp version` to see both upstream versions compiled into a binary.

## Usage

Download a Linux or macOS archive for x86-64 or ARM64 from GitHub Releases,
unpack it, then run either:

```text
bwkp export --region us --email alice@example.com --output vault.kdbx
bwkp export --server https://vault.example.com --email alice@example.com --output vault.kdbx
```

The program prompts for the master password, authenticator code when required,
and target database password. For automation, each secret can be supplied from
a permission-restricted file. See [the user guide](docs/user/usage.md).

## Build and test

The supported build entry point is Mage:

```text
go tool mage build
go tool mage test:unit
go tool mage test:native
go tool mage test:e2e
go tool mage lint
```

`go tool mage build` downloads the pinned KeePassXC source and builds its core
as a static C++ archive before linking `bwkp`. The host needs Go 1.26.5, Rust
1.93.1, CMake, a C++20 compiler, and KeePassXC 2.7.12's Qt/Botan/Argon2 build
dependencies. The Dockerfile provides a contained build environment.

Technical design, mappings, security properties, and testing are documented
under [docs/tech](docs/tech/architecture.md).

## License

GPL-3.0. See [LICENSE.md](LICENSE.md).
