//go:build !native

package kpdb_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Neur0toxine/bwkp/internal/native"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
	"github.com/Neur0toxine/bwkp/pkg/kpdb"
)

func TestNativeWriterReportsUnavailableAndCleansCandidate(t *testing.T) {
	options := kpdb.DefaultOptions()
	options.Iterations, options.TargetTime = 10, 0
	err := kpdb.NewNativeWriter().WriteFile(t.Context(), filepath.Join(t.TempDir(), "test.kdbx"), kp.Database{Root: kp.Group{Name: "Root"}}, kpdb.Credentials{Password: []byte("password")}, options, false)
	if !errors.Is(err, native.ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}
