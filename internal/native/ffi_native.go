//go:build native && cgo

package native

/*
#cgo linux,!android pkg-config: Qt5Core Qt5Concurrent
#cgo linux,!android LDFLAGS: ${SRCDIR}/../../target/release/libbwkp_native.a -L${SRCDIR}/../../target/keepassxc/lib -lbwkp_kpdb -lkeepassx_core -lbwkp_botan -largon2 -lz -lstdc++ -lrt -ldl -lm -lpthread
#cgo android pkg-config: Qt5Core Qt5Concurrent
#cgo android LDFLAGS: ${SRCDIR}/../../target/release/libbwkp_native.a -L${SRCDIR}/../../target/keepassxc/lib -lbwkp_kpdb -lkeepassx_core -lbotan-3 -largon2 -lz -lc++_static -lc++abi -lunwind -latomic -llog -ldl -lm
#cgo darwin pkg-config: Qt5Core Qt5Concurrent
#cgo darwin LDFLAGS: ${SRCDIR}/../../target/release/libbwkp_native.a -L${SRCDIR}/../../target/keepassxc/lib -lbwkp_kpdb -lkeepassx_core -lbotan-3 -largon2 -lz -lc++ -framework Security -framework CoreFoundation -framework Foundation -liconv
#cgo windows LDFLAGS: ${SRCDIR}/../../target/release/libbwkp_native.a -L${SRCDIR}/../../target/keepassxc/lib -lbwkp_kpdb -lkeepassx_core -lQt5Core -lQt5Concurrent -lbotan-3 -largon2 -lz -lbcrypt -lws2_32 -luserenv -lntdll
#cgo windows,386 LDFLAGS: -lstdc++
#cgo windows,amd64 LDFLAGS: -lstdc++
#cgo windows,arm64 LDFLAGS: -lc++
#include <stdint.h>
#include <stddef.h>

typedef struct {
    uint8_t *ptr;
    size_t len;
} bwkp_buffer;

typedef struct {
    uint8_t *ptr;
    size_t len;
} bwkp_kpdb_buffer;

void bwkp_buffer_free(bwkp_buffer buffer);
const char *bwkp_bitwarden_sdk_version(void);
uintptr_t bwkp_login(const uint8_t *request_ptr, size_t request_len, bwkp_buffer *output, bwkp_buffer *error);
int32_t bwkp_sync(uintptr_t handle, bwkp_buffer *output, bwkp_buffer *error);
int32_t bwkp_download_attachment(uintptr_t handle, const uint8_t *request_ptr, size_t request_len, bwkp_buffer *output, bwkp_buffer *error);
int32_t bwkp_mutate(uintptr_t handle, const uint8_t *request_ptr, size_t request_len, bwkp_buffer *output, bwkp_buffer *error);
int32_t bwkp_upload_attachment(uintptr_t handle, const uint8_t *request_ptr, size_t request_len, const uint8_t *content_ptr, size_t content_len, bwkp_buffer *output, bwkp_buffer *error);
void bwkp_session_close(uintptr_t handle);
int32_t bwkp_kpdb_write(const char *path_ptr, size_t path_len, const uint8_t *database_ptr, size_t database_len, const uint8_t *credentials_ptr, size_t credentials_len, const uint8_t *options_ptr, size_t options_len, bwkp_kpdb_buffer *error);
int32_t bwkp_kpdb_verify(const char *path_ptr, size_t path_len, const uint8_t *credentials_ptr, size_t credentials_len, bwkp_kpdb_buffer *error);
int32_t bwkp_kpdb_read(const char *path_ptr, size_t path_len, const uint8_t *credentials_ptr, size_t credentials_len, bwkp_kpdb_buffer *output, bwkp_kpdb_buffer *error);
const char *bwkp_keepassxc_version(void);
void bwkp_kpdb_buffer_free(bwkp_kpdb_buffer buffer);
*/
import "C"

import (
	"errors"
	"unsafe"
)

func login(request []byte) (LoginResult, error) {
	var output, nativeError C.bwkp_buffer
	handle := C.bwkp_login(bytePointer(request), C.size_t(len(request)), &output, &nativeError)
	if err := takeError(nativeError); err != nil {
		freeBuffer(output)
		return LoginResult{}, err
	}
	return LoginResult{Handle: Handle(handle), Challenge: takeBuffer(output)}, nil
}

func syncVault(handle Handle) ([]byte, error) {
	var output, nativeError C.bwkp_buffer
	code := C.bwkp_sync(C.uintptr_t(handle), &output, &nativeError)
	if err := resultError(code, nativeError); err != nil {
		freeBuffer(output)
		return nil, err
	}
	return takeBuffer(output), nil
}

func downloadAttachment(handle Handle, request []byte) ([]byte, error) {
	var output, nativeError C.bwkp_buffer
	code := C.bwkp_download_attachment(C.uintptr_t(handle), bytePointer(request), C.size_t(len(request)), &output, &nativeError)
	if err := resultError(code, nativeError); err != nil {
		freeBuffer(output)
		return nil, err
	}
	return takeBuffer(output), nil
}

func mutate(handle Handle, request []byte) ([]byte, error) {
	var output, nativeError C.bwkp_buffer
	code := C.bwkp_mutate(C.uintptr_t(handle), bytePointer(request), C.size_t(len(request)), &output, &nativeError)
	if err := resultError(code, nativeError); err != nil {
		freeBuffer(output)
		return nil, err
	}
	return takeBuffer(output), nil
}

func uploadAttachment(handle Handle, request, content []byte) ([]byte, error) {
	var output, nativeError C.bwkp_buffer
	code := C.bwkp_upload_attachment(
		C.uintptr_t(handle), bytePointer(request), C.size_t(len(request)),
		bytePointer(content), C.size_t(len(content)), &output, &nativeError,
	)
	if err := resultError(code, nativeError); err != nil {
		freeBuffer(output)
		return nil, err
	}
	return takeBuffer(output), nil
}

func closeHandle(handle Handle) error {
	C.bwkp_session_close(C.uintptr_t(handle))
	return nil
}

func writeKDBX(path string, database, credentials, options []byte) error {
	var nativeError C.bwkp_kpdb_buffer
	pathBytes := []byte(path)
	code := C.bwkp_kpdb_write(
		(*C.char)(unsafe.Pointer(bytePointer(pathBytes))), C.size_t(len(pathBytes)),
		bytePointer(database), C.size_t(len(database)),
		bytePointer(credentials), C.size_t(len(credentials)),
		bytePointer(options), C.size_t(len(options)), &nativeError,
	)
	return kpdbResultError(code, nativeError)
}

func verifyKDBX(path string, credentials []byte) error {
	var nativeError C.bwkp_kpdb_buffer
	pathBytes := []byte(path)
	code := C.bwkp_kpdb_verify(
		(*C.char)(unsafe.Pointer(bytePointer(pathBytes))), C.size_t(len(pathBytes)),
		bytePointer(credentials), C.size_t(len(credentials)), &nativeError,
	)
	return kpdbResultError(code, nativeError)
}

func readKDBX(path string, credentials []byte) ([]byte, error) {
	var output, nativeError C.bwkp_kpdb_buffer
	pathBytes := []byte(path)
	code := C.bwkp_kpdb_read(
		(*C.char)(unsafe.Pointer(bytePointer(pathBytes))), C.size_t(len(pathBytes)),
		bytePointer(credentials), C.size_t(len(credentials)), &output, &nativeError,
	)
	if err := kpdbResultError(code, nativeError); err != nil {
		C.bwkp_kpdb_buffer_free(output)
		return nil, err
	}
	defer C.bwkp_kpdb_buffer_free(output)
	if output.ptr == nil || output.len == 0 {
		return nil, errors.New("KeePassXC reader returned no database")
	}
	return C.GoBytes(unsafe.Pointer(output.ptr), C.int(output.len)), nil
}

func keepassXCVersion() string    { return C.GoString(C.bwkp_keepassxc_version()) }
func bitwardenSDKVersion() string { return C.GoString(C.bwkp_bitwarden_sdk_version()) }

func kpdbResultError(code C.int32_t, buffer C.bwkp_kpdb_buffer) error {
	defer C.bwkp_kpdb_buffer_free(buffer)
	if code == 0 {
		return nil
	}
	if buffer.ptr != nil && buffer.len > 0 {
		return errors.New(string(C.GoBytes(unsafe.Pointer(buffer.ptr), C.int(buffer.len))))
	}
	return errors.New("KeePassXC operation failed without an error message")
}

func bytePointer(value []byte) *C.uint8_t {
	if len(value) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(&value[0]))
}

func resultError(code C.int32_t, buffer C.bwkp_buffer) error {
	if code == 0 {
		freeBuffer(buffer)
		return nil
	}
	if err := takeError(buffer); err != nil {
		return err
	}
	return errors.New("native operation failed without an error message")
}

func takeError(buffer C.bwkp_buffer) error {
	value := takeBuffer(buffer)
	if len(value) == 0 {
		return nil
	}
	return errors.New(string(value))
}

func takeBuffer(buffer C.bwkp_buffer) []byte {
	if buffer.ptr == nil || buffer.len == 0 {
		freeBuffer(buffer)
		return nil
	}
	value := C.GoBytes(unsafe.Pointer(buffer.ptr), C.int(buffer.len))
	freeBuffer(buffer)
	return value
}

func freeBuffer(buffer C.bwkp_buffer) {
	if buffer.ptr != nil {
		C.bwkp_buffer_free(buffer)
	}
}
