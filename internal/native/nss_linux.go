//go:build native && cgo && linux && !android

package native

/*
#include <features.h>
#ifdef __GLIBC__
#include <nss.h>
#endif

static int bwkp_configure_static_nss(void) {
#ifdef __GLIBC__
    return __nss_configure_lookup("hosts", "files dns");
#else
    return 0;
#endif
}
*/
import "C"

func init() {
	// A static glibc process cannot safely load NSS modules compiled for the
	// host's potentially different glibc. The files and DNS backends are part
	// of libc itself; select that portable subset before the SDK resolves a
	// server name.
	C.bwkp_configure_static_nss()
}
