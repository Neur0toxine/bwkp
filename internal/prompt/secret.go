package prompt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

func Secret(label, path string, confirm bool) ([]byte, error) {
	if path != "" {
		value, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s file: %w", label, err)
		}
		return trimLineEnd(value), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, fmt.Errorf("%s requires an interactive terminal or a corresponding file option", label)
	}
	first, err := readTerminal(label + ": ")
	if err != nil {
		return nil, err
	}
	if !confirm {
		return first, nil
	}
	second, err := readTerminal("Confirm " + label + ": ")
	if err != nil {
		clear(first)
		return nil, err
	}
	defer clear(second)
	if !bytes.Equal(first, second) {
		clear(first)
		return nil, errors.New("database passwords do not match")
	}
	return first, nil
}

func readTerminal(label string) ([]byte, error) {
	if _, err := fmt.Fprint(os.Stderr, label); err != nil {
		return nil, err
	}
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	_, newlineErr := fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	if newlineErr != nil {
		clear(value)
		return nil, newlineErr
	}
	return value, nil
}

func trimLineEnd(value []byte) []byte {
	value = bytes.TrimSuffix(value, []byte("\n"))
	value = bytes.TrimSuffix(value, []byte("\r"))
	return value
}

func Code(label, path string) (string, error) {
	secret, err := Secret(label, path, false)
	if err != nil {
		return "", err
	}
	defer clear(secret)
	if len(secret) == 0 {
		return "", io.ErrUnexpectedEOF
	}
	return string(secret), nil
}
