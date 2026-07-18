package main

import (
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- RFC 6238 interoperability requires HMAC-SHA-1.
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: totp SECRET")
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(os.Args[1]))
	if err != nil {
		panic(err)
	}
	message := make([]byte, 8)
	counter := time.Now().Unix() / 30
	if counter < 0 {
		panic("system time predates the Unix epoch")
	}
	// #nosec G115 -- the negative case is rejected above.
	binary.BigEndian.PutUint64(message, uint64(counter))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(message)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	fmt.Printf("%06d\n", value%1_000_000)
	clear(secret)
}
