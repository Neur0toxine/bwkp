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

func TestCommandHelpAliasesListOptionsAndExamples(t *testing.T) {
	commands := []struct {
		name     string
		required []string
	}{
		{name: "export", required: []string{"output", "cipher", "kdf-target", "allow-lossy"}},
		{name: "import", required: []string{"input", "conflict", "append-source", "allow-lossy"}},
	}
	for _, command := range commands {
		for _, alias := range []string{"--help", "-help", "-h"} {
			t.Run(command.name+alias, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				if err := New(&stdout, &stderr).Run(t.Context(), []string{command.name, alias}); err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				output := stderr.String()
				for _, expected := range append(command.required, "Usage:", "Options:", "Examples:") {
					if !strings.Contains(output, expected) {
						t.Errorf("help does not contain %q: %q", expected, output)
					}
				}
			})
		}
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
