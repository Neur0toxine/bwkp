package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrTargetExists = errors.New("target already exists")

// Write writes an encrypted candidate beside target, verifies it, then installs it atomically.
func Write(target string, force bool, write func(string) error, verify func(string) error) (err error) {
	directory := filepath.Dir(target)
	if info, statErr := os.Stat(target); statErr == nil {
		if info.IsDir() {
			return fmt.Errorf("target is a directory: %s", target)
		}
		if !force {
			return ErrTargetExists
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	candidate, err := os.CreateTemp(directory, ".bwkp-*.kdbx")
	if err != nil {
		return fmt.Errorf("create encrypted candidate: %w", err)
	}
	candidatePath := candidate.Name()
	if closeErr := candidate.Close(); closeErr != nil {
		return closeErr
	}
	defer func() { err = errors.Join(err, removeIfPresent(candidatePath)) }()
	if err := os.Chmod(candidatePath, 0o600); err != nil {
		return fmt.Errorf("secure candidate: %w", err)
	}
	if err := write(candidatePath); err != nil {
		return fmt.Errorf("write encrypted candidate: %w", err)
	}
	if err := verify(candidatePath); err != nil {
		return fmt.Errorf("verify encrypted candidate: %w", err)
	}
	file, err := os.OpenFile(candidatePath, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	if syncErr := file.Sync(); syncErr != nil {
		_ = file.Close()
		return syncErr
	}
	if closeErr := file.Close(); closeErr != nil {
		return closeErr
	}
	if err := replace(candidatePath, target, force); err != nil {
		return fmt.Errorf("install target: %w", err)
	}
	candidatePath = ""
	if directoryFile, openErr := os.Open(directory); openErr == nil {
		defer func() { _ = directoryFile.Close() }()
		_ = directoryFile.Sync()
	}
	return nil
}

func removeIfPresent(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func replace(source, target string, force bool) error {
	if !force {
		return os.Rename(source, target)
	}
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, target)
}
