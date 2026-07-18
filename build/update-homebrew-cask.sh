#!/bin/sh
set -eu

if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
	echo "usage: $0 VERSION ARM64_SHA256 AMD64_SHA256 [CASK]" >&2
	exit 2
fi

version=$1
arm64_sha256=$2
amd64_sha256=$3
cask=${4:-Casks/bwkp.rb}

case "$version" in
	''|*[!0-9A-Za-z.+-]*)
		echo "invalid release version: $version" >&2
		exit 2
		;;
esac

validate_sha256() {
	case "$1" in
		*[!0-9a-f]*|'') return 1 ;;
	esac
	[ "${#1}" -eq 64 ]
}

if ! validate_sha256 "$arm64_sha256" || ! validate_sha256 "$amd64_sha256"; then
	echo "Homebrew archive checksums must be 64 lowercase hexadecimal characters" >&2
	exit 2
fi

sed -E -i \
	-e "s|^  version \"[^\"]+\" # x-release-please-version$|  version \"$version\" # x-release-please-version|" \
	-e "s|^  sha256 arm:          \"[0-9a-f]+\", # bwkp-release-arm64$|  sha256 arm:          \"$arm64_sha256\", # bwkp-release-arm64|" \
	-e "s|^         arm64_linux:  \"[0-9a-f]+\",$|         arm64_linux:  \"$arm64_sha256\",|" \
	-e "s|^         x86_64:       \"[0-9a-f]+\", # bwkp-release-amd64$|         x86_64:       \"$amd64_sha256\", # bwkp-release-amd64|" \
	-e "s|^         x86_64_linux: \"[0-9a-f]+\"$|         x86_64_linux: \"$amd64_sha256\"|" \
	"$cask"

grep -F "version \"$version\" # x-release-please-version" "$cask" >/dev/null
grep -F "sha256 arm:          \"$arm64_sha256\", # bwkp-release-arm64" "$cask" >/dev/null
grep -F "arm64_linux:  \"$arm64_sha256\"," "$cask" >/dev/null
grep -F "x86_64:       \"$amd64_sha256\", # bwkp-release-amd64" "$cask" >/dev/null
grep -F "x86_64_linux: \"$amd64_sha256\"" "$cask" >/dev/null
