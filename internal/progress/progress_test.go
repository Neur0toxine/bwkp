package progress_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Neur0toxine/bitwarden-keepass-exporter/internal/app"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/internal/progress"
)

func TestRendererUsesTwoStageBlockTheme(t *testing.T) {
	var output bytes.Buffer
	renderer := progress.New(&output, true)
	renderer.Update(app.ProgressUpdate{
		Stage: 1, Stages: 2, Description: "Downloading vault...", Completed: 1, Total: 2,
	})
	renderer.Update(app.ProgressUpdate{
		Stage: 1, Stages: 2, Description: "Downloading vault...", Completed: 2, Total: 2,
	})
	renderer.Update(app.ProgressUpdate{
		Stage: 2, Stages: 2, Description: "Converting entries...", Completed: 1, Total: 1,
	})
	renderer.Close()

	text := output.String()
	for _, expected := range []string{"[1/2]", "Downloading vault...", "[2/2]", "Converting entries...", "█"} {
		if !strings.Contains(text, expected) {
			t.Errorf("output does not contain %q: %q", expected, text)
		}
	}
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
