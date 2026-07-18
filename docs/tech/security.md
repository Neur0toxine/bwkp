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
