package backend

import (
	"strings"
	"testing"

	"github.com/Martin-Nyaga/muxpad/internal/config"
)

func TestWrappedCommandCloseExitsWithCommandStatus(t *testing.T) {
	got := WrappedCommand("false", config.ExitClose, CommandWrapOptions{})
	if !strings.Contains(got, "( false\n); status=$?") || !strings.Contains(got, `exit "$status"`) {
		t.Fatalf("wrapped close command = %q", got)
	}
}

func TestWrappedCommandKeepSeedsHistoryAndDropsToShell(t *testing.T) {
	got := WrappedCommand("pnpm dev", config.ExitKeep, CommandWrapOptions{})
	for _, want := range []string{"muxpad_seed_history", "pnpm dev", `exec "${SHELL:-/bin/sh}"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("wrapped keep command missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "[ $status -eq 0 ]") {
		t.Fatalf("keep command should always drop to shell: %q", got)
	}
}

func TestWrappedCommandKeepOnErrorClosesOnlyOnSuccess(t *testing.T) {
	got := WrappedCommand("go test ./...", config.ExitKeepOnError, CommandWrapOptions{CloseCommand: `herdr-close "$status"`})
	if !strings.Contains(got, `[ $status -eq 0 ]`) || !strings.Contains(got, `herdr-close "$status"`) || !strings.Contains(got, "else muxpad_drop") {
		t.Fatalf("wrapped keep-on-error command = %q", got)
	}
}

func TestWrappedCommandDefaultsToKeepOnError(t *testing.T) {
	got := WrappedCommand("pnpm dev", "", CommandWrapOptions{})
	if !strings.Contains(got, `[ $status -eq 0 ]`) || !strings.Contains(got, "else muxpad_drop") {
		t.Fatalf("default wrapper = %q", got)
	}
}
