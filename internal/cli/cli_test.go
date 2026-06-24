package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Martin-Nyaga/muxpad/internal/config"
)

type fakeApp struct {
	calls []any
}

func (f *fakeApp) Start(projectID string, empty, attach bool) (string, error) {
	f.calls = append(f.calls, []any{"start", projectID, empty, attach})
	return "", nil
}
func (f *fakeApp) Menu(attach bool) (string, error) {
	f.calls = append(f.calls, []any{"menu", attach})
	return "", nil
}
func (f *fakeApp) Task(id string, placement config.Placement, attach bool) error {
	f.calls = append(f.calls, []any{"task", id, placement, attach})
	return nil
}
func (f *fakeApp) Agent(id string, placement config.Placement, attach bool) error {
	f.calls = append(f.calls, []any{"agent", id, placement, attach})
	return nil
}

type fakeTmux struct {
	inside bool
	calls  []any
}

func (f *fakeTmux) Available() bool { return true }
func (f *fakeTmux) Inside() bool    { return f.inside }
func (f *fakeTmux) PopupMenu(program string) error {
	f.calls = append(f.calls, []any{"popup_menu", program})
	return nil
}

func TestDispatchesDirectCommandsAndPlacementFlags(t *testing.T) {
	app := &fakeApp{}
	code := (&CLI{Args: []string{"agent", "codex", "--vertical"}, Output: &bytes.Buffer{}, Error: &bytes.Buffer{}, App: app, Tmux: &fakeTmux{}}).Run()
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	call := app.calls[0].([]any)
	if call[0] != "agent" || call[1] != "codex" || call[2] != config.PlacementVertical {
		t.Fatalf("call = %#v", call)
	}
}

func TestDispatchesStartEmpty(t *testing.T) {
	app := &fakeApp{}
	code := (&CLI{Args: []string{"start", "sample-app", "--empty"}, Output: &bytes.Buffer{}, Error: &bytes.Buffer{}, App: app, Tmux: &fakeTmux{}}).Run()
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	call := app.calls[0].([]any)
	if call[0] != "start" || call[1] != "sample-app" || call[2] != true {
		t.Fatalf("call = %#v", call)
	}
}

func TestReportsInvalidCommands(t *testing.T) {
	var stderr bytes.Buffer
	code := (&CLI{Args: []string{"unknown"}, Output: &bytes.Buffer{}, Error: &stderr, App: &fakeApp{}, Tmux: &fakeTmux{}}).Run()
	if code != 1 || !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestMenuInsideTmuxOpensPopupInsteadOfRenderingPalette(t *testing.T) {
	tmux := &fakeTmux{inside: true}
	app := &fakeApp{}
	code := (&CLI{Args: []string{"menu"}, Output: &bytes.Buffer{}, Error: &bytes.Buffer{}, App: app, Tmux: tmux, Program: "muxpad"}).Run()
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if len(app.calls) != 0 || len(tmux.calls) != 1 || tmux.calls[0].([]any)[0] != "popup_menu" {
		t.Fatalf("app=%#v tmux=%#v", app.calls, tmux.calls)
	}
}
