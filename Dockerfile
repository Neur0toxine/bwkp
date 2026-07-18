# syntax=docker/dockerfile:1.7
FROM docker.io/library/rust:1.93.1-trixie AS build
ARG GO_VERSION=1.26.5
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
       binutils ca-certificates cmake curl file g++ gcc git libc6-dev make pkg-config python3 \
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

FROM scratch AS runtime
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /src/dist/bwkp /bwkp
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/bwkp"]
