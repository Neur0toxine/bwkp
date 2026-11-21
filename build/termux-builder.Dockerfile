FROM ghcr.io/termux/package-builder@sha256:374fedda8d2ce7a8ab499735d39329301c4f2f18ea4411b3cf7c93d4668768ab

# Docker initializes the build-cache volume from this directory.
RUN install -d -m 1777 /home/builder/.termux-build
