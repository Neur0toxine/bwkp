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

By default, an item that cannot be converted stops the export. Add
`--allow-lossy` to skip such items, print a warning for each one, and write the
remaining items to the database. Authentication, synchronization, attachment,
and KDBX write errors remain fatal.

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

## Import

Import uses the same endpoint and secret flags as export, but reads an existing
encrypted KDBX database:

```text
bwkp import --server https://vault.example.com --email alice@example.com --input vault.kdbx
```

The database is decrypted in memory by the pinned KeePassXC core, converted,
then written to the personal Bitwarden vault with the official SDK. Attachments
are encrypted before upload. No decrypted database, JSON export, credential, or
attachment is staged on disk.

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
