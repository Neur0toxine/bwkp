package kpdb_test

import (
	"testing"

	"github.com/Neur0toxine/bwkp/pkg/kpdb"
)

func TestOptionsValidation(t *testing.T) {
	options := kpdb.DefaultOptions()
	if err := options.Validate(kpdb.Credentials{Password: []byte("password")}); err != nil {
		t.Fatal(err)
	}
	if err := options.Validate(kpdb.Credentials{}); err == nil {
		t.Fatal("expected missing credential error")
	}
	options.Iterations = 10
	if err := options.Validate(kpdb.Credentials{Password: []byte("password")}); err == nil {
		t.Fatal("expected calibration conflict")
	}
}
