# Usage

Both commands accept `--help`, `-help`, and `-h`. Help exits successfully and
prints invocation forms, all options and defaults, and examples:

```text
bwkp export --help
bwkp import --help
```

## macOS and Homebrew

The repository is a Homebrew tap. Add it with its explicit GitHub URL, then
install the cask:

```text
brew tap Neur0toxine/bwkp https://github.com/Neur0toxine/bwkp
brew install --cask Neur0toxine/bwkp/bwkp
```

The cask selects a separate release binary for Intel or Apple silicon and
installs the required Homebrew runtime libraries. `brew update` and
`brew upgrade bwkp` install later releases. Intel builds are retained for
legacy Macs; Apple-silicon Macs use the native ARM64 binary rather than Rosetta.

## Android and Termux

Release archives named `android-arm64` support modern 64-bit ARM Android
devices. Archives named `android-armv7` support 32-bit ARMv7 devices and
64-bit devices whose Android installation still provides 32-bit execution.
These are Android/Bionic executables for the standard Termux application, not
GNU/Linux ARM executables and not APKs.

Install a current Termux release, enable its X11 package repository, and install
the runtime libraries used by the pinned KeePassXC core:

```text
pkg update
pkg install x11-repo
pkg install argon2 botan3 libc++ libminizip libqrencode qt5-qtbase qt5-qtsvg zlib
```

Download and unpack the matching `bwkp_vVERSION_android-ARCH.tar.gz` release
inside Termux, then install it in Termux's executable path:

```text
tar -xzf bwkp_vVERSION_android-arm64.tar.gz
install -m 700 bwkp "$PREFIX/bin/bwkp"
bwkp version
```

Use `android-armv7` in the archive name on a 32-bit ARM device. The CLI works
normally in the Termux terminal; no graphical X11 server is needed. Keep vault
outputs and secret files in Termux-private storage when possible. Android builds
assume the standard `com.termux` application data prefix; repackaged Termux apps
with a different application ID are not supported by these archives.

## Export

For Bitwarden cloud, select the account region:

```text
bwkp export --region us --email alice@example.com --output vault.kdbx
```

For Vaultwarden, provide its externally reachable base URL:

```text
bwkp export --server https://vault.example.com --email alice@example.com --output vault.kdbx
```

Each invocation performs password login, requests an authenticator TOTP when
the server requires it, synchronizes the complete vault, downloads and decrypts
attachments in memory, writes an encrypted candidate, reopens it through
KeePassXC, then atomically installs the requested file. No session is reused.

Interactive terminals show progress for vault and attachment downloads, entry
conversion, and encrypted database writing. Progress is automatically
suppressed when standard error is redirected; use `--no-progress` to disable
it explicitly.

Existing files are refused. Add `--force` to replace one after the new database
has been written and verified.

By default, protected `BW.*` attributes are added only for Bitwarden data that
has no exact KeePassXC representation. Add `--append-source` to append every
entry's complete protected source JSON and source identity metadata for archival
or future recovery purposes.

By default, an item that cannot be converted stops the export. Add
`--allow-lossy` to skip such items, print a warning for each one, and write the
remaining items to the database. Authentication, synchronization, attachment,
and KDBX write errors remain fatal.

## Import

For Bitwarden cloud, select the destination account region:

```text
bwkp import --region us --email alice@example.com --input vault.kdbx
```

For Vaultwarden, provide its externally reachable base URL:

```text
bwkp import --server https://vault.example.com --email alice@example.com --input vault.kdbx
```

The database is decrypted in memory by the pinned KeePassXC core, converted,
then written to the personal Bitwarden vault with the official SDK. Attachments
are encrypted before upload. No decrypted database, JSON export, credential, or
attachment is staged on disk.

Interactive terminals show progress for database reading, entry conversion,
folder and item mutations, and attachment uploads. As with export, progress is
automatically suppressed when standard error is redirected and can be disabled
with `--no-progress`.

When a destination record already exists, `--conflict` selects one of four
behaviors:

- `skip` (default): leave the existing record unchanged.
- `delete`: move the existing record to trash and create a replacement.
- `duplicate`: always create another record.
- `update`: preserve the destination ID and replace its fields and attachments.

Source item ID is preferred when protected export metadata is present;
otherwise an exact folder, item type, and title match is used. Ambiguous matches
stop before any record is changed.

Generic KeePassXC records are inferred as logins, cards, identities, SSH keys,
or secure notes. Bitwarden source metadata restores bank accounts, driver
licenses, and passports. An unsupported item kind is imported as a complete
secure-note fallback and reported as a warning. Conversion failures are fatal
unless `--allow-lossy` is supplied; that flag permits the affected entries to
be skipped. `--append-source` adds protected `KP.SourceJSON` preservation data
without embedding attachment bytes.

## Credentials and non-interactive use

Both export and import prompt for the Bitwarden master password, an
authenticator code when required, and the KDBX password. Import asks for the
password of the input database; export asks for the password to set on the new
database.

Use `--master-password-file`, `--totp-file`, and
`--database-password-file`. The program rejects secret files readable by group
or other users on Unix. `--key-file` adds an existing KeePass key file;
`--key-file-only` omits the database password.

## Export database settings

- `--cipher aes256|chacha20`
- `--compression gzip|none`
- `--kdf-memory-kib N`
- `--kdf-parallelism N`
- `--kdf-target 1s` to let KeePassXC calibrate Argon2id
- `--kdf-iterations N` for a fixed iteration count

The default is KDBX 4.1, AES-256, gzip, and Argon2id calibrated near one second.

## Export option reference

Endpoint and account options:

- `--region us|eu`: select Bitwarden's US or EU cloud. Use either this or
  `--server`.
- `--server URL`: use a self-hosted Vaultwarden base URL.
- `--api-url URL` and `--identity-url URL`: advanced endpoint overrides.
- `--ca-cert FILE`: trust an additional PEM certificate authority for a
  self-hosted server.
- `--email EMAIL`: account email (required).
- `--output FILE`: destination `.kdbx` file (required).

Credential and output options:

- `--master-password-file FILE`, `--totp-file FILE`, and
  `--database-password-file FILE`: read secrets from protected files.
- `--key-file FILE`: add an existing KeePass key file.
- `--key-file-only`: protect the database only with the key file.
- `--force`: atomically replace an existing destination after verification.
- `--no-progress`: disable terminal progress.
- `--append-source`: append complete protected Bitwarden source metadata.
- `--allow-lossy`: skip unconvertible items and report warnings.

Database options:

- `--cipher aes256|chacha20` (default `aes256`).
- `--compression gzip|none` (default `gzip`).
- `--kdf-memory-kib N` (default `65536`).
- `--kdf-parallelism N` (default: up to four host CPUs).
- `--kdf-target DURATION` (default `1s`).
- `--kdf-iterations N`: use a fixed iteration count and disable calibration.

Use `bwkp version` in bug reports. It prints the exporter version, commit,
platform, KeePassXC version, and Bitwarden SDK version.

## Import option reference

Import accepts the same endpoint, account, CA certificate, secret file, key
file, `--key-file-only`, `--no-progress`, `--append-source`, and `--allow-lossy`
options documented above. Import-specific options are:

- `--input FILE`: source `.kdbx` file (required).
- `--conflict skip|delete|duplicate|update`: existing-item behavior (default
  `skip`).

Import does not accept export-only output and KDBX creation settings such as
`--output`, `--force`, `--cipher`, compression, or KDF tuning.
