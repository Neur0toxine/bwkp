# Contributor instructions

## Project overview

`bwkp` is a Go CLI that transfers records in both directions between
Bitwarden/Vaultwarden and KeePassXC without creating a plaintext export. Go
owns CLI and application orchestration, the pinned official Bitwarden Rust SDK
owns vault access and cryptography, and the pinned KeePassXC C++ core owns KDBX
reading, writing, and verification.

The repository targets Go 1.26.5, Rust 1.93.1 with edition 2024, C++20, and
KeePassXC 2.7.12. Treat `go.mod`, `Cargo.toml`, `Cargo.lock`, and
`native/kpdb/CMakeLists.txt` as the authoritative pins.

## Repository layout

- `cmd/bwkp` contains only the process entry point.
- `internal/cli` handles flags, prompts, endpoint selection, and output.
- `internal/app` orchestrates export/import and owns secret and session lifetime.
- `pkg/dto/{bw,kp}` contains neutral data-transfer objects.
- `pkg/convert` contains pure deterministic conversion in both directions.
- `pkg/bwapi` and `pkg/kpdb` expose small Go interfaces around native behavior.
- `internal/native` owns cgo declarations, native buffer copying, and frees.
- `native/bw` wraps the official Bitwarden Rust SDK; `native/ffi` exposes its C ABI.
- `native/kpdb` wraps the pinned KeePassXC core and its C++ ABI.
- `internal/atomicfile` stages, verifies, fsyncs, and atomically renames the
  encrypted KDBX candidate.
- `test/e2e` is a separate Go module backed by dockerized Vaultwarden.
- `docs/user` documents supported user behavior; `docs/tech` documents design,
  mapping, testing, security, builds, and releases.

Generated build output belongs under `target/` and `dist/`; do not commit it.

## Build and verification

Use `go tool mage` as the public build and test interface. Do not require
contributors to install a standalone Mage binary or invoke private build steps
when a Mage target exists.

Common targets are:

```text
go tool mage build
go tool mage lint
go tool mage test:unit
go tool mage test:native
go tool mage test:e2e
go tool mage coverage
go tool mage verify
```

- `test:unit` runs race-enabled Go tests and Rust workspace tests without a
  server.
- `test:native` builds the Rust and KeePassXC integrations and tests with the
  `native` build tag.
- `test:e2e` exercises authentication, synchronization, KDBX, imports, and
  attachments against disposable Vaultwarden infrastructure.
- `lint` runs golangci-lint, rustfmt checks, and Clippy with warnings denied.
- `verify` runs lint, coverage, unit tests, and native tests.

Run `go tool mage verify` before submitting. Run `go tool mage test:e2e` when
changing authentication, synchronization, attachments, native bindings, or
KDBX behavior. Unit tests must not need a network; server behavior belongs in
the dockerized e2e suite.

## Architecture and implementation rules

- Keep conversion code pure and deterministic under `pkg/convert`; API, SDK,
  KDBX file-format, filesystem, and UI details must not leak into it.
- Keep orchestration behind small interfaces so the Go core remains testable
  without native libraries or a server.
- Complete import conversion, folder resolution, and conflict planning before
  the first remote mutation. Preserve stable ordering in conversion and plans.
- Keep neutral DTOs independent of native SDK types. Translate native JSON and
  buffers at the boundary.
- Preserve the existing `native` build tag and portable non-native stubs when
  changing native integration code.
- Keep the C ABIs small and length-delimited. Never pass Go pointers into
  retained native state. Native sessions must remain opaque, native-owned
  handles, and every returned allocation must use its matching free function.
- Do not add another KDBX writer. `native/kpdb` must wrap the pinned KeePassXC
  core, and `native/bw` must use the pinned official Bitwarden Rust SDK.
- Catch Rust panics and C++ exceptions at their ABI boundaries; they must not
  unwind across languages.
- Follow the repository's gofmt/goimports and golangci-lint configuration for
  Go, and rustfmt/Clippy for Rust. Keep cgo-specific suppressions narrow.

## Security and data handling

- Never write decrypted vault data, credentials, plaintext JSON, session data,
  or attachment plaintext to an intermediate file. Only the permission-
  restricted encrypted candidate KDBX may touch disk before its atomic rename.
- Import must keep decrypted KDBX data in memory and must not create local
  output files.
- Do not log or commit secrets, vault contents, tokens, signed attachment URLs,
  password/key files, or unsanitized fixtures.
- Preserve restrictive secret-file checks, best-effort clearing, encrypted
  candidate verification, fsync, and atomic replacement behavior.
- Attachment downloads use signed URLs without an account bearer token but
  must retain the official Bitwarden CLI identity headers expected by servers
  and the e2e WAF fixture.

## Tests and mapping changes

- Add tests for every mapping change in both relevant directions. Cover exact
  field values, source metadata behavior, warnings/lossy behavior, hierarchy,
  and deterministic ordering as applicable.
- Prefer table-driven pure tests in `pkg/convert`; use interface fakes for
  orchestration in `internal/app`.
- Add native tests when changing buffer ownership, ABI behavior, KDBX
  compatibility, or the native build tag. Add e2e coverage for observable
  server behavior.
- Do not weaken the 70% core coverage gate or lint exclusions to make a change
  pass without explaining and testing the underlying behavior.

## Documentation and releases

- Update `docs/user` for user-visible CLI, installation, or behavior changes.
  Update the relevant `docs/tech` guide for architecture, mapping, security,
  testing, build, or release changes.
- Use focused Conventional Commit subjects (`fix:`, `feat:`, `docs:`,
  `test:`, `build:`, `chore:`). Keep unrelated tasks in separate commits.
- Preserve the upstream version output in `bwkp version` when updating either
  native dependency. Review upstream release notes and update mapping and
  compatibility tests.
- Platform support is incomplete until build/CI coverage, release packaging,
  runtime dependency documentation, and artifact-format verification agree.
- When an AI agent creates an issue or pull request from a repository template,
  leave `[x] Issue was created by AI` checked. Human authors should clear it.
