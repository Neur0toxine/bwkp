# Release lifecycle

`bwkp` uses release-please to turn Conventional Commits on `main` into a
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

1. Confirm CI is green on `main` and run `go tool mage verify` locally.
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

On each push to `main`, `.github/workflows/release-please.yml` runs
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
Conventional commits merged to main
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
| Termux Android ARM64 | `android-arm64` | pinned Termux cross-builder |
| Termux Android ARMv7 | `android-armv7` | pinned Termux cross-builder |

These cover the popular desktop/server 64-bit architectures and both current
64-bit ARM and legacy 32-bit ARM Termux devices. Adding a supported platform is
not complete until its Mage target, CI build, release matrix entry, runtime
dependency documentation, and a real artifact-format check all exist.

Each file is named `bwkp_vX.Y.Z_TARGET.tar.gz` and contains:

- one executable named `bwkp`;
- `README.md`;
- `LICENSE.md`.

The build injects the release version, tagged commit SHA, and UTC build time.
`bwkp version` also preserves the pinned upstream Bitwarden SDK and KeePassXC
version output, which is part of dependency-upgrade review.

All builds omit linker symbols, debug tables, local source paths, and build
identifiers. Linux and Android artifacts are additionally packed with the
pinned, checksum-verified UPX release and tested by UPX before packaging. macOS
Mach-O executables are stripped but remain unpacked because UPX does not support
that format.

Every archive receives a GitHub build-provenance attestation. After all target
jobs finish, the checksum job downloads the archives and uploads `SHA256SUMS`.
Users can therefore verify both the archive digest and its association with the
GitHub Actions build.

## Static linking and compatibility

Release binaries statically link the project-owned Rust bridge, official
Bitwarden SDK code, KeePassXC core, and the small C++ bridge. Keeping those
version-sensitive components inside the executable avoids asking users to find
matching SDK or KeePassXC development packages and makes each artifact behave
consistently across supported machines.

The executables are intentionally not completely static. They dynamically use
platform libraries including Qt, Botan, Argon2, minizip, qrencode, zlib, and
the OS C/C++ runtime. Linux/container and Termux documentation must list those
runtime packages, and release builders should target conservative platform
baselines. A claim that a release is a “fully static binary” would be
incorrect; the support benefit comes from statically embedding the two pinned
application-level native dependencies while retaining maintained system
libraries.

When changing link behavior, inspect release candidates with `file`,
`readelf -d` on ELF systems, or `otool -L` on macOS. Confirm that every dynamic
dependency is available on the documented target before publishing.

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
