package progress_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Neur0toxine/bwkp/internal/app"
	"github.com/Neur0toxine/bwkp/internal/progress"
)

func TestRendererUsesBlockTheme(t *testing.T) {
	var output bytes.Buffer
	renderer := progress.New(&output, true)
	renderer.Update(app.ProgressUpdate{
		Stage: 1, Stages: 3, Description: "Downloading vault...", Completed: 1, Total: 2,
	})
	renderer.Update(app.ProgressUpdate{
		Stage: 1, Stages: 3, Description: "Downloading vault...", Completed: 2, Total: 2,
	})
	renderer.Update(app.ProgressUpdate{
		Stage: 2, Stages: 3, Description: "Converting entries...", Completed: 1, Total: 1,
	})
	renderer.Close()

	text := output.String()
	for _, expected := range []string{"[1/3]", "Downloading vault...", "[2/3]", "Converting entries...", "█"} {
		if !strings.Contains(text, expected) {
			t.Errorf("output does not contain %q: %q", expected, text)
		}
	}
}

func TestRendererUsesSpinnerForIndeterminateWork(t *testing.T) {
	var output bytes.Buffer
	renderer := progress.New(&output, true)
	renderer.Update(app.ProgressUpdate{
		Stage: 3, Stages: 3, Description: "Writing encrypted database...", Indeterminate: true,
	})

	text := output.String()
	if !strings.Contains(text, "[3/3]") || !strings.Contains(text, "Writing encrypted database...") {
		t.Fatalf("spinner output = %q", text)
	}
	if strings.Contains(text, "%") {
		t.Fatalf("indeterminate output contains a percentage: %q", text)
	}

	renderer.Update(app.ProgressUpdate{
		Stage: 3, Stages: 3, Description: "Writing encrypted database...", Completed: 1, Total: 1,
	})
	renderer.Close()
}

func TestDisabledRendererWritesNothing(t *testing.T) {
	var output bytes.Buffer
	renderer := progress.New(&output, false)
	renderer.Update(app.ProgressUpdate{Stage: 1, Stages: 2, Description: "Downloading", Total: 1})
	renderer.Close()
	if output.Len() != 0 {
		t.Fatalf("disabled output = %q", output.String())
	}
}
