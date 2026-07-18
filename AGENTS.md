# Contributor instructions

- Use `go tool mage` as the public build and test interface.
- Keep conversion code pure and deterministic under `pkg/convert`; API or file
  format details must not leak into it.
- Keep the C ABIs small. Never pass Go pointers into retained native state.
- Do not add another KDBX writer. `native/kpdb` must wrap the pinned KeePassXC
  core, and `native/bw` must use the pinned official Bitwarden Rust SDK.
- Never write decrypted vault data, credentials, or attachment plaintext to an
  intermediate file. Only the encrypted candidate KDBX may touch disk.
- Add tests for every mapping change. Unit tests must not need a network;
  server behavior belongs in the dockerized e2e suite.
- Run `go tool mage verify` before submitting. Run `test:e2e` when changing
  authentication, synchronization, attachments, native bindings, or KDBX.
- Preserve the upstream version output in `bwkp version` when updating either
  native dependency. Review upstream release notes and update mapping tests.
