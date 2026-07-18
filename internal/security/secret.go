package security

import "strings"

func Clear(secret []byte) { clear(secret) }

func Redact(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "[redacted]"
	}
	return value[:2] + strings.Repeat("*", min(8, len(value)-4)) + value[len(value)-2:]
}
