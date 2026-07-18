// Package native owns the stable Go side of the Rust and KeePassXC C ABIs.
package native

import "errors"

var ErrUnavailable = errors.New("native support is unavailable; build with Mage or the native build tag")

type Handle uintptr

type LoginResult struct {
	Handle    Handle
	Challenge []byte
}

func Login(request []byte) (LoginResult, error) { return login(request) }
func Sync(handle Handle) ([]byte, error)        { return syncVault(handle) }
func DownloadAttachment(handle Handle, request []byte) ([]byte, error) {
	return downloadAttachment(handle, request)
}
func Close(handle Handle) error { return closeHandle(handle) }
func WriteKDBX(path string, database, credentials, options []byte) error {
	return writeKDBX(path, database, credentials, options)
}
func VerifyKDBX(path string, credentials []byte) error { return verifyKDBX(path, credentials) }
func KeePassXCVersion() string                         { return keepassXCVersion() }
func BitwardenSDKVersion() string                      { return bitwardenSDKVersion() }
