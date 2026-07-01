package palette

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A resize (SIGWINCH) must trigger a repaint on its own, without waiting for a
// keypress. This is the regression for the overlay-startup flicker: the first
// paint lands at the not-yet-settled size, and the loop must redraw when the
// pane resizes instead of parking until any key is pressed.
func TestRunLoopRepaintsOnResizeAndKey(t *testing.T) {
	painted := make(chan struct{}, 8)
	render := func() { painted <- struct{}{} }
	handle := func(key string) (Selection, bool, bool) {
		if key == "enter" {
			return Selection{Token: "chosen"}, true, true
		}
		return Selection{}, false, false
	}

	keys := make(chan string)
	resize := make(chan os.Signal, 1)
	result := make(chan Selection, 1)
	go func() {
		selection, _ := runLoop(render, handle, keys, resize)
		result <- selection
	}()

	waitPaint(t, painted) // initial paint
	resize <- unix.SIGWINCH
	waitPaint(t, painted) // resize repaints with no key
	keys <- "down"
	waitPaint(t, painted) // a non-terminal key still repaints

	keys <- "enter"
	select {
	case selection := <-result:
		if selection.Token != "chosen" {
			t.Fatalf("got %q, want chosen", selection.Token)
		}
	case <-time.After(time.Second):
		t.Fatal("runLoop did not return after enter")
	}
}

func waitPaint(t *testing.T, painted <-chan struct{}) {
	t.Helper()
	select {
	case <-painted:
	case <-time.After(time.Second):
		t.Fatal("expected a repaint")
	}
}
