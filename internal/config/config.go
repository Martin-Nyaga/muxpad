package config

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/shellwords"
	"gopkg.in/yaml.v3"
)

const DefaultPath = "~/.config/muxpad/config.yml"

type Placement string

const (
	PlacementWindow     Placement = "window"
	PlacementVertical   Placement = "vertical"
	PlacementHorizontal Placement = "horizontal"
)

var Placements = []Placement{PlacementWindow, PlacementVertical, PlacementHorizontal}

type ExitMode string

const (
	ExitClose       ExitMode = "close"
	ExitKeep        ExitMode = "keep"
	ExitKeepOnError ExitMode = "keep-on-error"
)

type Definition struct {
	ID          string
	Name        string
	Description string
	Command     string
	Directory   string
	Placement   Placement
	ExitMode    ExitMode
	Enabled     bool
	Executable  string
}

type Project struct {
	ID               string
	Name             string
	Root             string
	Tasks            []Definition
	DefaultTasks     []string
	DiscoveryExclude []string
}

func (p Project) Task(id string) (Definition, bool) {
	for _, task := range p.Tasks {
		if task.ID == id {
			return task, true
		}
	}
	return Definition{}, false
}

type Config struct {
	Projects []Project
	Agents   []Definition
}

func Load() (*Config, error) {
	path := os.Getenv("MUXPAD_CONFIG")
	if path == "" {
		path = DefaultPath
	}
	return LoadPath(path)
}

func LoadPath(path string) (*Config, error) {
	expanded := canonicalPath(path)
	data, err := os.ReadFile(expanded)
	if errors.Is(err, os.ErrNotExist) {
		data = []byte("{}")
	} else if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		data = []byte("{}")
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	node := documentValue(&root)
	if node == nil || node.Kind == 0 || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
		node = &yaml.Node{Kind: yaml.MappingNode}
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config must contain a YAML mapping: %s", path)
	}
	projectsNode := mappingLookup(node, "projects")
	agentsNode := mappingLookup(node, "agents")
	projects, err := parseProjects(projectsNode, path)
	if err != nil {
		return nil, err
	}
	agents, err := parseAgents(agentsNode, path)
	if err != nil {
		return nil, err
	}
	return &Config{Projects: projects, Agents: agents}, nil
}

func (c *Config) Project(id string) (Project, bool) {
	if c == nil {
		return Project{}, false
	}
	for _, project := range c.Projects {
		if project.ID == id {
			return project, true
		}
	}
	return Project{}, false
}

func (c *Config) Agent(id string) (Definition, bool) {
	if c == nil {
		return Definition{}, false
	}
	for _, agent := range c.Agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return Definition{}, false
}

func (c *Config) ProjectFor(path string) (Project, bool) {
	expanded := canonicalPath(path)
	var found Project
	ok := false
	for _, project := range c.Projects {
		root := canonicalPath(project.Root)
		if within(expanded, root) && (!ok || len(root) > len(found.Root)) {
			found = project
			ok = true
		}
	}
	return found, ok
}

type entry struct {
	id   string
	node *yaml.Node
}

var idPattern = regexp.MustCompile(`\A[a-zA-Z0-9_-]+\z`)

func parseProjects(node *yaml.Node, path string) ([]Project, error) {
	entries, err := normalizeEntries(node, "projects", path)
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(entries))
	for _, item := range entries {
		id := item.id
		attrs := item.node
		if !idPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid project identifier: %q", id)
		}
		root := stringValue(mappingLookup(attrs, "root"))
		if root == "" {
			return nil, fmt.Errorf("project %q requires root", id)
		}
		root = canonicalPath(root)
		tasks, err := parseTasks(mappingLookup(attrs, "tasks"), id)
		if err != nil {
			return nil, err
		}
		defaults := stringArray(mappingLookup(attrs, "default_tasks"))
		for _, defaultID := range defaults {
			if _, ok := taskByID(tasks, defaultID); !ok {
				return nil, fmt.Errorf("project %s has unknown default tasks: %s", id, defaultID)
			}
		}
		discovery := mappingLookup(attrs, "discovery")
		if discovery != nil && discovery.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("project %s discovery must be a mapping", id)
		}
		excludes := stringArray(mappingLookup(discovery, "exclude"))
		name := stringValue(mappingLookup(attrs, "name"))
		if name == "" {
			name = id
		}
		projects = append(projects, Project{
			ID:               id,
			Name:             name,
			Root:             root,
			Tasks:            tasks,
			DefaultTasks:     defaults,
			DiscoveryExclude: excludes,
		})
	}
	return projects, nil
}

func parseTasks(node *yaml.Node, projectID string) ([]Definition, error) {
	entries, err := normalizeEntries(node, "tasks for "+projectID, "")
	if err != nil {
		return nil, err
	}
	tasks := make([]Definition, 0, len(entries))
	for _, item := range entries {
		id := item.id
		attrs := item.node
		if !idPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid task identifier: %q", id)
		}
		command := stringValue(mappingLookup(attrs, "command"))
		if command == "" {
			return nil, fmt.Errorf("task %s/%s requires command", projectID, id)
		}
		if stringValue(mappingLookup(attrs, "name")) == "" {
			return nil, fmt.Errorf("task %s/%s requires a display name", projectID, id)
		}
		if stringValue(mappingLookup(attrs, "description")) == "" {
			return nil, fmt.Errorf("task %s/%s requires a description", projectID, id)
		}
		directory := stringValue(mappingLookup(attrs, "directory"))
		if directory != "" && filepath.IsAbs(directory) {
			return nil, fmt.Errorf("task %s/%s directory must be relative", projectID, id)
		}
		def, err := definition(id, attrs, command, ExitKeepOnError)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, def)
	}
	return tasks, nil
}

func parseAgents(node *yaml.Node, path string) ([]Definition, error) {
	overrides, err := normalizeEntries(node, "agents", path)
	if err != nil {
		return nil, err
	}
	overrideMap := map[string]*yaml.Node{}
	for _, entry := range overrides {
		overrideMap[entry.id] = entry.node
	}
	for id := range overrideMap {
		if id != "claude" && id != "codex" && id != "opencode" {
			return nil, fmt.Errorf("unknown agent overrides: %s", id)
		}
	}
	defaults := []Definition{
		{ID: "claude", Name: "claude", Description: "anthropic coding agent", Command: "claude", Executable: "claude", Placement: PlacementWindow, ExitMode: ExitClose, Enabled: true},
		{ID: "codex", Name: "codex", Description: "openai coding agent", Command: "codex", Executable: "codex", Placement: PlacementWindow, ExitMode: ExitClose, Enabled: true},
		{ID: "opencode", Name: "opencode", Description: "open-source coding agent", Command: "opencode", Executable: "opencode", Placement: PlacementWindow, ExitMode: ExitClose, Enabled: true},
	}
	agents := make([]Definition, 0, len(defaults))
	for _, base := range defaults {
		override := overrideMap[base.ID]
		attrs := definitionNode(base, override)
		command := stringValue(mappingLookup(attrs, "command"))
		def, err := definition(base.ID, attrs, command, ExitClose)
		if err != nil {
			return nil, err
		}
		agents = append(agents, def)
	}
	return agents, nil
}

func definition(id string, attrs *yaml.Node, command string, defaultExit ExitMode) (Definition, error) {
	if command == "" {
		return Definition{}, fmt.Errorf("%s command must be a non-empty string", id)
	}
	placement := Placement(stringDefault(mappingLookup(attrs, "placement"), string(PlacementWindow)))
	if !validPlacement(placement) {
		return Definition{}, fmt.Errorf("invalid placement %q for %s", placement, id)
	}
	exitMode := ExitMode(stringDefault(mappingLookup(attrs, "exit_mode"), string(defaultExit)))
	if !validExitMode(exitMode) {
		return Definition{}, fmt.Errorf("invalid exit mode %q for %s", exitMode, id)
	}
	name := stringDefault(mappingLookup(attrs, "name"), id)
	description := stringValue(mappingLookup(attrs, "description"))
	enabled := true
	if enabledNode := mappingLookup(attrs, "enabled"); enabledNode != nil {
		enabled = boolValue(enabledNode)
	}
	if disabledNode := mappingLookup(attrs, "disabled"); disabledNode != nil && mappingLookup(attrs, "enabled") == nil {
		enabled = !boolValue(disabledNode)
	}
	executable := stringValue(mappingLookup(attrs, "executable"))
	if executable == "" {
		parts, _ := shellwords.Split(command)
		if len(parts) > 0 {
			executable = parts[0]
		}
	}
	return Definition{
		ID:          id,
		Name:        name,
		Description: description,
		Command:     command,
		Directory:   stringValue(mappingLookup(attrs, "directory")),
		Placement:   placement,
		ExitMode:    exitMode,
		Enabled:     enabled,
		Executable:  executable,
	}, nil
}

func normalizeEntries(node *yaml.Node, label, path string) ([]entry, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		out := make([]entry, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i+1].Kind != yaml.MappingNode {
				return nil, fmt.Errorf("%s entry %q must be a mapping in %s", label, node.Content[i].Value, path)
			}
			out = append(out, entry{id: node.Content[i].Value, node: node.Content[i+1]})
		}
		return out, nil
	case yaml.SequenceNode:
		out := make([]entry, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("%s entries must be mappings in %s", label, path)
			}
			idNode := mappingLookup(item, "id")
			id := stringValue(idNode)
			if id == "" {
				return nil, fmt.Errorf("%s entry requires id in %s", label, path)
			}
			out = append(out, entry{id: id, node: item})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be a mapping or list in %s", label, path)
	}
}

func documentValue(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	return node
}

func mappingLookup(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func stringValue(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	return node.Value
}

func stringDefault(node *yaml.Node, fallback string) string {
	if value := stringValue(node); value != "" {
		return value
	}
	return fallback
}

func stringArray(node *yaml.Node) []string {
	if node == nil || node.Kind == 0 {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return []string{node.Value}
	}
	out := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		out = append(out, item.Value)
	}
	return out
}

func boolValue(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	return strings.EqualFold(node.Value, "true")
}

func definitionNode(base Definition, override *yaml.Node) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k, v string) {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &yaml.Node{Kind: yaml.ScalarNode, Value: v})
	}
	add("name", base.Name)
	add("description", base.Description)
	add("command", base.Command)
	add("executable", base.Executable)
	add("placement", string(base.Placement))
	add("exit_mode", string(base.ExitMode))
	add("enabled", "true")
	if override != nil {
		hasEnabledOverride := mappingLookup(override, "enabled") != nil
		for i := 0; i+1 < len(override.Content); i += 2 {
			key := override.Content[i].Value
			if key == "command" && mappingLookup(override, "executable") == nil {
				parts, _ := shellwords.Split(override.Content[i+1].Value)
				if len(parts) > 0 {
					setMapping(node, "executable", parts[0])
				}
			}
			if key == "disabled" && !hasEnabledOverride {
				setMapping(node, "enabled", fmt.Sprintf("%t", !boolValue(override.Content[i+1])))
			}
			setMappingNode(node, key, override.Content[i+1])
		}
	}
	return node
}

func setMapping(node *yaml.Node, key, value string) {
	setMappingNode(node, key, &yaml.Node{Kind: yaml.ScalarNode, Value: value})
}

func setMappingNode(node *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1] = value
			return
		}
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}

func taskByID(tasks []Definition, id string) (Definition, bool) {
	for _, task := range tasks {
		if task.ID == id {
			return task, true
		}
	}
	return Definition{}, false
}

func validPlacement(value Placement) bool {
	for _, placement := range Placements {
		if value == placement {
			return true
		}
	}
	return false
}

func validExitMode(value ExitMode) bool {
	return value == ExitClose || value == ExitKeep || value == ExitKeepOnError
}

func within(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(os.PathSeparator))
}

func expandPath(path string) string {
	if path == "" || path[0] != '~' {
		expanded, _ := filepath.Abs(path)
		return expanded
	}
	home := ""
	if current, err := user.Current(); err == nil {
		home = current.HomeDir
	}
	if home == "" {
		home = os.Getenv("HOME")
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func canonicalPath(path string) string {
	expanded := expandPath(path)
	if resolved, err := filepath.EvalSymlinks(expanded); err == nil {
		return resolved
	}
	var suffix []string
	current := expanded
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return expanded
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			parts := append([]string{resolved}, suffix...)
			return filepath.Join(parts...)
		}
	}
}
