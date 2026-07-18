# Architecture

The public Go packages define stable interfaces and neutral DTOs. Native SDK
types remain behind C ABIs so an upstream update does not ripple through the
converter.

```mermaid
flowchart LR
    CLI[cmd/bwkp + internal/cli] --> APP[internal/app]
    APP --> API[pkg/bwapi]
    API --> BW[native/bw: official Rust SDK]
    APP --> CVT[pkg/convert]
    CVT --> BDTO[pkg/dto/bw]
    CVT --> KDTO[pkg/dto/kp]
    APP --> DB[pkg/kpdb]
    DB --> AF[internal/atomicfile]
    AF --> KP[native/kpdb: KeePassXC C++ core]
```

## Package responsibilities

- `cmd/bwkp`: process entry point only.
- `internal/cli`: flags, prompting, endpoint selection, and user-facing output.
- `internal/app`: login/sync/download/convert/write orchestration through small
  interfaces; owns lifetime and secret clearing.
- `internal/native`: CGo declarations and buffer ownership for both native
  libraries. Non-native stubs keep unit tests portable.
- `internal/atomicfile`: same-directory encrypted staging, verification, fsync,
  and rename.
- `internal/prompt`, `internal/security`: terminal input, secret-file checks,
  and best-effort memory clearing.
- `pkg/bwapi`: endpoint and session interfaces plus the official-SDK adapter.
- `pkg/dto/bw`: decrypted, SDK-neutral vault snapshot.
- `pkg/dto/kp`: writer-neutral database tree.
- `pkg/convert`: pure deterministic mapping; no I/O and no SDK dependency.
- `pkg/kpdb`: database options and writer interface.
- `native/bw`: Rust ownership of login, 2FA, sync, organization crypto, item
  decryption, attachment download and decryption.
- `native/kpdb`: C++ ownership of KeePassXC object construction, KDF
  calibration, KDBX 4.1 writing, and authenticated reopen verification.
- `native/ffi`: Rust C ABI for Bitwarden only.

## Export sequence

```mermaid
sequenceDiagram
    actor User
    participant Go as Go orchestration
    participant BW as Bitwarden Rust SDK
    participant KP as KeePassXC core
    User->>Go: export flags + secrets
    Go->>BW: password login
    BW-->>Go: authenticated or authenticator required
    Go->>BW: retry with TOTP, remember=false
    Go->>BW: sync and decrypt snapshot
    Go->>BW: download/decrypt each attachment
    Go->>Go: deterministic DTO conversion
    Go->>KP: construct and write encrypted candidate
    Go->>KP: reopen candidate with target credentials
    Go->>Go: fsync and atomic rename
    Go-->>User: counts and final path
```

All cross-language calls exchange length-delimited JSON or byte buffers.
Sessions are opaque Rust-owned handles. C++ and Rust catch failures at their ABI
boundaries; foreign exceptions and panics never cross into Go.
