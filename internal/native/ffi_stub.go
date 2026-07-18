//go:build !native || !cgo

package native

func login([]byte) (LoginResult, error)                 { return LoginResult{}, ErrUnavailable }
func syncVault(Handle) ([]byte, error)                  { return nil, ErrUnavailable }
func downloadAttachment(Handle, []byte) ([]byte, error) { return nil, ErrUnavailable }
func closeHandle(Handle) error                          { return nil }
func writeKDBX(string, []byte, []byte, []byte) error    { return ErrUnavailable }
func verifyKDBX(string, []byte) error                   { return ErrUnavailable }
func keepassXCVersion() string                          { return "unavailable" }
func bitwardenSDKVersion() string                       { return "unavailable" }
