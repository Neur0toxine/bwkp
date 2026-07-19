#!/bin/sh
set -eu

usage() {
	cat <<EOF
Install bwkp from GitHub Releases.

Usage: $0 [-b BINDIR] [VERSION]

  -b BINDIR  Install into BINDIR (default: \$PREFIX/bin in Termux, otherwise \$HOME/.local/bin if it exists, or ./bin; \$BINDIR overrides).
  VERSION     Release version such as v1.2.3 or 1.2.3 (default: latest).
EOF
	exit 2
}

if [ -n "${BINDIR:-}" ]; then
	bindir=$BINDIR
elif case "${PREFIX:-}" in */com.termux/*) true ;; *) false ;; esac; then
	bindir=$PREFIX/bin
elif [ -n "${HOME:-}" ] && [ -d "$HOME/.local/bin" ]; then
	bindir=$HOME/.local/bin
else
	bindir=./bin
fi
while getopts "b:h" option; do
	case "$option" in
		b) bindir=$OPTARG ;;
		h) usage ;;
		*) usage ;;
	esac
done
shift $((OPTIND - 1))

if [ "$#" -gt 1 ]; then
	usage
fi

requested_version=${1:-}
repository=Neur0toxine/bwkp
release_base_url=${BWKP_RELEASE_BASE_URL:-https://github.com/$repository/releases/download}

has_command() {
	command -v "$1" >/dev/null 2>&1
}

download() {
	destination=$1
	url=$2
	if has_command curl; then
		curl --fail --silent --show-error --location "$url" --output "$destination"
	elif has_command wget; then
		wget --quiet --output-document="$destination" "$url"
	else
		echo "bwkp installer requires curl or wget" >&2
		return 1
	fi
}

download_stdout() {
	url=$1
	if has_command curl; then
		curl --fail --silent --show-error --location "$url"
	elif has_command wget; then
		wget --quiet --output-document=- "$url"
	else
		echo "bwkp installer requires curl or wget" >&2
		return 1
	fi
}

latest_tag() {
	download_stdout "https://api.github.com/repos/$repository/releases/latest" |
		sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' |
		sed -n '1p'
}

if [ -n "$requested_version" ]; then
	tag=$requested_version
	case "$tag" in
		v*) ;;
		*) tag="v$tag" ;;
	esac
else
	echo "Looking up the latest bwkp release..." >&2
	tag=$(latest_tag)
	if [ -z "$tag" ]; then
		echo "could not determine the latest bwkp release" >&2
		exit 1
	fi
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
architecture=$(uname -m)

case "$architecture" in
	x86_64|amd64) architecture=amd64 ;;
	i386|i486|i586|i686|x86) architecture=386 ;;
	aarch64|arm64) architecture=arm64 ;;
	armv7l|armv7*) architecture=armv7 ;;
esac

android=false
executable=bwkp
case "${PREFIX:-}" in
	*/com.termux/*) android=true ;;
esac
if [ "$os" = linux ] && uname -o 2>/dev/null | grep -qi android; then
	android=true
fi

if [ "$android" = true ]; then
	case "$architecture" in
		arm64|armv7) target="android-$architecture" ;;
		*)
			echo "unsupported Android architecture: $architecture" >&2
			exit 1
			;;
	esac
else
	case "$os/$architecture" in
		linux/amd64|linux/arm64) target="linux-$architecture" ;;
		darwin/amd64|darwin/arm64) target="macos-$architecture" ;;
		msys*/386|msys*/amd64|msys*/arm64|mingw*/386|mingw*/amd64|mingw*/arm64|cygwin*/386|cygwin*/amd64|cygwin*/arm64)
			target="windows-$architecture"
			executable=bwkp.exe
			;;
		*)
			echo "unsupported platform: $os/$architecture" >&2
			exit 1
			;;
	esac
fi

archive="bwkp_${tag}_${target}.tar.gz"
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

echo "Downloading $archive..." >&2
download "$temporary/$archive" "$release_base_url/$tag/$archive"
download "$temporary/SHA256SUMS" "$release_base_url/$tag/SHA256SUMS"

expected_sha256=$(awk -v archive="$archive" '$2 == archive { print $1 }' "$temporary/SHA256SUMS")
if [ -z "$expected_sha256" ]; then
	echo "$archive is missing from the release checksums" >&2
	exit 1
fi

if has_command sha256sum; then
	actual_sha256=$(sha256sum "$temporary/$archive" | awk '{ print $1 }')
elif has_command shasum; then
	actual_sha256=$(shasum -a 256 "$temporary/$archive" | awk '{ print $1 }')
else
	echo "bwkp installer requires sha256sum or shasum" >&2
	exit 1
fi

if [ "$actual_sha256" != "$expected_sha256" ]; then
	echo "checksum verification failed for $archive" >&2
	exit 1
fi

mkdir "$temporary/extracted"
tar --no-same-owner -C "$temporary/extracted" -xzf "$temporary/$archive"
if [ ! -f "$temporary/extracted/$executable" ]; then
	echo "$archive does not contain $executable" >&2
	exit 1
fi

mkdir -p "$bindir"
if has_command install; then
	install -m 0755 "$temporary/extracted/$executable" "$bindir/$executable"
else
	cp "$temporary/extracted/$executable" "$bindir/$executable"
	chmod 0755 "$bindir/$executable"
fi

if [ "$executable" = bwkp.exe ]; then
	find "$temporary/extracted" -maxdepth 1 -type f -name '*.dll' -exec cp {} "$bindir/" \;
fi

echo "Installed $bindir/$executable ($tag, $target)" >&2
