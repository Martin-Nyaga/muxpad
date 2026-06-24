package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadsMultipleProjectsAndResolvesDeepestRoot(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "child")
	must(t, os.MkdirAll(child, 0o755))
	cfg := loadConfig(t, tmp, `
projects:
  parent:
    root: `+parent+`
    tasks: {}
  child:
    root: `+child+`
    tasks: {}
`)

	if got := projectIDs(cfg.Projects); !reflect.DeepEqual(got, []string{"parent", "child"}) {
		t.Fatalf("project order = %v", got)
	}
	project, ok := cfg.ProjectFor(filepath.Join(child, "nested"))
	if !ok || project.ID != "child" {
		t.Fatalf("ProjectFor child nested = %#v, %v", project, ok)
	}
	if _, ok := cfg.ProjectFor(tmp); ok {
		t.Fatal("ProjectFor outside root returned a project")
	}
}

func TestValidatesDefaultTasksAndDefinitionValues(t *testing.T) {
	tmp := t.TempDir()
	_, err := LoadPath(writeConfig(t, tmp, `
projects:
  broken:
    root: `+tmp+`
    default_tasks: [missing]
    tasks: {}
`))
	if err == nil || !strings.Contains(err.Error(), "unknown default tasks") {
		t.Fatalf("expected unknown default tasks error, got %v", err)
	}

	_, err = LoadPath(writeConfig(t, tmp, `
projects:
  broken:
    root: `+tmp+`
    tasks:
      unnamed:
        command: 'true'
`))
	if err == nil || !strings.Contains(err.Error(), "display name") {
		t.Fatalf("expected display name error, got %v", err)
	}
}

func TestBuiltInAgentsCanBeOverriddenAndDisabled(t *testing.T) {
	tmp := t.TempDir()
	cfg := loadConfig(t, tmp, `
agents:
  codex:
    command: my-codex --fast
  claude:
    disabled: true
`)
	codex, _ := cfg.Agent("codex")
	claude, _ := cfg.Agent("claude")
	if codex.Command != "my-codex --fast" || codex.Executable != "my-codex" {
		t.Fatalf("codex override = %#v", codex)
	}
	if claude.Enabled {
		t.Fatal("claude should be disabled")
	}
	if got := defIDs(cfg.Agents); !reflect.DeepEqual(got, []string{"claude", "codex", "opencode"}) {
		t.Fatalf("agent order = %v", got)
	}
}

func TestAgentDisabledAcceptsYamlTruthyValues(t *testing.T) {
	tmp := t.TempDir()
	cfg := loadConfig(t, tmp, `
agents:
  claude:
    disabled: yes
`)
	claude, _ := cfg.Agent("claude")
	if claude.Enabled {
		t.Fatal("claude should be disabled")
	}
}

func TestLoadsProjectDiscoveryExclusions(t *testing.T) {
	tmp := t.TempDir()
	cfg := loadConfig(t, tmp, `
projects:
  work:
    root: `+tmp+`
    tasks: {}
    discovery:
      exclude:
        - "*:postinstall"
        - "mobile:translations:*"
`)
	project, _ := cfg.Project("work")
	want := []string{"*:postinstall", "mobile:translations:*"}
	if !reflect.DeepEqual(project.DiscoveryExclude, want) {
		t.Fatalf("DiscoveryExclude = %v", project.DiscoveryExclude)
	}
}

func loadConfig(t *testing.T, tmp, content string) *Config {
	t.Helper()
	cfg, err := LoadPath(writeConfig(t, tmp, content))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeConfig(t *testing.T, tmp, content string) string {
	t.Helper()
	path := filepath.Join(tmp, "config.yml")
	must(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func projectIDs(projects []Project) []string {
	out := make([]string, 0, len(projects))
	for _, project := range projects {
		out = append(out, project.ID)
	}
	return out
}

func defIDs(defs []Definition) []string {
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.ID)
	}
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
