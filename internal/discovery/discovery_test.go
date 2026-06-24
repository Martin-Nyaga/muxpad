package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/Martin-Nyaga/muxpad/internal/config"
)

func TestDiscoversRootAndWorkspaceScriptsWithNoiseFiltering(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	pkg := filepath.Join(root, "packages", "mobile")
	must(t, os.MkdirAll(pkg, 0o755))
	writeJSON(t, filepath.Join(root, "package.json"), map[string]any{
		"name":           "root",
		"packageManager": "pnpm@9.0.0",
		"workspaces":     []string{"packages/*"},
		"scripts": map[string]string{
			"dev":         "vite",
			"predev":      "setup",
			"postinstall": "setup",
			"lint":        "eslint .",
		},
	})
	writeJSON(t, filepath.Join(pkg, "package.json"), map[string]any{
		"name": "app-mobile",
		"scripts": map[string]string{
			"dev":                "expo start",
			"translations:check": "node check.js",
		},
	})

	scripts := Discovery{}.Scripts(root, []string{"app-mobile:translations:*"})
	got := ids(scripts)
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"app-mobile:dev", "dev", "lint"}) {
		t.Fatalf("scripts = %v", got)
	}
	if find(scripts, "dev").Command != "pnpm dev" {
		t.Fatalf("root command = %q", find(scripts, "dev").Command)
	}
	mobile := find(scripts, "app-mobile:dev")
	if mobile.Directory != "packages/mobile" || mobile.Description != "expo start" {
		t.Fatalf("mobile script = %#v", mobile)
	}
}

func TestUsesLockfilesThenNpmFallback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	must(t, os.MkdirAll(root, 0o755))
	writeJSON(t, filepath.Join(root, "package.json"), map[string]any{"scripts": map[string]string{"test": "vitest"}})
	must(t, os.WriteFile(filepath.Join(root, "yarn.lock"), nil, 0o644))
	if got := (Discovery{}).Scripts(root, nil)[0].Command; got != "yarn test" {
		t.Fatalf("with yarn lock = %q", got)
	}
	must(t, os.Remove(filepath.Join(root, "yarn.lock")))
	if got := (Discovery{}).Scripts(root, nil)[0].Command; got != "npm run test" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestInvalidOrMissingPackageFilesProduceNoScripts(t *testing.T) {
	tmp := t.TempDir()
	must(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("not json"), 0o644))
	if got := (Discovery{}).Scripts(tmp, nil); len(got) != 0 {
		t.Fatalf("invalid package scripts = %#v", got)
	}
	if got := (Discovery{}).Scripts(filepath.Join(tmp, "missing"), nil); len(got) != 0 {
		t.Fatalf("missing package scripts = %#v", got)
	}
}

func TestExcludePatternsMatchRubyFnmatchSemantics(t *testing.T) {
	if !excluded("@northstar/web:e2e", []string{"*:e2e"}) {
		t.Fatal("star should match scoped package names containing slash")
	}
	if !excluded("mobile:translations:check", []string{"{mobile,web}:translations:*"}) {
		t.Fatal("brace alternatives should match")
	}
	if excluded("api:test", []string{"{mobile,web}:translations:*"}) {
		t.Fatal("unmatched pattern should not exclude")
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	must(t, err)
	must(t, os.WriteFile(path, data, 0o644))
}

func ids(defs []config.Definition) []string {
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.ID)
	}
	return out
}

func find(defs []config.Definition, id string) config.Definition {
	for _, def := range defs {
		if def.ID == id {
			return def
		}
	}
	return config.Definition{}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
