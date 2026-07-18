# Usage

## Interactive export

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

Interactive terminals show two progress bars for vault/attachment downloads
and entry/KDBX conversion. Progress is automatically suppressed when standard
error is redirected; use `--no-progress` to disable it explicitly.

Existing files are refused. Add `--force` to replace one after the new database
has been written and verified.

By default, protected `BW.*` attributes are added only for Bitwarden data that
has no exact KeePassXC representation. Add `--append-source` to append every
entry's complete protected source JSON and source identity metadata for archival
or future recovery purposes.

## Non-interactive secrets

Use `--master-password-file`, `--totp-file`, and
`--database-password-file`. The program rejects secret files readable by group
or other users on Unix. `--key-file` adds an existing KeePass key file;
`--key-file-only` omits the database password.

## Database settings

- `--cipher aes256|chacha20`
- `--compression gzip|none`
- `--kdf-memory-kib N`
- `--kdf-parallelism N`
- `--kdf-target 1s` to let KeePassXC calibrate Argon2id
- `--kdf-iterations N` for a fixed iteration count

The default is KDBX 4.1, AES-256, gzip, and Argon2id calibrated near one second.

Use `bwkp version` in bug reports. It prints the exporter version, commit,
platform, KeePassXC version, and Bitwarden SDK version.
