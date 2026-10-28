package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Martin-Nyaga/muxpad/internal/backend"
	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/palette"
)

func TestDeclaredTaskMenuDiscoversAndLaunchesWithoutRegisteredProject(t *testing.T) {
	for _, mode := range []string{"missing config", "empty config", "unrelated project"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			mobile := filepath.Join(root, "packages", "mobile")
			if err := os.MkdirAll(mobile, 0o755); err != nil {
				t.Fatal(err)
			}
			for path, contents := range map[string]string{
				filepath.Join(root, "package.json"):   `{"name":"demo","scripts":{"dev":"echo dev"},"workspaces":["packages/*"]}`,
				filepath.Join(mobile, "package.json"): `{"name":"mobile","scripts":{"test":"echo test"}}`,
			} {
				if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), "config.toml")
			if mode == "empty config" {
				if err := os.WriteFile(path, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := config.LoadHerdrPath(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "unrelated project" {
				cfg.Projects = []config.Project{{ID: "other", Root: t.TempDir()}}
			}
			fake := &fakeTmux{inside: true, root: root}
			app := New(cfg, fake)
			pal := &recordingPalette{selectResult: palette.Selection{Action: "enter", Token: "script:mobile:test"}, selectOK: true}
			app.Palette = pal

			if err := app.DeclaredTaskMenu(); err != nil {
				t.Fatal(err)
			}
			if len(pal.items) != 2 || findItem(pal.items, "script:dev", "Discovered scripts").Directory != root {
				t.Fatalf("root script missing or incorrect: %#v", pal.items)
			}
			if got := findItem(pal.items, "script:mobile:test", "Discovered scripts"); got.Directory != mobile {
				t.Fatalf("workspace script = %#v", got)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("calls = %#v", fake.calls)
			}
			launch := fake.calls[0].(backend.LaunchSpec)
			if launch.Kind != "script" || launch.Root != root || launch.Definition.Directory != "packages/mobile" || launch.Definition.Command != "npm run test" || launch.Target != "%9" {
				t.Fatalf("launch = %#v", launch)
			}
		})
	}
}

func TestDeclaredTaskMenuOpensEmptyPaletteWithoutPackageScripts(t *testing.T) {
	app := New(&config.Config{}, &fakeTmux{inside: true, root: t.TempDir()})
	pal := &recordingPalette{}
	app.Palette = pal
	if err := app.DeclaredTaskMenu(); err != nil {
		t.Fatal(err)
	}
	if len(pal.items) != 0 || len(pal.sectionOrder) == 0 {
		t.Fatalf("palette did not open empty: %#v", pal)
	}
}
