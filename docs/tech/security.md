# Security model

The exporter is stateless. It does not persist a Bitwarden session, device
token, plaintext JSON, decrypted attachment, or configuration file. The only
disk output is a permission-restricted encrypted KDBX candidate and its final
atomic rename.

Secrets are read without terminal echo or from files checked for restrictive
Unix permissions. Go and Rust buffers are cleared on normal return where their
runtimes permit it. This is defense in depth, not a guarantee against runtime,
allocator, swap, core-dump, or privileged process inspection.

The Rust ABI owns SDK sessions and allocations. Go receives opaque handles and
copies returned buffers before invoking the matching free function. The C++ ABI
has a separate allocator/free pair. Both boundaries catch language failures.

Database verification uses the same statically linked KeePassXC reader that
wrote the file, with the selected password/key file, before replacement. CI
also opens and inventories output with a separately installed KeePassXC CLI.

Vaultwarden attachment URLs are signed, unauthenticated `/attachments/...`
URLs and are fetched without the account bearer token. The download request
does carry the official Bitwarden CLI user-agent and platform headers so that
client-aware reverse proxies and WAF policies treat it consistently with API
requests.

Import reads KDBX through the pinned KeePassXC reader and keeps the decrypted
tree in memory. Record encryption and attachment encryption use the official
Bitwarden SDK. Mutation and attachment-upload requests carry the official CLI
user-agent plus mandatory client name, version, and platform device headers.
