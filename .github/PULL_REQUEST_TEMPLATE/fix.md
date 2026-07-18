## Fix

Describe the defect, its root cause, and how this change corrects it.

Related issue: <!-- Use "Fixes #123" when applicable. -->

## Verification

List the exact commands and scenarios used to verify the fix.

## Checklist

<!-- AI agents must leave the checkbox below checked. Human authors must clear it. -->

- [x] Issue was created by AI
- [ ] The change is focused and includes a regression test where practical.
- [ ] `go tool mage verify` passes.
- [ ] `go tool mage test:e2e` passes, or the change does not affect authentication, synchronization, attachments, native bindings, or KDBX behavior.
- [ ] No credentials, decrypted vault data, or attachment plaintext were written to an intermediate file or committed.
- [ ] User-facing behavior and release notes are documented where needed.
