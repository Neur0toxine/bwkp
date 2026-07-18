# syntax=docker/dockerfile:1.7
FROM docker.io/library/rust:1.93.1-trixie AS build
ARG GO_VERSION=1.26.5
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
       ca-certificates cmake curl g++ gcc git libc6-dev pkg-config \
       qtbase5-dev qttools5-dev libqt5svg5-dev \
       libbotan-3-dev libargon2-dev libminizip-dev libqrencode-dev \
    && rm -rf /var/lib/apt/lists/* \
    && curl --fail --silent --show-error --location "https://go.dev/dl/go${GO_VERSION}.linux-$(dpkg --print-architecture | sed 's/amd64/amd64/;s/arm64/arm64/').tar.gz" -o /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV PATH=/usr/local/go/bin:$PATH
WORKDIR /src
COPY . .
RUN ./build/install-upx.sh /usr/local/bin \
    && VERSION="$VERSION" COMMIT="$COMMIT" BUILD_DATE="$BUILD_DATE" go tool mage build

FROM scratch AS artifact
COPY --from=build /src/dist/bwkp /bwkp

FROM docker.io/library/debian:trixie-slim AS runtime
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
       ca-certificates libqt5concurrent5t64 libqt5dbus5t64 libqt5network5t64 libqt5svg5 libqt5widgets5t64 \
       libbotan-3-7 libargon2-1 libminizip1t64 libqrencode4 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /src/dist/bwkp /usr/local/bin/bwkp
ENTRYPOINT ["/usr/local/bin/bwkp"]
