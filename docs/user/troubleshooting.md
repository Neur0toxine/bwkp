# Troubleshooting

- A TOTP is intentionally requested on every export or import when authenticator 2FA is
  configured. Codes are never remembered.
- Export's `target already exists` error is a safety check; use `--force` only
  when replacement is intended. Import never replaces the input KDBX file.
- A self-signed server currently requires trust through the operating system's
  certificate store. The `--ca-cert` transport hook is reserved but the pinned
  Bitwarden SDK does not yet expose a safe per-client root-store extension.
- Static Linux releases resolve server names through `/etc/hosts` and DNS. If a
  name exists only through an optional NSS plugin such as mDNS or LDAP, add an
  `/etc/hosts` entry or use its DNS name.
- If a database cannot be opened, include `bwkp version`, the command flags
  without secrets, and server version. Never attach the generated vault.
- Builds from source need a C++20 toolchain, CMake, pkg-config, Python, Make,
  curl, and Git. Mage downloads and builds the pinned native dependencies.
- Import refuses ambiguous destination matches before making changes. Rename or
  move duplicate records/folders, or use `--conflict duplicate` when intentional.
- Import mutations are not a server-side transaction. If an upload or network
  request fails partway through, inspect the destination vault and rerun with an
  appropriate `--conflict` policy; the default `skip` avoids recreating exact
  matches.
