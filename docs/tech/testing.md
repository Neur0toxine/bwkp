# Testing

`test:unit` runs race-enabled Go tests and Rust workspace tests. Conversion is
table-driven and exercises hierarchy, fields, TOTP, passkeys, SSH keys,
attachments, history, archive/trash, and deterministic output.
Login tests cover typed two-factor and new-device challenges, including a
new-device retry followed by authenticator 2FA. Rust tests verify the distinct
`newDeviceOtp` identity parameter, the token endpoint's snake_case OAuth fields,
CLI device metadata, and form encoding.

`test:native` builds KeePassXC 2.7.12 and the Bitwarden SDK adapter, then runs
Go with the `native` build tag. Its KDBX test writes and reopens through the
linked core and independently invokes the installed KeePassXC CLI.

`test:e2e` starts a pinned Vaultwarden 1.36.0 compose project and registers
isolated source and destination accounts. It exports login, note, card,
identity, and SSH records plus multiple attachments into a real KDBX, edits
that database with KeePassXC CLI, then imports it through `bwkp`. It verifies
secure-note fallback for a generic unsupported KeePassXC entry and exercises
skip, update, delete, and duplicate conflict modes against the live destination
vault. Newer structured SDK types are covered in pure mapping tests because the
pinned Vaultwarden server does not accept them.

The suite is a separate Go module under `test/e2e`. Its test cases run
sequentially and use the Go testing lifecycle for disposable TLS certificates,
the Compose service, the WAF fixture, command timeouts, and teardown.

The server is fronted by a small WAF fixture which returns HTTP 401 when an
attachment or mutation lacks the official Bitwarden CLI user-agent, client
name, client version, and device type headers. Rust unit tests also assert all
four headers on the native attachment request. Attachment bytes are compared
in both transfer directions.

`coverage` gates the testable Go core packages at 70%. Native SDK code is gated
by its integration/e2e behavior because line coverage across cgo and C++/Rust
static libraries would be misleading. Every native build also checks the
uncompressed executable's dependency table before optional UPX packing, so
compression cannot disguise an unexpected shared-library dependency.
