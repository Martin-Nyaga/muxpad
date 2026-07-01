package app

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Martin-Nyaga/muxpad/internal/agent"
	"github.com/Martin-Nyaga/muxpad/internal/backend"
	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/palette"
)

type fakeTmux struct {
	inside         bool
	existing       bool
	workspaces     []backend.Workspace
	panes          []backend.Pane
	projectContext string
	root           string
	calls          []any
}

func (f *fakeTmux) Inside() bool                      { return f.inside }
func (f *fakeTmux) CurrentSession() (string, error)   { return "original", nil }
func (f *fakeTmux) CurrentWorkspace() (string, error) { return "original", nil }
func (f *fakeTmux) CurrentPane() (string, error)      { return "%9", nil }
func (f *fakeTmux) SessionExists(string) bool         { return f.existing }
func (f *fakeTmux) WorkspaceExists(string) bool       { return f.existing }
func (f *fakeTmux) Sessions() []string                { return nil }
func (f *fakeTmux) Workspaces() []string              { return nil }
func (f *fakeTmux) WorkspaceList() ([]backend.Workspace, error) {
	return f.workspaces, nil
}
func (f *fakeTmux) CreateSession(name, root, projectID string) (string, error) {
	f.calls = append(f.calls, []any{"create_session", name, root, projectID})
	return "%1", nil
}
func (f *fakeTmux) CreateWorkspace(name, root, projectID string) (string, error) {
	return f.CreateSession(name, root, projectID)
}
func (f *fakeTmux) ProjectContext(string) string { return f.projectContext }
func (f *fakeTmux) SessionRoot(string) string {
	if f.root != "" {
		return f.root
	}
	return "/ordinary"
}
func (f *fakeTmux) WorkspaceRoot(string) string          { return f.SessionRoot("") }
func (f *fakeTmux) ManagedRoot(string) string            { return "" }
func (f *fakeTmux) Panes(string) ([]backend.Pane, error) { return f.panes, nil }
func (f *fakeTmux) Launch(spec backend.LaunchSpec) (string, error) {
	f.calls = append(f.calls, spec)
	return "%10", nil
}
func (f *fakeTmux) Focus(p backend.Pane) error {
	f.calls = append(f.calls, []any{"focus", p.ID})
	return nil
}
func (f *fakeTmux) Restart(p backend.Pane, d config.Definition) error {
	f.calls = append(f.calls, []any{"restart", p.ID, d.ID})
	return nil
}
func (f *fakeTmux) Attach(string) error { return nil }
func (f *fakeTmux) Switch(session string) error {
	f.calls = append(f.calls, []any{"switch", session})
	return nil
}
func (f *fakeTmux) FocusWorkspace(workspace string) error {
	f.calls = append(f.calls, []any{"focus_workspace", workspace})
	return nil
}
func (f *fakeTmux) PopupMenu(string) error                                { return nil }
func (f *fakeTmux) KillSession(string) error                              { return nil }
func (f *fakeTmux) KillWorkspace(string) error                            { return nil }
func (f *fakeTmux) CreateTab(backend.CreateTabSpec) (backend.Pane, error) { return backend.Pane{}, nil }
func (f *fakeTmux) SplitPane(backend.SplitPaneSpec) (backend.Pane, error) { return backend.Pane{}, nil }
func (f *fakeTmux) RunInPane(backend.Pane, string) error                  { return nil }

type fakeAgentDiscovery map[string]string

func (f fakeAgentDiscovery) Detect([]agent.Pane) map[string]string { return f }

type fakeDiscovery []config.Definition

func (f fakeDiscovery) Scripts(string, []string) []config.Definition { return f }

func TestDecliningInsideTmuxSwitchDoesNotCreateOrChangeTarget(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{inside: true, root: project}
	var out bytes.Buffer
	app := testApp(t, project, tmuxFake)
	app.Input = strings.NewReader("\n")
	app.Output = &out
	session, err := app.Start("work", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if session != "work" || len(tmuxFake.calls) != 0 || !strings.Contains(out.String(), "Switch tmux client") {
		t.Fatalf("session=%s calls=%#v output=%q", session, tmuxFake.calls, out.String())
	}
}

func TestAcceptingInsideTmuxSwitchCreatesDefaultsThenSwitches(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{inside: true}
	app := testApp(t, project, tmuxFake)
	app.Input = strings.NewReader("yes\n")
	app.Output = &bytes.Buffer{}
	if _, err := app.Start("work", false, false); err != nil {
		t.Fatal(err)
	}
	if got := []string{tmuxFake.calls[0].([]any)[0].(string), "launch", tmuxFake.calls[2].([]any)[0].(string)}; !reflect.DeepEqual(got, []string{"create_session", "launch", "switch"}) {
		t.Fatalf("calls = %#v", tmuxFake.calls)
	}
}

func TestDirectAgentInsideOrdinarySessionTargetsCurrentPane(t *testing.T) {
	cfg := &config.Config{Agents: []config.Definition{{ID: "codex", Name: "codex", Command: "sleep 30", Executable: "sleep", Placement: config.PlacementWindow, ExitMode: config.ExitClose, Enabled: true}}}
	tmuxFake := &fakeTmux{inside: true, existing: true}
	app := &Application{Config: cfg, Tmux: tmuxFake, Discovery: fakeDiscovery{}, AgentDiscovery: fakeAgentDiscovery{}, Palette: nil, Input: strings.NewReader(""), Output: &bytes.Buffer{}}
	if err := app.Agent("codex", config.PlacementHorizontal, false); err != nil {
		t.Fatal(err)
	}
	launch := tmuxFake.calls[0].(backend.LaunchSpec)
	if launch.Workspace != "original" || launch.Target != "%9" || launch.Placement != config.PlacementHorizontal {
		t.Fatalf("launch = %#v", launch)
	}
}

func TestCodexLaunchRequestsThreadTerminalTitle(t *testing.T) {
	def := config.Definition{ID: "codex", Command: "codex --model test-model"}
	got := agentLaunchDefinition(def).Command
	if !strings.Contains(got, "terminal_title") || !strings.Contains(got, "--model test-model") {
		t.Fatalf("command = %q", got)
	}
}

func TestAgentSummaryUsesOnlyMeaningfulTitles(t *testing.T) {
	pane := backend.Pane{Kind: "agent", DefinitionID: "codex", Name: "codex", Title: "  Fix   flaky tests  "}
	if got := AgentSummary(pane); got != "Fix flaky tests" {
		t.Fatalf("summary = %q", got)
	}
	pane.Title = "Codex"
	if AgentSummary(pane) != "" {
		t.Fatal("generic codex title should be ignored")
	}
	pane.Title = "019edd47-91f2-7102-b113-d047160a33d8"
	if AgentSummary(pane) != "" {
		t.Fatal("codex uuid title should be ignored")
	}
	pane.DefinitionID = "claude"
	pane.Name = "claude"
	pane.Title = "✳ Refactor authentication"
	if got := AgentSummary(pane); got != "* Refactor authentication" {
		t.Fatalf("claude summary = %q", got)
	}
}

func TestRunningAndFinishedTasksInPaletteItems(t *testing.T) {
	project := t.TempDir()
	pane := backend.Pane{ID: "%1", Workspace: "work", Window: "@1", WindowIndex: "1", Kind: "task", DefinitionID: "server", Name: "Server", CurrentCommand: "sleep"}
	tmuxFake := &fakeTmux{panes: []backend.Pane{pane}, projectContext: "work"}
	app := testApp(t, project, tmuxFake)
	items, err := app.PaletteItems("work")
	if err != nil {
		t.Fatal(err)
	}
	launch := findItem(items, "task:server", "Tasks")
	running := findItem(items, "running:%1", palette.RunningSection)
	if launch.StateKind != palette.StateRunning || running.Description != "window 1 · sleep" {
		t.Fatalf("items = %#v %#v", launch, running)
	}

	tmuxFake.panes[0].Finished = true
	items, _ = app.PaletteItems("work")
	sidebar := findItem(items, "task:server", palette.RunningSection)
	if sidebar.StateKind != palette.StateFinished {
		t.Fatalf("finished sidebar item = %#v", sidebar)
	}
}

func TestUnmanagedDetectedAgentAppearsAsNumberedInstance(t *testing.T) {
	managed := backend.Pane{ID: "%1", WindowIndex: "1", Kind: "agent", DefinitionID: "codex", Name: "codex", CurrentCommand: "node", Title: "Codex", PID: "100"}
	unmanaged := backend.Pane{ID: "%2", WindowIndex: "2", CurrentCommand: "node", Title: "Investigate timeout", PID: "200"}
	tmuxFake := &fakeTmux{panes: []backend.Pane{managed, unmanaged}}
	app := &Application{Config: &config.Config{Agents: []config.Definition{}}, Tmux: tmuxFake, Discovery: fakeDiscovery{}, AgentDiscovery: fakeAgentDiscovery{"%2": "codex"}, Input: strings.NewReader(""), Output: &bytes.Buffer{}}
	items, err := app.PaletteItems("work")
	if err != nil {
		t.Fatal(err)
	}
	item := findItem(items, "running:%2", palette.RunningSection)
	if item.Name != "codex 2" || item.Summary != "Investigate timeout" {
		t.Fatalf("item = %#v", item)
	}
}

func TestDeclaredTaskMenuResolvesProjectFromWorkspaceRootAndLaunchesSelection(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{inside: true, root: project}
	app := testApp(t, project, tmuxFake)
	app.Config.Projects[0].Tasks[0].Placement = config.PlacementVertical
	app.Palette = stubPalette{selectResult: palette.Selection{Action: "enter", Token: "task:server"}, selectOK: true}

	if err := app.DeclaredTaskMenu(); err != nil {
		t.Fatal(err)
	}
	launch := tmuxFake.calls[0].(backend.LaunchSpec)
	if launch.Workspace != "original" || launch.Root != project || launch.Definition.ID != "server" || launch.Placement != config.PlacementVertical || launch.Target != "%9" {
		t.Fatalf("launch = %#v", launch)
	}
}

func TestDeclaredTaskMenuFocusesExistingTaskSelection(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{
		inside: true,
		root:   project,
		panes: []backend.Pane{{
			ID:           "w1:p1",
			Kind:         "task",
			DefinitionID: "server",
			Name:         "Server",
		}},
	}
	app := testApp(t, project, tmuxFake)
	app.Palette = stubPalette{selectResult: palette.Selection{Action: "enter", Token: "task:server"}, selectOK: true}

	if err := app.DeclaredTaskMenu(); err != nil {
		t.Fatal(err)
	}
	if got, want := tmuxFake.calls, []any{[]any{"focus", "w1:p1"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestDeclaredTaskMenuIncludesDiscoveredScriptsBelowTasks(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{inside: true, root: project}
	app := testApp(t, project, tmuxFake)
	app.Discovery = fakeDiscovery{
		{ID: "duplicate", Name: "duplicate", Description: "duplicate", Command: "sleep 30", Placement: config.PlacementWindow, ExitMode: config.ExitKeepOnError, Enabled: true, Executable: "sleep"},
		{ID: "dev", Name: "dev", Description: "vite", Command: "npm run dev", Placement: config.PlacementWindow, ExitMode: config.ExitKeepOnError, Enabled: true, Executable: "npm"},
	}
	pal := &recordingPalette{selectResult: palette.Selection{Action: "enter", Token: "script:dev"}, selectOK: true}
	app.Palette = pal

	if err := app.DeclaredTaskMenu(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pal.sectionOrder, []string{"Tasks", "Discovered scripts"}) {
		t.Fatalf("section order = %#v", pal.sectionOrder)
	}
	if findItem(pal.items, "task:server", "Tasks").Token == "" {
		t.Fatalf("task missing from palette: %#v", pal.items)
	}
	if findItem(pal.items, "script:dev", "Discovered scripts").Token == "" {
		t.Fatalf("discovered script missing from palette: %#v", pal.items)
	}
	if findItem(pal.items, "script:duplicate", "Discovered scripts").Token != "" {
		t.Fatalf("duplicate discovered script should be filtered: %#v", pal.items)
	}
	launch := tmuxFake.calls[0].(backend.LaunchSpec)
	if launch.Kind != "script" || launch.Definition.ID != "dev" || launch.Root != project || launch.Target != "%9" {
		t.Fatalf("launch = %#v", launch)
	}
}

func TestDeclaredTaskMenuFocusesExistingDiscoveredScriptSelection(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{
		inside: true,
		root:   project,
		panes: []backend.Pane{{
			ID:           "w1:p2",
			Kind:         "script",
			DefinitionID: "dev",
			Name:         "dev",
		}},
	}
	app := testApp(t, project, tmuxFake)
	app.Discovery = fakeDiscovery{
		{ID: "dev", Name: "dev", Description: "vite", Command: "npm run dev", Placement: config.PlacementWindow, ExitMode: config.ExitKeepOnError, Enabled: true, Executable: "npm"},
	}
	app.Palette = stubPalette{selectResult: palette.Selection{Action: "enter", Token: "script:dev"}, selectOK: true}

	if err := app.DeclaredTaskMenu(); err != nil {
		t.Fatal(err)
	}
	if got, want := tmuxFake.calls, []any{[]any{"focus", "w1:p2"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestOpenProjectCreatesEmptyWorkspaceWhenMissing(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{}
	app := testApp(t, project, tmuxFake)

	if err := app.OpenProject("work"); err != nil {
		t.Fatal(err)
	}
	if got, want := tmuxFake.calls, []any{[]any{"create_session", "Work", project, "work"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestOpenProjectFocusesExistingWorkspaceByRoot(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{workspaces: []backend.Workspace{{ID: "w2", Label: "renamed", Root: project}}}
	app := testApp(t, project, tmuxFake)

	if err := app.OpenProject("work"); err != nil {
		t.Fatal(err)
	}
	if got, want := tmuxFake.calls, []any{[]any{"focus_workspace", "w2"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestProjectLauncherMenuEnumeratesConfiguredProjectsAndFocusesSelection(t *testing.T) {
	project := t.TempDir()
	tmuxFake := &fakeTmux{workspaces: []backend.Workspace{{ID: "w2", Label: "Work", Root: project}}}
	app := testApp(t, project, tmuxFake)
	paletteFake := stubPalette{selectResult: palette.Selection{Action: "enter", Token: "project:work"}, selectOK: true}
	app.Palette = paletteFake

	if err := app.ProjectLauncherMenu(); err != nil {
		t.Fatal(err)
	}
	if got, want := tmuxFake.calls, []any{[]any{"focus_workspace", "w2"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func testApp(t *testing.T, project string, tmuxFake *fakeTmux) *Application {
	t.Helper()
	cfg := &config.Config{Projects: []config.Project{{
		ID:           "work",
		Name:         "Work",
		Root:         project,
		DefaultTasks: []string{"server"},
		Tasks: []config.Definition{{
			ID: "server", Name: "Server", Description: "Run server", Command: "sleep 30",
			Placement: config.PlacementWindow, ExitMode: config.ExitKeepOnError, Enabled: true, Executable: "sleep",
		}},
	}}, Agents: []config.Definition{{ID: "codex", Name: "codex", Description: "openai coding agent", Command: "sleep 30", Executable: "sleep", Placement: config.PlacementWindow, ExitMode: config.ExitClose, Enabled: true}}}
	return &Application{Config: cfg, Tmux: tmuxFake, Discovery: fakeDiscovery{}, AgentDiscovery: fakeAgentDiscovery{}, Input: strings.NewReader(""), Output: &bytes.Buffer{}}
}

type recordingPalette struct {
	items        []palette.Item
	sectionOrder []string
	selectResult palette.Selection
	selectOK     bool
}

func (r *recordingPalette) Select(items []palette.Item, sectionOrder []string) (palette.Selection, bool, error) {
	r.items = append([]palette.Item{}, items...)
	r.sectionOrder = append([]string{}, sectionOrder...)
	return r.selectResult, r.selectOK, nil
}

func (r *recordingPalette) Choose([]palette.Option, string) (string, bool, error) {
	return "", false, nil
}

func findItem(items []palette.Item, token, section string) palette.Item {
	for _, item := range items {
		if item.Token == token && item.Section == section {
			return item
		}
	}
	return palette.Item{}
}

var _ = filepath.Join
