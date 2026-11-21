FROM ghcr.io/termux/package-builder@sha256:374fedda8d2ce7a8ab499735d39329301c4f2f18ea4411b3cf7c93d4668768ab

# The pinned Termux dependency termux-am uses these SDK components when built from source.
RUN /home/builder/lib/android-sdk-9123335/cmdline-tools/bin/sdkmanager \
    --sdk_root=/home/builder/lib/android-sdk-9123335 \
    --install "build-tools;30.0.3" "platforms;android-33"

# Docker initializes the build-cache volume from this directory.
RUN install -d -m 1777 /home/builder/.termux-build
