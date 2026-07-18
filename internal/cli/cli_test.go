package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Neur0toxine/bwkp/pkg/convert"
)

func TestExportAcceptsAllowLossyFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := New(&stdout, &stderr).Run(t.Context(), []string{"export", "--allow-lossy"})
	if err == nil || err.Error() != "--email is required" {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestImportAcceptsConflictAndConversionFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := New(&stdout, &stderr).Run(t.Context(), []string{"import", "--conflict", "update", "--append-source", "--allow-lossy"})
	if err == nil || err.Error() != "--email is required" {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWriteWarnings(t *testing.T) {
	var output bytes.Buffer
	err := writeWarnings(&output, []convert.Warning{{
		ItemID: "item-id", ItemName: "Future item", Message: `unsupported Bitwarden item: type "future"`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "Warning: skipped item \"Future item\" (item-id): unsupported Bitwarden item: type \"future\"\n"
	if output.String() != want {
		t.Fatalf("warning = %q, want %q", output.String(), want)
	}
}
