# Troubleshooting

- A TOTP is intentionally requested on every export or import when authenticator 2FA is
  configured. Codes are never remembered.
- `target already exists` is a safety check; use `--force` only when replacement
  is intended.
- A self-signed server currently requires trust through the operating system's
  certificate store. The `--ca-cert` transport hook is reserved but the pinned
  Bitwarden SDK does not yet expose a safe per-client root-store extension.
- If a database cannot be opened, include `bwkp version`, the command flags
  without secrets, and server version. Never attach the generated vault.
- Builds from source need the dependency family used by KeePassXC 2.7.12,
  notably Qt 5, Botan, Argon2, Minizip, and QRencode development files.
- Import refuses ambiguous destination matches before making changes. Rename or
  move duplicate records/folders, or use `--conflict duplicate` when intentional.
