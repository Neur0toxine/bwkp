package security

import "testing"

func TestClearAndRedact(t *testing.T) {
	secret := []byte("sensitive")
	Clear(secret)
	for _, value := range secret {
		if value != 0 {
			t.Fatal("secret was not cleared")
		}
	}
	if got := Redact("secret@example.test"); got == "secret@example.test" {
		t.Fatal("value not redacted")
	}
}
