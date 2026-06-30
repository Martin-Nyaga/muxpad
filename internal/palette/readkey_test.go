package palette

import (
	"bufio"
	"os"
	"testing"
	"time"
)

// A lone ESC must resolve to "escape" promptly without waiting for another
// keypress — the bug that made dismissing the menu take several Esc presses.
func TestReadKeyLoneEscapeDoesNotBlock(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if _, err := w.Write([]byte{0x1b}); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(r)
	fd := int(r.Fd())
	done := make(chan string, 1)
	go func() { done <- readKey(reader, fd) }()

	select {
	case key := <-done:
		if key != "escape" {
			t.Fatalf("got %q, want escape", key)
		}
	case <-time.After(time.Second):
		t.Fatal("readKey blocked on a lone ESC")
	}
}

// A full escape sequence still decodes to the arrow key.
func TestReadKeyArrowSequence(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if _, err := w.Write([]byte("\x1b[A")); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(r)
	if key := readKey(reader, int(r.Fd())); key != "up" {
		t.Fatalf("got %q, want up", key)
	}
}
