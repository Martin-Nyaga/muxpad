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

func TestLoadHerdrMissingAndEmptyConfigAreValid(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := LoadHerdrPath(filepath.Join(tmp, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 0 || len(cfg.Agents) != 0 {
		t.Fatalf("cfg = %#v", cfg)
	}
	path := filepath.Join(tmp, "empty.toml")
	must(t, os.WriteFile(path, []byte("  \n"), 0o644))
	cfg, err = LoadHerdrPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 0 {
		t.Fatalf("projects = %#v", cfg.Projects)
	}
}

func TestLoadHerdrParsesProjectsAndDeclaredTasks(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	must(t, os.MkdirAll(project, 0o755))
	path := filepath.Join(tmp, "config.toml")
	must(t, os.WriteFile(path, []byte(`
[projects.work]
name = "Work"
root = "`+project+`"
default_tasks = ["web"]

[projects.work.discovery]
exclude = ["*:postinstall"]

[projects.work.tasks.web]
name = "Web"
description = "web dev server"
command = "pnpm dev"

[projects.work.tasks.api]
name = "API"
description = "api dev server"
command = "pnpm api"
directory = "services/api"
placement = "vertical"
exit_mode = "keep"
`), 0o644))
	cfg, err := LoadHerdrPath(path)
	if err != nil {
		t.Fatal(err)
	}
	projectCfg, ok := cfg.Project("work")
	if !ok {
		t.Fatalf("projects = %#v", cfg.Projects)
	}
	resolvedProject, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if projectCfg.Name != "Work" || projectCfg.Root != resolvedProject || !reflect.DeepEqual(projectCfg.DefaultTasks, []string{"web"}) {
		t.Fatalf("project = %#v", projectCfg)
	}
	if !reflect.DeepEqual(projectCfg.DiscoveryExclude, []string{"*:postinstall"}) {
		t.Fatalf("exclude = %#v", projectCfg.DiscoveryExclude)
	}
	if got := defIDs(projectCfg.Tasks); !reflect.DeepEqual(got, []string{"web", "api"}) {
		t.Fatalf("task order = %v", got)
	}
	api, _ := projectCfg.Task("api")
	if api.Directory != "services/api" || api.Placement != PlacementVertical || api.ExitMode != ExitKeep {
		t.Fatalf("api = %#v", api)
	}
	web, _ := projectCfg.Task("web")
	if web.Placement != PlacementWindow || web.ExitMode != ExitKeepOnError {
		t.Fatalf("web defaults = %#v", web)
	}
	if len(cfg.Agents) != 0 {
		t.Fatalf("herdr config should not load tmux agents: %#v", cfg.Agents)
	}
}

func TestLoadHerdrValidatesProjectAndTaskValues(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	must(t, os.MkdirAll(project, 0o755))
	absoluteTaskDir := filepath.Join(tmp, "task")

	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "invalid project id",
			content: `
[projects."bad project"]
root = "` + project + `"
`,
			want: "invalid project identifier",
		},
		{
			name: "missing project root",
			content: `
[projects.work]
`,
			want: `project "work" requires root`,
		},
		{
			name: "invalid task id",
			content: `
[projects.work]
root = "` + project + `"

[projects.work.tasks."bad task"]
name = "Bad"
description = "bad task"
command = "true"
`,
			want: "invalid task identifier",
		},
		{
			name: "missing task command",
			content: `
[projects.work]
root = "` + project + `"

[projects.work.tasks.web]
name = "Web"
description = "web task"
`,
			want: "requires command",
		},
		{
			name: "missing task name",
			content: `
[projects.work]
root = "` + project + `"

[projects.work.tasks.web]
description = "web task"
command = "true"
`,
			want: "requires a display name",
		},
		{
			name: "missing task description",
			content: `
[projects.work]
root = "` + project + `"

[projects.work.tasks.web]
name = "Web"
command = "true"
`,
			want: "requires a description",
		},
		{
			name: "absolute task directory",
			content: `
[projects.work]
root = "` + project + `"

[projects.work.tasks.web]
name = "Web"
description = "web task"
command = "true"
directory = "` + absoluteTaskDir + `"
`,
			want: "directory must be relative",
		},
		{
			name: "invalid placement",
			content: `
[projects.work]
root = "` + project + `"

[projects.work.tasks.web]
name = "Web"
description = "web task"
command = "true"
placement = "floating"
`,
			want: "invalid placement",
		},
		{
			name: "invalid exit mode",
			content: `
[projects.work]
root = "` + project + `"

[projects.work.tasks.web]
name = "Web"
description = "web task"
command = "true"
exit_mode = "detach"
`,
			want: "invalid exit mode",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadHerdrPath(writeHerdrConfig(t, tmp, tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
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

func writeHerdrConfig(t *testing.T, tmp, content string) string {
	t.Helper()
	path := filepath.Join(tmp, strings.ReplaceAll(t.Name(), "/", "_")+".toml")
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
