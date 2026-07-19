# bwkp

`bwkp` moves vault data between Bitwarden or Vaultwarden and KeePassXC. Export
your Bitwarden vault to a KeePassXC database, or import a KeePassXC database
into Bitwarden, without creating a plaintext export file.

It is cross-platform and uses the upstream Bitwarden and KeePassXC components
directly to maximize compatibility and avoid problems that third-party
libraries can introduce:

- Bitwarden access and decryption use the official Rust SDK 3.0.0, pinned to
  its commit. Requests use the official Bitwarden CLI 2026.6.0 client identity
  and platform-specific user agent.
- KDBX reading, writing, and verification use the statically linked KeePassXC 2.7.12
  C++ core. There is no independent KDBX implementation.

Run `bwkp version` to see both upstream versions compiled into a binary.

## Installation

`bwkp` supports multiple platforms and installation methods. Release binaries
are deliberately fully static where possible, and otherwise near-static, so
they do not require third-party runtime dependencies.

<table>
  <thead>
    <tr>
      <th>Platform</th>
      <th>Installation method</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td rowspan="2">🪟 <strong>Windows</strong></td>
      <td>
        <strong>1. Release archive (recommended)</strong><br>
        Download <code>windows-386</code> for 32-bit x86, <code>windows-amd64</code> for x86-64, or
        <code>windows-arm64</code> for ARM64 from
        <a href="https://github.com/Neur0toxine/bwkp/releases">GitHub Releases</a>, extract it, and
        place <code>bwkp.exe</code> on <code>PATH</code>.
      </td>
    </tr>
    <tr>
      <td>
        <strong>2. Installer script (Git Bash or WSL)</strong><br>
        <code>curl -sSfL https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh | bash</code><br>
        Detects all three supported Windows architectures.
      </td>
    </tr>
    <tr>
      <td rowspan="3">🍎 <strong>macOS</strong></td>
      <td>
        <strong>1. Homebrew (recommended)</strong><br>
        <code>brew tap Neur0toxine/bwkp https://github.com/Neur0toxine/bwkp</code><br>
        <code>brew install --cask Neur0toxine/bwkp/bwkp</code><br>
        Selects the native Intel or Apple-silicon archive.
      </td>
    </tr>
    <tr>
      <td>
        <strong>2. Installer script</strong><br>
        <code>curl -sSfL https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh | bash</code>
      </td>
    </tr>
    <tr>
      <td>
        <strong>3. Release archive</strong><br>
        Download <code>macos-amd64</code> for Intel or <code>macos-arm64</code> for Apple silicon from
        <a href="https://github.com/Neur0toxine/bwkp/releases">GitHub Releases</a>, extract it, and
        place <code>bwkp</code> on <code>PATH</code>.
      </td>
    </tr>
    <tr>
      <td rowspan="3">🐧 <strong>Linux</strong></td>
      <td>
        <strong>1. mise (recommended)</strong><br>
        <code>mise use --global github:Neur0toxine/bwkp</code><br>
        Selects the matching architecture and adds <code>bwkp</code> to the managed <code>PATH</code>.
      </td>
    </tr>
    <tr>
      <td>
        <strong>2. Installer script</strong><br>
        <code>curl -sSfL https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh | bash</code>
      </td>
    </tr>
    <tr>
      <td>
        <strong>3. Release archive</strong><br>
        Download the matching archive from
        <a href="https://github.com/Neur0toxine/bwkp/releases">GitHub Releases</a>, extract it, and
        place <code>bwkp</code> on <code>PATH</code>.
      </td>
    </tr>
    <tr>
      <td>🤖 <strong>Android</strong></td>
      <td>
        <strong>Termux installer script</strong><br>
        Install a current Termux release, then run
        <code>curl -sSfL https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh | bash</code>.
        Use the dedicated <code>android-arm64</code> or
        <code>android-armv7</code> binary; Linux ARM archives are incompatible.
      </td>
    </tr>
    <tr>
      <td>📱 <strong>iOS</strong></td>
      <td>
        <strong>Linux virtual machine (experimental)</strong><br>
        There is no native iOS binary. Advanced users can try a supported x86-64 or ARM64 Linux guest
        in <a href="https://mac.getutm.app/">UTM</a>. iOS is untested; iSH commonly emulates an
        unsupported architecture.
      </td>
    </tr>
    <tr>
      <td>❓ <strong>Other</strong></td>
      <td>
        <strong>Build from source</strong><br>
        Follow the <a href="#build-and-test">build instructions</a> for unsupported platforms.
      </td>
    </tr>
  </tbody>
</table>

The installer uses the latest release by default and installs it to `./bin`:

```text
curl -sSfL https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh | bash
```

Pass `-b` to choose another directory. You can also select an exact release:

```text
curl -sSfL https://raw.githubusercontent.com/Neur0toxine/bwkp/master/install.sh |
  bash -s -- -b "$HOME/.local/bin" v1.2.3
```

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

`go tool mage build` downloads checksum-pinned Qt, Botan, Argon2, zlib, and
KeePassXC sources, builds the KDBX-only native closure, and links it into
`bwkp`. The host needs Go 1.26.5, Rust 1.93.1, Git, CMake, pkg-config, Python,
Make, curl, and a C++20 compiler. The Dockerfile provides a contained build
environment.

The Android targets use the pinned Termux package toolchain in Docker and place
Termux-compatible binaries in `dist/`. See [the user guide](docs/user/usage.md#android-and-termux)
for installation details.

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
