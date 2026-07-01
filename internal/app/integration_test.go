package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Martin-Nyaga/muxpad/internal/agent"
	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/discovery"
	"github.com/Martin-Nyaga/muxpad/internal/palette"
	"github.com/Martin-Nyaga/muxpad/internal/tmux"
)

type integrationFixture struct {
	tmp       string
	project   string
	mobile    string
	marker    string
	socket    string
	config    *config.Config
	tmux      *tmux.Client
	app       *Application
	configYML string
}

type stubPalette struct {
	selectResult palette.Selection
	selectOK     bool
	chooseResult string
	chooseOK     bool
}

func (s stubPalette) Select([]palette.Item, []string) (palette.Selection, bool, error) {
	return s.selectResult, s.selectOK, nil
}

func (s stubPalette) Choose([]palette.Option, string) (string, bool, error) {
	return s.chooseResult, s.chooseOK, nil
}

func newIntegrationFixture(t *testing.T) *integrationFixture {
	t.Helper()
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	mobile := filepath.Join(project, "mobile")
	mustIT(t, os.MkdirAll(mobile, 0o755))
	marker := filepath.Join(tmp, "discovered-ran")
	writeJSONIT(t, filepath.Join(project, "package.json"), map[string]any{
		"name":           "first",
		"packageManager": "npm@10",
		"workspaces":     []string{"mobile"},
		"scripts": map[string]string{
			"duplicate": "sleep 30",
			"rootcheck": "printf ok > " + marker + "; sleep 30",
		},
	})
	writeJSONIT(t, filepath.Join(mobile, "package.json"), map[string]any{
		"name": "app-mobile",
		"scripts": map[string]string{
			"dev":            "sleep 30",
			"noise:internal": "sleep 30",
		},
	})
	configPath := filepath.Join(tmp, "config.yml")
	configYML := fmt.Sprintf(`
projects:
  first:
    name: First
    root: %s
    default_tasks: [api, mobile]
    tasks:
      api:
        name: API server
        description: API
        command: sleep 30
        exit_mode: keep
      mobile:
        name: Mobile app
        description: Mobile
        command: sleep 30
        directory: mobile
        exit_mode: keep
      failure:
        name: Failure
        description: Fails
        command: 'exit 7'
      success:
        name: Success
        description: Succeeds
        command: 'exit 0'
      kept:
        name: Kept
        description: Kept output
        command: 'exit 0'
        exit_mode: keep
      closed:
        name: Closed
        description: Closed output
        command: 'exit 7'
        exit_mode: close
      duplicate:
        name: duplicate
        description: Configured version wins
        command: npm run duplicate
    discovery:
      exclude:
        - "app-mobile:noise:*"
agents:
  codex:
    command: sleep 30
    executable: sleep
  claude:
    disabled: true
  opencode:
    executable: muxpad-test-missing-opencode
`, project)
	mustIT(t, os.WriteFile(configPath, []byte(configYML), 0o644))
	cfg, err := config.LoadPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("muxpad-go-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Setenv("TMUX", "")
	t.Setenv("MUXPAD_TMUX_SOCKET", socket)
	t.Setenv("MUXPAD_CONFIG", configPath)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})
	client := tmux.New()
	application := &Application{
		Config:         cfg,
		Backend:        client,
		Discovery:      discovery.Discovery{},
		AgentDiscovery: agent.Discovery{},
		Palette:        stubPalette{},
		Input:          strings.NewReader(""),
		Output:         os.Stdout,
	}
	return &integrationFixture{tmp: tmp, project: project, mobile: mobile, marker: marker, socket: socket, config: cfg, tmux: client, app: application, configYML: configYML}
}

func TestIntegrationStartDefaultsRepeatEmptyAndNestedResolution(t *testing.T) {
	f := newIntegrationFixture(t)
	inDir(t, f.mobile, func() {
		session, err := f.app.Start("", false, false)
		if err != nil {
			t.Fatal(err)
		}
		if session != "first" {
			t.Fatalf("session = %q", session)
		}
	})
	got := sorted(windows(t, f.socket, "first"))
	if !reflect.DeepEqual(got, []string{"API server", "Mobile app", "shell"}) {
		t.Fatalf("windows = %v", got)
	}
	before := paneIDs(t, f.tmux, "first")
	if _, err := f.app.Start("first", false, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, paneIDs(t, f.tmux, "first")) {
		t.Fatal("repeat start should not create duplicate panes")
	}
	mustIT(t, f.tmux.KillSession("first"))
	if _, err := f.app.Start("first", true, false); err != nil {
		t.Fatal(err)
	}
	if got := windows(t, f.socket, "first"); !reflect.DeepEqual(got, []string{"shell"}) {
		t.Fatalf("empty windows = %v", got)
	}
}

func TestIntegrationTaskSingletonAgentNumberingAndPlacements(t *testing.T) {
	f := newIntegrationFixture(t)
	if _, err := f.app.Start("first", true, false); err != nil {
		t.Fatal(err)
	}
	inDir(t, f.project, func() {
		mustIT(t, f.app.Task("api", "", false))
		mustIT(t, f.app.Task("api", "", false))
		mustIT(t, f.app.Agent("codex", "", false))
		mustIT(t, f.app.Agent("codex", config.PlacementVertical, false))
	})
	if got := managed(t, f.tmux, "first", "task", "api"); len(got) != 1 {
		t.Fatalf("managed api panes = %#v", got)
	}
	var names []string
	for _, pane := range managed(t, f.tmux, "first", "agent", "codex") {
		names = append(names, pane.Name)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"codex", "codex 2"}) {
		t.Fatalf("agent names = %v", names)
	}
	if got := len(windows(t, f.socket, "first")); got != 3 {
		t.Fatalf("window count = %d", got)
	}
	if got := len(paneIDs(t, f.tmux, "first")); got != 4 {
		t.Fatalf("pane count = %d", got)
	}
}

func TestIntegrationExitModesAndRestart(t *testing.T) {
	f := newIntegrationFixture(t)
	if _, err := f.app.Start("first", true, false); err != nil {
		t.Fatal(err)
	}
	inDir(t, f.project, func() {
		mustIT(t, f.app.Task("failure", "", false))
		mustIT(t, f.app.Task("success", "", false))
		mustIT(t, f.app.Task("kept", "", false))
		mustIT(t, f.app.Task("closed", "", false))
	})
	time.Sleep(600 * time.Millisecond)
	failure := firstPane(managed(t, f.tmux, "first", "task", "failure"))
	kept := firstPane(managed(t, f.tmux, "first", "task", "kept"))
	if !failure.Finished || !kept.Finished || failure.Dead || kept.Dead {
		t.Fatalf("failure=%#v kept=%#v", failure, kept)
	}
	if got := managed(t, f.tmux, "first", "task", "success"); len(got) != 0 {
		t.Fatalf("success pane should close: %#v", got)
	}
	definition, _ := f.config.Project("first")
	task, _ := definition.Task("failure")
	mustIT(t, f.tmux.Restart(failure, task))
	time.Sleep(300 * time.Millisecond)
	if !firstPane(managed(t, f.tmux, "first", "task", "failure")).Finished {
		t.Fatal("restarted failure should finish again")
	}
}

func TestIntegrationAdHocSessionsUseCleanNamesAndDisambiguate(t *testing.T) {
	f := newIntegrationFixture(t)
	one := filepath.Join(f.tmp, "a", "web")
	two := filepath.Join(f.tmp, "b", "web")
	mustIT(t, os.MkdirAll(one, 0o755))
	mustIT(t, os.MkdirAll(two, 0o755))
	var first, reused, second string
	inDir(t, one, func() { first, _ = f.app.Start("", false, false) })
	inDir(t, one, func() { reused, _ = f.app.Start("", false, false) })
	inDir(t, two, func() { second, _ = f.app.Start("", false, false) })
	if first != "web" || reused != "web" || second != "web-2" {
		t.Fatalf("sessions = %q %q %q", first, reused, second)
	}
}

func TestIntegrationPaletteLabelsAndDiscoveredScripts(t *testing.T) {
	f := newIntegrationFixture(t)
	if _, err := f.app.Start("first", true, false); err != nil {
		t.Fatal(err)
	}
	f.app.Palette = stubPalette{selectResult: palette.Selection{Action: "tab", Token: "agent:codex"}, selectOK: true, chooseResult: "vertical", chooseOK: true}
	inDir(t, f.project, func() {
		if _, err := f.app.Menu(false); err != nil {
			t.Fatal(err)
		}
	})
	items, err := f.app.PaletteItems("first")
	if err != nil {
		t.Fatal(err)
	}
	if findItem(items, "task:api", "Tasks").Name != "API server" {
		t.Fatalf("task item missing: %#v", items)
	}
	claude := findItem(items, "agent:claude", "Agents")
	if claude.StateKind != palette.StateDisabled {
		t.Fatalf("claude item = %#v", claude)
	}
	if findItem(items, "script:rootcheck", "Discovered scripts").Token == "" ||
		findItem(items, "script:app-mobile:dev", "Discovered scripts").Token == "" ||
		findItem(items, "script:duplicate", "Discovered scripts").Token != "" {
		t.Fatalf("discovered script items = %#v", items)
	}
	mustIT(t, f.app.HandleSelection("first", palette.Selection{Action: "enter", Token: "script:rootcheck"}))
	mustIT(t, f.app.HandleSelection("first", palette.Selection{Action: "enter", Token: "script:rootcheck"}))
	data, err := waitForFile(f.marker, 3*time.Second)
	if err != nil || string(data) != "ok" {
		t.Fatalf("marker = %q err=%v", data, err)
	}
	if got := managed(t, f.tmux, "first", "script", "rootcheck"); len(got) != 1 {
		t.Fatalf("rootcheck panes = %#v", got)
	}
}

func inDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mustIT(t, os.Chdir(dir))
	defer func() { mustIT(t, os.Chdir(old)) }()
	fn()
}

func windows(t *testing.T, socket, session string) []string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "list-windows", "-t", session, "-F", "#{window_name}").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func paneIDs(t *testing.T, client *tmux.Client, session string) []string {
	t.Helper()
	panes, err := client.Panes(session)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, pane := range panes {
		ids = append(ids, pane.ID)
	}
	return ids
}

func managed(t *testing.T, client *tmux.Client, session, kind, id string) []tmux.Pane {
	t.Helper()
	panes, err := client.Panes(session)
	if err != nil {
		t.Fatal(err)
	}
	var out []tmux.Pane
	for _, pane := range panes {
		if pane.Kind == kind && pane.DefinitionID == id {
			out = append(out, pane)
		}
	}
	return out
}

func firstPane(panes []tmux.Pane) tmux.Pane {
	if len(panes) == 0 {
		return tmux.Pane{}
	}
	return panes[0]
}

func sorted(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

func writeJSONIT(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	mustIT(t, err)
	mustIT(t, os.WriteFile(path, data, 0o644))
}

func waitForFile(path string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	return nil, lastErr
}

func mustIT(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
