package discovery

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/shellwords"
	"gopkg.in/yaml.v3"
)

type Discovery struct{}

type script struct {
	Name string
	Body string
}

type packageJSON struct {
	Name           string
	PackageManager string
	Workspaces     []string
	Scripts        []script
}

var lifecycleScripts = map[string]bool{
	"dependencies": true, "install": true, "postinstall": true, "postpack": true,
	"postpublish": true, "postversion": true, "preinstall": true, "prepack": true,
	"prepare": true, "prepublish": true, "prepublishOnly": true, "preversion": true,
	"version": true,
}

func (Discovery) Scripts(root string, exclude []string) []config.Definition {
	root, _ = filepath.Abs(root)
	rootPackage, ok := readPackage(filepath.Join(root, "package.json"))
	if !ok {
		return nil
	}
	manager := packageManager(root, rootPackage)
	var out []config.Definition
	for _, pkg := range packages(root, rootPackage) {
		packageName := pkg.pkg.Name
		if packageName == "" {
			packageName = filepath.Base(pkg.dir)
		}
		relative, err := filepath.Rel(root, pkg.dir)
		if err != nil {
			continue
		}
		prefix := ""
		if relative != "." {
			prefix = packageName
		}
		for _, item := range filteredScripts(pkg.pkg.Scripts) {
			id := item.Name
			if prefix != "" {
				id = prefix + ":" + item.Name
			}
			if excluded(id, exclude) {
				continue
			}
			out = append(out, config.Definition{
				ID:          id,
				Name:        id,
				Description: item.Body,
				Command:     scriptCommand(manager, item.Name),
				Directory:   relative,
				Placement:   config.PlacementWindow,
				ExitMode:    config.ExitKeepOnError,
				Enabled:     true,
				Executable:  manager,
			})
		}
	}
	return out
}

type packageEntry struct {
	dir string
	pkg packageJSON
}

func packages(root string, rootPackage packageJSON) []packageEntry {
	entries := []packageEntry{{dir: root, pkg: rootPackage}}
	seen := map[string]bool{root: true}
	for _, pattern := range workspacePatterns(root, rootPackage) {
		if strings.HasPrefix(pattern, "!") {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, pattern, "package.json"))
		sort.Strings(matches)
		for _, path := range matches {
			abs, err := filepath.Abs(path)
			if err != nil || !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
				continue
			}
			dir := filepath.Dir(abs)
			if seen[dir] {
				continue
			}
			pkg, ok := readPackage(abs)
			if ok {
				entries = append(entries, packageEntry{dir: dir, pkg: pkg})
				seen[dir] = true
			}
		}
	}
	return entries
}

func workspacePatterns(root string, rootPackage packageJSON) []string {
	patterns := append([]string{}, rootPackage.Workspaces...)
	path := filepath.Join(root, "pnpm-workspace.yaml")
	data, err := os.ReadFile(path)
	if err == nil {
		var doc struct {
			Packages []string `yaml:"packages"`
		}
		if yaml.Unmarshal(data, &doc) == nil {
			patterns = append(patterns, doc.Packages...)
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" && !seen[pattern] {
			out = append(out, pattern)
			seen[pattern] = true
		}
	}
	return out
}

func filteredScripts(scripts []script) []script {
	names := make(map[string]bool, len(scripts))
	for _, item := range scripts {
		names[item.Name] = true
	}
	out := make([]script, 0, len(scripts))
	for _, item := range scripts {
		if lifecycleScripts[item.Name] || hookForExistingScript(item.Name, names) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func hookForExistingScript(name string, names map[string]bool) bool {
	for _, prefix := range []string{"pre", "post"} {
		if strings.HasPrefix(name, prefix) && names[strings.TrimPrefix(name, prefix)] {
			return true
		}
	}
	return false
}

func packageManager(root string, pkg packageJSON) string {
	declared := strings.SplitN(pkg.PackageManager, "@", 2)[0]
	switch declared {
	case "pnpm", "yarn", "bun", "npm":
		return declared
	}
	if exists(filepath.Join(root, "pnpm-lock.yaml")) {
		return "pnpm"
	}
	if exists(filepath.Join(root, "yarn.lock")) {
		return "yarn"
	}
	if exists(filepath.Join(root, "bun.lock")) || exists(filepath.Join(root, "bun.lockb")) {
		return "bun"
	}
	return "npm"
}

func scriptCommand(manager, name string) string {
	escaped := shellwords.Escape(name)
	if manager == "npm" {
		return "npm run " + escaped
	}
	return manager + " " + escaped
}

func readPackage(path string) (packageJSON, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return packageJSON{}, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return packageJSON{}, false
	}
	var pkg packageJSON
	_ = json.Unmarshal(raw["name"], &pkg.Name)
	_ = json.Unmarshal(raw["packageManager"], &pkg.PackageManager)
	pkg.Workspaces = parseWorkspaces(raw["workspaces"])
	pkg.Scripts = parseScripts(raw["scripts"])
	return pkg, true
}

func parseWorkspaces(raw json.RawMessage) []string {
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Packages
	}
	return nil
}

func parseScripts(raw json.RawMessage) []script {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil
	}
	var scripts []script
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil
		}
		key, _ := keyToken.(string)
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil
		}
		scripts = append(scripts, script{Name: key, Body: toString(value)})
	}
	return scripts
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}

func excluded(id string, patterns []string) bool {
	for _, pattern := range patterns {
		if fnmatch(pattern, id) {
			return true
		}
	}
	return false
}

func fnmatch(pattern, value string) bool {
	for _, expanded := range expandBraces(pattern) {
		re := globRegexp(expanded)
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

func expandBraces(pattern string) []string {
	start := strings.Index(pattern, "{")
	if start < 0 {
		return []string{pattern}
	}
	end := strings.Index(pattern[start:], "}")
	if end < 0 {
		return []string{pattern}
	}
	end += start
	prefix, suffix := pattern[:start], pattern[end+1:]
	var out []string
	for _, choice := range strings.Split(pattern[start+1:end], ",") {
		for _, rest := range expandBraces(suffix) {
			out = append(out, prefix+choice+rest)
		}
	}
	return out
}

func globRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	inClass := false
	for _, r := range pattern {
		switch {
		case inClass:
			b.WriteRune(r)
			if r == ']' {
				inClass = false
			}
		case r == '*':
			b.WriteString(".*")
		case r == '?':
			b.WriteString(".")
		case r == '[':
			inClass = true
			b.WriteRune(r)
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
