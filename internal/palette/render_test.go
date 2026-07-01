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

func TestRenderIncludesQueryDetailBoxAndHint(t *testing.T) {
	item := Item{
		Token:       "task:server",
		Section:     "Tasks",
		Name:        "Server",
		Description: "Run server",
		Command:     "npm run dev",
		Directory:   "/tmp/work",
		State:       "not running",
		StateKind:   StateIdle,
	}
	model := newModel([]Item{item}, nil, []string{"Tasks"})
	model.query = "srv"
	lines := strings.Join(model.Render(80, 24), "\n")
	// Query echoed, rounded accent box with the "Run" verb title, command and
	// directory preview lines, and the key hint with placement actions.
	for _, want := range []string{"srv", "╭─", "Run", "npm run dev", "/tmp/work", "enter run", "tab actions"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("rendered palette missing %q:\n%s", want, lines)
		}
	}
}

func TestRenderEmptyQueryShowsPlaceholder(t *testing.T) {
	item := Item{Token: "task:server", Section: "Tasks", Name: "Server", Command: "npm run dev", StateKind: StateIdle}
	model := newModel([]Item{item}, nil, []string{"Tasks"})
	lines := strings.Join(model.Render(80, 24), "\n")
	if !strings.Contains(lines, "Type to filter") {
		t.Fatalf("empty query should show placeholder:\n%s", lines)
	}
}

func TestRenderProjectEntryOpensWithoutTabActions(t *testing.T) {
	item := Item{
		Token:       "project:web",
		Section:     "Projects",
		Name:        "web",
		Description: "~/code/web",
		Directory:   "/home/user/code/web",
		State:       "new workspace",
		StateKind:   StateAvailable,
	}
	model := newModel([]Item{item}, nil, []string{"Projects"})
	lines := strings.Join(model.Render(80, 24), "\n")
	if !strings.Contains(lines, "Open") || !strings.Contains(lines, "enter open") {
		t.Fatalf("project entry should use the Open verb:\n%s", lines)
	}
	if strings.Contains(lines, "tab actions") {
		t.Fatalf("project entry should not offer placement actions:\n%s", lines)
	}
}

func TestLaunchListScrollsToCursorWithinViewport(t *testing.T) {
	var items []Item
	for i := 0; i < 12; i++ {
		items = append(items, Item{
			Token:       "task:item",
			Section:     "Tasks",
			Name:        "Item" + string(rune('A'+i)),
			Description: "Run item",
			Command:     "true",
			State:       "not running",
			StateKind:   StateIdle,
		})
	}
	model := newModel(items, nil, []string{"Tasks"})
	for i := 0; i < 9; i++ {
		model.move(1)
	}
	lines := strings.Join(model.BodyLines(80, 5), "\n")
	if strings.Contains(lines, "ItemA") || !strings.Contains(lines, "ItemJ") {
		t.Fatalf("viewport did not scroll to selected item:\n%s", lines)
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
