// Package ui provides click's styled terminal output: the CLICK-AI banner, success/fail/step/info
// lines, and a spinner-backed step runner — matching gentle-ai's Charmbracelet-based aesthetic
// while degrading to plain, ANSI-free text when color isn't safe (non-TTY, NO_COLOR, --no-color).
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

// styleRenderer is a lipgloss renderer with its color profile forced on, independent of the
// actual stdout it's attached to. Renderer.Color already encodes click's own TTY/NO_COLOR/
// --no-color decision (shouldUseColor); once that decision is "color", styling must always
// produce real ANSI codes rather than lipgloss silently downgrading to NoColor because it can't
// detect a terminal on its own (e.g. under `go test`, or when Out is a bytes.Buffer).
//
// Note: passing termenv.WithProfile to lipgloss.NewRenderer is not enough on its own —
// lipgloss.Renderer.ColorProfile() re-derives the profile from the environment unless
// SetColorProfile is also called explicitly (see lipgloss's renderer.go: it only trusts an
// explicit profile once explicitColorProfile is set).
var styleRenderer = newForcedColorRenderer()

func newForcedColorRenderer() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(termenv.ANSI))
	r.SetColorProfile(termenv.ANSI)
	return r
}

// Renderer renders click's CLI output, either styled (color) or plain (no ANSI codes at all).
// Color is decided once, up front, per tech-spec.md §2's TTY/color-safety rules: it must be off
// whenever stdout isn't a real terminal, NO_COLOR is set, or the caller passed --no-color.
type Renderer struct {
	// Color enables ANSI/lipgloss styling. When false, every render method returns plain text
	// with no escape sequences — safe for piping, logging, and non-interactive CI output.
	Color bool
	// Out is where RunStep writes its spinner/result lines. Renderer's pure render methods
	// (Banner, Success, Fail, Step, Info) don't use it — they just return strings.
	Out io.Writer
}

// NewRenderer builds a Renderer for out, deciding Color from (in priority order): the
// --no-color flag, the NO_COLOR env var, and whether out is a real terminal.
func NewRenderer(out io.Writer, noColorFlag bool) *Renderer {
	return &Renderer{Color: shouldUseColor(out, noColorFlag), Out: out}
}

// shouldUseColor implements the TTY/color-safety rule: color is only ever on when nothing tells
// it to be off and out is provably a terminal (an *os.File that isatty reports as a TTY).
func shouldUseColor(out io.Writer, noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// Banner renders the CLICK-AI ASCII banner plus its tagline. In color mode each banner line gets
// a color from the cyan→blue ramp (bannerGradient) and the tagline is dimmed. In plain mode it's
// returned verbatim with no styling at all.
func (r *Renderer) Banner() string {
	if !r.Color {
		return bannerArt + "\n" + tagline
	}

	lines := strings.Split(bannerArt, "\n")
	var sb strings.Builder
	for i, line := range lines {
		color := bannerGradient[i%len(bannerGradient)]
		sb.WriteString(styleRenderer.NewStyle().Foreground(lipgloss.Color(color)).Render(line))
		sb.WriteString("\n")
	}
	sb.WriteString(styleRenderer.NewStyle().Faint(true).Render(tagline))
	return sb.String()
}

// Success renders a completed-step line: "✓ msg" in color mode, "[OK] msg" in plain mode.
func (r *Renderer) Success(msg string) string {
	if !r.Color {
		return "[OK] " + msg
	}
	return styleRenderer.NewStyle().Foreground(lipgloss.Color("2")).Render("✓ " + msg)
}

// Fail renders a failed-step line: "✗ msg" in color mode, "[FAIL] msg" in plain mode.
func (r *Renderer) Fail(msg string) string {
	if !r.Color {
		return "[FAIL] " + msg
	}
	return styleRenderer.NewStyle().Foreground(lipgloss.Color("1")).Render("✗ " + msg)
}

// Step renders an in-progress step label (no leading marker — RunStep prepends the spinner
// frame itself). Plain mode prefixes "[..] " so the line still reads clearly without color.
func (r *Renderer) Step(msg string) string {
	if !r.Color {
		return "[..] " + msg
	}
	return styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Render(msg)
}

// Info renders an informational line: dimmed in color mode, "[i] msg" in plain mode.
func (r *Renderer) Info(msg string) string {
	if !r.Color {
		return "[i] " + msg
	}
	return styleRenderer.NewStyle().Foreground(lipgloss.Color("4")).Render(msg)
}

// spinnerTick is how often RunStep redraws its spinner frame in color mode.
const spinnerTick = 80 * time.Millisecond

// RunStep runs fn, showing a spinner next to runningLabel while it's in flight (color mode
// only), then prints a Success(doneLabel) or Fail(doneLabel) line once fn returns. It returns
// fn's error unchanged. In plain mode there is no spinner animation — just a "[..] runningLabel"
// line followed by the result line, so output stays ANSI-free.
func (r *Renderer) RunStep(runningLabel, doneLabel string, fn func() error) error {
	if !r.Color {
		fmt.Fprintln(r.Out, "[..] "+runningLabel)
		err := fn()
		if err != nil {
			fmt.Fprintln(r.Out, r.Fail(doneLabel))
		} else {
			fmt.Fprintln(r.Out, r.Success(doneLabel))
		}
		return err
	}

	frames := spinner.Dot.Frames
	style := styleRenderer.NewStyle().Foreground(lipgloss.Color("6"))

	done := make(chan error, 1)
	go func() { done <- fn() }()

	ticker := time.NewTicker(spinnerTick)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case err := <-done:
			fmt.Fprint(r.Out, "\r\033[K")
			if err != nil {
				fmt.Fprintln(r.Out, r.Fail(doneLabel))
			} else {
				fmt.Fprintln(r.Out, r.Success(doneLabel))
			}
			return err
		case <-ticker.C:
			fmt.Fprintf(r.Out, "\r\033[K%s %s", style.Render(frames[frame%len(frames)]), runningLabel)
			frame++
		}
	}
}
