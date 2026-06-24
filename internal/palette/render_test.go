package palette

import (
	"strings"
	"testing"
)

func TestSidebarRendersOptionalSummaryBelowRunningAgent(t *testing.T) {
	lines := strings.Join(SidebarLines([]Item{runningItem("Fix flaky tests")}, 30, false, 0), "\n")
	if !strings.Contains(lines, "codex") || !strings.Contains(lines, "Fix flaky tests") {
		t.Fatalf("sidebar lines = %q", lines)
	}
}

func TestSidebarSummaryKeepsSpaceBeforeDivider(t *testing.T) {
	lines := SidebarLines([]Item{runningItem("A summary long enough to reach the divider")}, 30, false, 0)
	summary := strings.TrimPrefix(strings.TrimSuffix(lines[len(lines)-1], Reset), Dim)
	if len([]rune(summary)) != 30 {
		t.Fatalf("summary visible length = %d, line=%q", len([]rune(summary)), summary)
	}
	if !strings.HasSuffix(summary, " ") {
		t.Fatalf("summary should end with space: %q", summary)
	}
}

func TestSidebarKeepsSingleLineItemWhenSummaryIsMissing(t *testing.T) {
	lines := SidebarLines([]Item{runningItem("")}, 30, false, 0)
	if len(lines) != 2 {
		t.Fatalf("line count = %d", len(lines))
	}
}

func runningItem(summary string) Item {
	return Item{
		Token:       "running:%1",
		Section:     RunningSection,
		Name:        "codex",
		Description: "window 1",
		Command:     "codex",
		State:       "running",
		StateKind:   StateRunning,
		Summary:     summary,
	}
}
