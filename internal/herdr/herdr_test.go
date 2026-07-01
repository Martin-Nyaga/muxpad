package herdr

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Martin-Nyaga/muxpad/internal/backend"
	"github.com/Martin-Nyaga/muxpad/internal/config"
)

func TestCreateTabCreatesFocusedTabAndFallsBackToCurrentPane(t *testing.T) {
	var calls [][]string
	client := &Client{
		Bin: "herdr-test",
		Run: func(args ...string) Result {
			calls = append(calls, append([]string{}, args...))
			switch len(calls) {
			case 1:
				return Result{Stdout: "created tab t1\n", OK: true}
			case 2:
				return Result{Stdout: `{"result":{"pane":{"pane_id":"p1"}}}`, OK: true}
			default:
				t.Fatalf("unexpected call: %#v", args)
				return Result{}
			}
		},
	}

	pane, err := client.CreateTab(backend.CreateTabSpec{Label: "muxpad skeleton", Directory: "/tmp/project", Focus: true})
	if err != nil {
		t.Fatal(err)
	}
	if pane.ID != "p1" {
		t.Fatalf("pane.ID = %q, want p1", pane.ID)
	}
	want := [][]string{
		{"herdr-test", "tab", "create", "--cwd", "/tmp/project", "--label", "muxpad skeleton", "--focus"},
		{"herdr-test", "pane", "current", "--current"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestRunInPaneSubmitsCommandToHerdrPaneRun(t *testing.T) {
	var got []string
	client := &Client{
		Bin: "herdr-test",
		Run: func(args ...string) Result {
			got = append([]string{}, args...)
			return Result{OK: true}
		},
	}

	err := client.RunInPane(backend.Pane{ID: "p1"}, "while :; do sleep 60; done")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"herdr-test", "pane", "run", "p1", "while :; do sleep 60; done"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestOpenPaletteUsesHerdrOverlayPaneEntrypoint(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"workspace_id":"w1","focused_pane_id":"w1:p1"}`)
	var got []string
	client := &Client{
		Bin: "herdr-test",
		Run: func(args ...string) Result {
			got = append([]string{}, args...)
			return Result{OK: true}
		},
	}

	if err := client.OpenPalette(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"herdr-test", "plugin", "pane", "open",
		"--plugin", "muxpad",
		"--entrypoint", "palette",
		"--placement", "overlay",
		"--focus",
		"--env", `MUXPAD_HERDR_CONTEXT_JSON={"workspace_id":"w1","focused_pane_id":"w1:p1"}`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestLaunchCreatesWorkspaceTabAndRunsDeclaredCommand(t *testing.T) {
	var calls [][]string
	client := &Client{
		Bin: "herdr-test",
		Run: func(args ...string) Result {
			calls = append(calls, append([]string{}, args...))
			switch len(calls) {
			case 1:
				return Result{Stdout: `{"result":{"root_pane":{"pane_id":"w1:p2"}}}`, OK: true}
			case 2:
				return Result{OK: true}
			default:
				t.Fatalf("unexpected call: %#v", args)
				return Result{}
			}
		},
	}

	_, err := client.Launch(backend.LaunchSpec{
		Workspace: "w1",
		Name:      "API",
		Root:      "/repo",
		Definition: config.Definition{
			ID:        "api",
			Name:      "API",
			Command:   "pnpm dev:api",
			Directory: "services/api",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := calls[0], []string{"herdr-test", "tab", "create", "--workspace", "w1", "--cwd", "/repo/services/api", "--label", "API", "--focus"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tab call = %#v, want %#v", got, want)
	}
	if got := calls[1]; got[0] != "herdr-test" || got[1] != "pane" || got[2] != "run" || got[3] != "w1:p2" || !strings.HasPrefix(got[4], "exec sh -c ") || !strings.Contains(got[4], "pnpm dev:api") || !strings.Contains(got[4], "[ $status -eq 0 ]") {
		t.Fatalf("run call = %#v", got)
	}
}

func TestLaunchHonorsVerticalSplitPlacement(t *testing.T) {
	var calls [][]string
	client := &Client{
		Bin: "herdr-test",
		Run: func(args ...string) Result {
			calls = append(calls, append([]string{}, args...))
			switch len(calls) {
			case 1:
				return Result{Stdout: `{"result":{"pane":{"pane_id":"w1:p3"}}}`, OK: true}
			case 2:
				return Result{OK: true}
			default:
				t.Fatalf("unexpected call: %#v", args)
				return Result{}
			}
		},
	}

	_, err := client.Launch(backend.LaunchSpec{
		Workspace: "w1",
		Target:    "w1:p1",
		Name:      "API",
		Root:      "/repo",
		Placement: config.PlacementVertical,
		Definition: config.Definition{
			ID:        "api",
			Name:      "API",
			Command:   "pnpm dev:api",
			Placement: config.PlacementVertical,
			ExitMode:  config.ExitKeep,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := calls[0], []string{"herdr-test", "pane", "split", "--pane", "w1:p1", "--direction", "down", "--cwd", "/repo", "--focus"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("split call = %#v, want %#v", got, want)
	}
	if got := calls[1]; got[0] != "herdr-test" || got[1] != "pane" || got[2] != "run" || got[3] != "w1:p3" || !strings.HasPrefix(got[4], "exec sh -c ") || !strings.Contains(got[4], "muxpad_seed_history") || strings.Contains(got[4], "[ $status -eq 0 ]") {
		t.Fatalf("run call = %#v", got)
	}
}

func TestLaunchHonorsHorizontalSplitPlacementAndCloseExitMode(t *testing.T) {
	var calls [][]string
	client := &Client{
		Bin: "herdr-test",
		Run: func(args ...string) Result {
			calls = append(calls, append([]string{}, args...))
			switch len(calls) {
			case 1:
				return Result{Stdout: `{"result":{"pane":{"pane_id":"w1:p4"}}}`, OK: true}
			case 2:
				return Result{OK: true}
			default:
				t.Fatalf("unexpected call: %#v", args)
				return Result{}
			}
		},
	}

	_, err := client.Launch(backend.LaunchSpec{
		Target:    "w1:p1",
		Name:      "Tests",
		Root:      "/repo",
		Placement: config.PlacementHorizontal,
		Definition: config.Definition{
			ID:       "test",
			Name:     "Tests",
			Command:  "go test ./...",
			ExitMode: config.ExitClose,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := calls[0], []string{"herdr-test", "pane", "split", "--pane", "w1:p1", "--direction", "right", "--cwd", "/repo", "--focus"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("split call = %#v, want %#v", got, want)
	}
	if got := calls[1][4]; !strings.HasPrefix(got, "exec sh -c ") || !strings.Contains(got, "go test ./...") || !strings.Contains(got, `exit "$status"`) || strings.Contains(got, "muxpad_seed_history") {
		t.Fatalf("close wrapper = %q", got)
	}
}

func TestPaneIDParsesNestedJSON(t *testing.T) {
	got := paneID(`{"result":{"created_tab":{"tab_id":"t1"},"root_pane":{"pane_id":"p1"}}}`)
	if got != "p1" {
		t.Fatalf("paneID = %q, want p1", got)
	}
}
