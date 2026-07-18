# Release lifecycle

`bwkp` uses release-please to turn Conventional Commits on `master` into a
versioned release pull request. Merging that pull request creates the Git tag
and GitHub release; publishing the release starts the separate artifact build.
Maintainers normally do not edit the version, changelog, or tag by hand.

## Versioning policy

Releases follow Semantic Versioning: `MAJOR.MINOR.PATCH`.

- `PATCH` contains backward-compatible fixes, such as a `fix:` commit.
- `MINOR` adds backward-compatible behavior, normally from `feat:` commits.
- `MAJOR` contains an incompatible CLI, file behavior, or supported-interface
  change. Mark it with a `BREAKING CHANGE:` footer or `!` after the Conventional
  Commit type, for example `feat!:`.

Before 1.0, maintainers should still describe incompatible changes explicitly;
release-please applies its configured SemVer rules to calculate the next
version. The current released version is recorded in
`.release-please-manifest.json`. Tags omit a component name and use the form
`v1.2.3` because this repository releases one package.

Commit subjects are release input, not just internal history. Use a focused
Conventional Commit subject:

```text
fix: prevent progress output from repainting after completion
feat: add Termux arm64 releases
docs: document container builds
chore: update CI runner images
```

Put user-relevant context in the body. Keep unrelated changes in separate
commits so release notes can classify them correctly. A pull request title
should follow the same convention when the repository uses squash merges.

## Release preparation

Before merging a release pull request:

1. Confirm CI is green on `master` and run `go tool mage verify` locally.
2. Run `go tool mage test:e2e` when authentication, synchronization,
   attachments, native bindings, or KDBX behavior changed.
3. Review the generated version and `CHANGELOG.md`, including breaking changes
   and security-sensitive wording.
4. Confirm new user-facing behavior is documented and the release artifact
   matrix still covers every supported platform.
5. If the pinned Bitwarden SDK or KeePassXC version changed, review its upstream
   release notes, update mapping/compatibility tests, and confirm `bwkp version`
   still reports both upstream versions.

Do not publish a release from a dirty or unverified local worktree. Release
artifacts are built by GitHub Actions from the tagged commit, not uploaded from
a maintainer machine.

## Automatic changelog

On each push to `master`, `.github/workflows/release-please.yml` runs
release-please using `release-please-config.json`. It opens or refreshes one
release pull request containing:

- the next version in `.release-please-manifest.json`;
- generated `CHANGELOG.md` entries since the previous release;
- links back to the commits or pull requests represented by those entries.

The changelog is newest release first. Entries are grouped by Conventional
Commit category, typically sections such as Features and Bug Fixes, and each
entry is a concise bullet derived from the commit or squash-merge subject.
Breaking changes must be obvious in both the relevant group and the release
summary. Documentation and maintenance commits may appear in their own groups
according to release-please defaults, but they should not be disguised as
features or fixes merely to force a version bump.

Review generated prose like authored documentation. Correct unclear commit
subjects before merge when practical; for an exceptional correction, edit the
release pull request's changelog while preserving its structure so the next
release-please update can reconcile it.

## Triggering a release

The normal lifecycle is:

```text
Conventional commits merged to master
        -> release-please updates a release PR
        -> maintainer reviews and merges the release PR
        -> release-please creates tag vX.Y.Z and publishes a GitHub release
        -> the published-release workflow builds and uploads artifacts
        -> the checksum job publishes SHA256SUMS
```

Merging ordinary feature or fix pull requests does not immediately produce an
artifact release; it updates the pending release PR. Merging the release PR is
the maintainer's release trigger. The Actions UI can re-run a failed artifact
job for the same published release. Creating a tag or GitHub release manually
is reserved for recovery because it bypasses the version and changelog state
managed by release-please.

The workflows require repository permission to create pull requests, tags,
releases, attestations, and release assets. If release-please stops updating its
PR, check Actions permissions and branch protection before changing manifest
state.

## Release artifacts

The published-release workflow builds these archives:

| Target | Archive suffix | Runner/build path |
| --- | --- | --- |
| Linux x86-64 | `linux-amd64` | native build on Ubuntu x86-64 |
| Linux ARM64 | `linux-arm64` | native build on Ubuntu ARM64 |
| macOS Intel | `macos-amd64` | native build on macOS Intel |
| macOS Apple silicon | `macos-arm64` | native build on macOS ARM64 |
| Windows x86 | `windows-386` | MinGW build on Windows x86-64 |
| Windows x86-64 | `windows-amd64` | MinGW build on Windows x86-64 |
| Windows ARM64 | `windows-arm64` | LLVM-MinGW build on Windows ARM64 |
| Termux Android ARM64 | `android-arm64` | pinned Termux cross-builder |
| Termux Android ARMv7 | `android-armv7` | pinned Termux cross-builder |

These cover the popular desktop/server 64-bit architectures and both current
64-bit ARM and legacy 32-bit ARM Termux devices. Adding a supported platform is
not complete until its Mage target, CI build, release matrix entry, system-ABI
policy, and a real artifact-format check all exist.

Each file is named `bwkp_vX.Y.Z_TARGET.tar.gz` and contains:

- one executable named `bwkp` (`bwkp.exe` on Windows);
- `README.md`;
- `LICENSE.md`.

Windows archives contain only `bwkp.exe` and the two documentation files. The
workflow builds each architecture in its matching MINGW32, MINGW64, or
CLANGARM64 environment and rejects imports of non-system DLLs. The checksum job
waits for both the Unix/macOS/Android matrix and this separate Windows matrix.

The build injects the release version, tagged commit SHA, and UTC build time.
`bwkp version` also preserves the pinned upstream Bitwarden SDK and KeePassXC
version output, which is part of dependency-upgrade review.

All builds omit linker symbols, debug tables, local source paths, and build
identifiers. Linux and Android artifacts are additionally packed with the
pinned, checksum-verified UPX release and tested by UPX before packaging. macOS
Mach-O executables are stripped but remain unpacked because UPX does not support
that format. Windows artifacts are also left unpacked. Linux and Android release
runners install UPX through `build/install-upx.sh`, whose host-specific archives
and SHA-256 digests pin the compression tool as part of the release input.

Every archive receives a GitHub build-provenance attestation. After all target
jobs finish, the checksum job downloads the archives and uploads `SHA256SUMS`.
Users can therefore verify both the archive digest and its association with the
GitHub Actions build.

That checksum job also writes the Intel and ARM64 macOS archive digests into
`Casks/bwkp.rb` and commits the cask update to `master`. Release Please has
already updated the cask version in the release pull request. Until the
first artifact release completes, the all-zero bootstrap digests intentionally
make cask installation fail closed; they are never valid release checksums.
The cask selects `macos-arm64` or `macos-amd64` from Homebrew's detected
architecture. It has no formula runtime dependencies.
`build/update-homebrew-cask.sh` validates both digests before editing the cask,
and the release job skips its bot commit when the generated file is unchanged.
The checksum job therefore needs permission to push its cask-only commit to
`master`; branch protection must allow that automation path.

## Static linking and compatibility

Release binaries statically link the project-owned Rust bridge, official
Bitwarden SDK code, the KDBX-only KeePassXC core, reduced QtCore/QtConcurrent,
Botan, Argon2, zlib, and non-system compiler runtimes. The removed KeePassXC
GUI path is why this does not embed Qt Widgets, GUI, Network, DBus, SVG,
minizip, or QRencode.

Linux and container executables are fully static ELF files: they have neither
an ELF interpreter nor `DT_NEEDED` entries. Other platforms retain the ABI
which their operating system requires. macOS uses only Apple frameworks and
`/usr/lib`; Windows imports only system DLLs; Android uses only Bionic/system
libraries. Those platform references are not third-party runtime dependencies.

Mage validates linkage before UPX runs. Linux uses `readelf`, macOS uses
`otool`, Windows uses `objdump`, and Android applies an explicit Bionic allow
list. This order is important because `file` can describe a UPX-packed dynamic
ELF as static and therefore is not sufficient release verification.

## Failure and recovery

- If one matrix job fails, fix the tagged source only through a new patch
  release; do not replace a successfully published archive with an unrelated
  local build. A transient infrastructure failure may be re-run in Actions.
- If checksums fail after all archives uploaded, re-run the checksum job and
  verify that it includes exactly the release's `bwkp_*.tar.gz` assets.
- If a release contains a security or data-integrity defect, describe impact
  without exposing secrets, publish a corrected release promptly, and mark the
  affected release clearly. Do not delete history solely to hide a bad release.
- If manifest, tag, and GitHub release versions diverge, stop automation and
  reconcile all three deliberately before merging more release commits.
