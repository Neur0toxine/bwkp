// Package kpdb defines KDBX credentials, options, and writer contracts.
package kpdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
)

type Cipher string
type KDF string
type Compression string

const (
	CipherAES256    Cipher      = "aes256"
	CipherChaCha20  Cipher      = "chacha20"
	KDFArgon2id     KDF         = "argon2id"
	CompressionGZip Compression = "gzip"
	CompressionNone Compression = "none"
)

type Credentials struct {
	Password []byte
	KeyFile  []byte
}

type Options struct {
	Cipher      Cipher
	KDF         KDF
	Compression Compression
	MemoryKiB   uint64
	Iterations  uint64
	Parallelism uint32
	TargetTime  time.Duration
}

func DefaultOptions() Options {
	return Options{
		Cipher: CipherAES256, KDF: KDFArgon2id, Compression: CompressionGZip,
		MemoryKiB: 64 * 1024, Parallelism: 4, TargetTime: time.Second,
	}
}

func (o Options) Validate(credentials Credentials) error {
	var problems []error
	if len(credentials.Password) == 0 && len(credentials.KeyFile) == 0 {
		problems = append(problems, errors.New("a password or key file is required"))
	}
	if o.Cipher != CipherAES256 && o.Cipher != CipherChaCha20 {
		problems = append(problems, fmt.Errorf("unsupported cipher %q", o.Cipher))
	}
	if o.KDF != KDFArgon2id {
		problems = append(problems, fmt.Errorf("unsupported KDF %q", o.KDF))
	}
	if o.Compression != CompressionGZip && o.Compression != CompressionNone {
		problems = append(problems, fmt.Errorf("unsupported compression %q", o.Compression))
	}
	if o.MemoryKiB < 8*1024 {
		problems = append(problems, errors.New("Argon2 memory must be at least 8192 KiB"))
	}
	if o.Parallelism == 0 {
		problems = append(problems, errors.New("Argon2 parallelism must be positive"))
	}
	if o.Iterations == 0 && o.TargetTime <= 0 {
		problems = append(problems, errors.New("iterations or target time must be positive"))
	}
	if o.Iterations > 0 && o.TargetTime > 0 {
		problems = append(problems, errors.New("iterations and target time are mutually exclusive"))
	}
	return errors.Join(problems...)
}

type Writer interface {
	WriteFile(context.Context, string, kp.Database, Credentials, Options, bool) error
}
