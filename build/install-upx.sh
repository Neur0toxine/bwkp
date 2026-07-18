#!/bin/sh
set -eu

version=5.1.1
destination=${1:-./bin}

case "$(uname -m)" in
	x86_64|amd64)
		architecture=amd64
		sha256=1ff660454227861e00772f743f66b900072116b9dc24f6ee28b97cce88a7828a
		;;
	aarch64|arm64)
		architecture=arm64
		sha256=a307c2c821eeab47607ba5c232408b22ab884cca13884682508b98f7308b8443
		;;
	*)
		echo "unsupported UPX host architecture: $(uname -m)" >&2
		exit 1
		;;
esac

archive="upx-${version}-${architecture}_linux.tar.xz"
url="https://github.com/upx/upx/releases/download/v${version}/${archive}"
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

curl --fail --silent --show-error --location "$url" --output "$temporary/$archive"
printf '%s  %s\n' "$sha256" "$temporary/$archive" | sha256sum --check --status
tar -C "$temporary" -xJf "$temporary/$archive"
mkdir -p "$destination"
install -m 0755 "$temporary/upx-${version}-${architecture}_linux/upx" "$destination/upx"
