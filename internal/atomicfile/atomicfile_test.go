package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCommitsVerifiedCandidate(t *testing.T) {
	target := filepath.Join(t.TempDir(), "vault.kdbx")
	err := Write(target, false,
		func(path string) error { return os.WriteFile(path, []byte("encrypted"), 0o600) },
		func(path string) error { _, err := os.ReadFile(path); return err },
	)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "encrypted" {
		t.Fatalf("content = %q", content)
	}
	if err := Write(target, false, func(string) error { return nil }, func(string) error { return nil }); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteRemovesFailedCandidate(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "vault.kdbx")
	errExpected := errors.New("broken")
	err := Write(target, false, func(string) error { return errExpected }, func(string) error { return nil })
	if !errors.Is(err, errExpected) {
		t.Fatalf("error = %v", err)
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("leftover files: %v", files)
	}
}

func TestWriteDoesNotCommitFailedVerification(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "vault.kdbx")
	errExpected := errors.New("invalid database")
	err := Write(target, false,
		func(path string) error { return os.WriteFile(path, []byte("bad"), 0o600) },
		func(string) error { return errExpected },
	)
	if !errors.Is(err, errExpected) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target exists: %v", err)
	}
}

func TestWriteForceReplacesExistingFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "vault.kdbx")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Write(target, true,
		func(path string) error { return os.WriteFile(path, []byte("new"), 0o600) },
		func(string) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "new" {
		t.Fatalf("content=%q error=%v", content, err)
	}
}
