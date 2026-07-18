TERMUX_PKG_HOMEPAGE=https://github.com/Neur0toxine/bwkp
TERMUX_PKG_DESCRIPTION="Transfer records between Bitwarden or Vaultwarden and KeePassXC"
TERMUX_PKG_LICENSE="GPL-3.0-only"
TERMUX_PKG_MAINTAINER="Neur0toxine"
TERMUX_PKG_VERSION=${VERSION:-0.1.0}
TERMUX_PKG_SRCURL=file:///home/builder/termux-packages/sources/bwkp
TERMUX_PKG_SHA256=SKIP_CHECKSUM
TERMUX_PKG_BUILD_DEPENDS="qt5-qtbase, qt5-qtbase-cross-tools"
TERMUX_PKG_FORCE_CMAKE=true

termux_step_pre_configure() {
	BWKP_SOURCE_ROOT="$TERMUX_PKG_SRCDIR"
	TERMUX_PKG_SRCDIR="$BWKP_SOURCE_ROOT/native/kpdb"
	termux_setup_golang
	TERMUX_RUST_VERSION=1.93.1
	termux_setup_rust

	export BWKP_STATIC_PREFIX="$BWKP_SOURCE_ROOT/target/static"
	"$BWKP_SOURCE_ROOT/build/static-dependencies.sh"
	export PKG_CONFIG_PATH="$BWKP_STATIC_PREFIX/lib/pkgconfig:$BWKP_STATIC_PREFIX/share/pkgconfig"
	export CGO_LDFLAGS="-L$BWKP_STATIC_PREFIX/lib -lqtpcre2 -lz -lc++_static -lc++abi -lunwind -latomic -llog -ldl -lm"
	TERMUX_PKG_EXTRA_CONFIGURE_ARGS+=" -DCMAKE_PREFIX_PATH=$BWKP_STATIC_PREFIX -DCMAKE_FIND_LIBRARY_SUFFIXES=.a"
}

termux_step_make() {
	cmake --build "$TERMUX_PKG_BUILDDIR" --target bwkp_kpdb --parallel "$TERMUX_PKG_MAKE_PROCESSES"

	cd "$BWKP_SOURCE_ROOT"
	cargo build --locked --release --package bwkp-native --target "$CARGO_TARGET_NAME" --jobs "$TERMUX_PKG_MAKE_PROCESSES"
	mkdir -p target/release target/keepassxc/lib
	cp "target/$CARGO_TARGET_NAME/release/libbwkp_native.a" target/release/
	cp "$TERMUX_PKG_BUILDDIR"/lib/*.a target/keepassxc/lib/

	local goarch goarm
	case "$TERMUX_ARCH" in
		aarch64) goarch=arm64; goarm= ;;
		arm) goarch=arm; goarm=7 ;;
		*) termux_error_exit "unsupported bwkp Android architecture: $TERMUX_ARCH" ;;
	esac
	CGO_ENABLED=1 GOOS=android GOARCH="$goarch" GOARM="$goarm" \
		go build -a -trimpath -tags native \
		-ldflags "-s -w -buildid= -extldflags=-Wl,--gc-sections,--build-id=none -X github.com/Neur0toxine/bwkp/internal/buildinfo.Version=${VERSION:-dev} -X github.com/Neur0toxine/bwkp/internal/buildinfo.Commit=${COMMIT:-unknown} -X github.com/Neur0toxine/bwkp/internal/buildinfo.Date=${BUILD_DATE:-unknown}" \
		-o "$TERMUX_PREFIX/bin/bwkp" ./cmd/bwkp
}

termux_step_make_install() {
	:
}
