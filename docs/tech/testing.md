# Testing

`test:unit` runs race-enabled Go tests and Rust workspace tests. Conversion is
table-driven and exercises hierarchy, fields, TOTP, passkeys, SSH keys,
attachments, history, archive/trash, and deterministic output.

`test:native` builds KeePassXC 2.7.12 and the Bitwarden SDK adapter, then runs
Go with the `native` build tag. Its KDBX test writes and reopens through the
linked core and independently invokes the installed KeePassXC CLI.

`test:e2e` starts a pinned Vaultwarden 1.36.0 compose project, registers an
isolated account, imports varied records, uploads an attachment, enables known
authenticator TOTP state, exports through `bwkp`, and inventories the result
with KeePassXC CLI. State is created under a temporary directory and removed by
the harness.

The server is fronted by a small WAF fixture which returns HTTP 401 for legacy
Vaultwarden `/attachments/...` downloads unless the request has an official
Bitwarden CLI user-agent. Attachment extraction is compared with the source
bytes after the resulting KDBX is opened by `keepassxc-cli`.

`coverage` gates the testable Go core packages at 70%. Native SDK code is gated
by its integration/e2e behavior because line coverage across CGo and C++/Rust
static libraries would be misleading.
