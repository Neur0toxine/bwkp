package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretAndCodeFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("123456\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret, err := Secret("test", path, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "123456" {
		t.Fatalf("secret = %q", secret)
	}
	code, err := Code("test", path)
	if err != nil || code != "123456" {
		t.Fatalf("code=%q error=%v", code, err)
	}
	if _, err := Secret("test", path+"-missing", false); err == nil {
		t.Fatal("missing file accepted")
	}
}
