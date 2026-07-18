package progress

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/Neur0toxine/bitwarden-keepass-exporter/internal/app"
	"github.com/k0kubun/go-ansi"
	progressbar "github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

type Renderer struct {
	mu      sync.Mutex
	writer  io.Writer
	enabled bool
	bar     *progressbar.ProgressBar
	stage   int
}

func NewTerminal(writer io.Writer, requested bool) *Renderer {
	file, terminal := writer.(*os.File)
	enabled := requested && terminal && term.IsTerminal(int(file.Fd()))
	if enabled && file == os.Stderr {
		writer = ansi.NewAnsiStderr()
	}
	return New(writer, enabled)
}

func New(writer io.Writer, enabled bool) *Renderer {
	return &Renderer{writer: writer, enabled: enabled}
}

func (r *Renderer) Update(update app.ProgressUpdate) {
	if !r.enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	total := max(update.Total, 1)
	completed := min(max(update.Completed, 0), total)
	if r.bar == nil || r.stage != update.Stage {
		r.clear()
		r.stage = update.Stage
		r.bar = r.newBar(update, total)
	} else if r.bar.GetMax64() != int64(total) {
		r.bar.ChangeMax64(int64(total))
	}
	_ = r.bar.Set64(int64(completed))
	if completed == total {
		r.bar = nil
	}
}

func (r *Renderer) Close() {
	if !r.enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clear()
}

func (r *Renderer) newBar(update app.ProgressUpdate, total int) *progressbar.ProgressBar {
	description := fmt.Sprintf(
		"[green][%d/%d][reset] [cyan]%s[reset]",
		update.Stage,
		update.Stages,
		update.Description,
	)
	return progressbar.NewOptions(total,
		progressbar.OptionSetWriter(r.writer),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "▒",
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionOnCompletion(func() { _, _ = fmt.Fprintln(r.writer) }),
	)
}

func (r *Renderer) clear() {
	if r.bar != nil {
		_ = r.bar.Clear()
		r.bar = nil
	}
}
