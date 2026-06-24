package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/agent"
	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/discovery"
	"github.com/Martin-Nyaga/muxpad/internal/palette"
	"github.com/Martin-Nyaga/muxpad/internal/shellwords"
	"github.com/Martin-Nyaga/muxpad/internal/tmux"
)

var SectionOrder = []string{"Tasks", "Agents", "Discovered scripts"}

type Tmux interface {
	Inside() bool
	CurrentSession() (string, error)
	CurrentPane() (string, error)
	SessionExists(string) bool
	Sessions() []string
	CreateSession(name, root, projectID string) (string, error)
	ProjectContext(session string) string
	SessionRoot(session string) string
	ManagedRoot(session string) string
	Panes(session string) ([]tmux.Pane, error)
	Launch(tmux.LaunchSpec) (string, error)
	Focus(tmux.Pane) error
	Restart(tmux.Pane, config.Definition) error
	Attach(session string) error
	Switch(session string) error
	PopupMenu(program string) error
	KillSession(session string) error
}

type Palette interface {
	Select(items []palette.Item, sectionOrder []string) (palette.Selection, bool, error)
	Choose(options []palette.Option, title string) (string, bool, error)
}

type Discoverer interface {
	Scripts(root string, exclude []string) []config.Definition
}

type AgentDetector interface {
	Detect(panes []agent.Pane) map[string]string
}

type Application struct {
	Config         *config.Config
	Tmux           Tmux
	Discovery      Discoverer
	AgentDiscovery AgentDetector
	Palette        Palette
	Input          io.Reader
	Output         io.Writer
}

func New() (*Application, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &Application{
		Config:         cfg,
		Tmux:           tmux.New(),
		Discovery:      discovery.Discovery{},
		AgentDiscovery: agent.Discovery{},
		Palette:        palette.New(),
		Input:          os.Stdin,
		Output:         os.Stdout,
	}, nil
}

func (a *Application) Start(projectID string, empty, attach bool) (string, error) {
	project, hasProject := config.Project{}, false
	if projectID != "" {
		var ok bool
		project, ok = a.Config.Project(projectID)
		if !ok {
			return "", fmt.Errorf("unknown project: %s", projectID)
		}
		hasProject = true
	} else {
		cwd, _ := os.Getwd()
		project, hasProject = a.Config.ProjectFor(cwd)
	}
	session := a.sessionName(project, hasProject)
	if a.Tmux.Inside() {
		current, err := a.Tmux.CurrentSession()
		if err != nil {
			return "", err
		}
		if current == session {
			return session, nil
		}
		if !a.confirmSwitch(session) {
			return session, nil
		}
		if _, _, err := a.ensureSession(project, hasProject, !empty); err != nil {
			return "", err
		}
		return session, a.Tmux.Switch(session)
	}
	_, created, err := a.ensureSession(project, hasProject, !empty)
	if err != nil {
		return "", err
	}
	if created {
		_ = a.focusShell(session)
	}
	if attach {
		return session, a.Tmux.Attach(session)
	}
	return session, nil
}

func (a *Application) Menu(attach bool) (string, error) {
	session, created, err := a.sessionForCommand()
	if err != nil {
		return "", err
	}
	items, err := a.PaletteItems(session)
	if err != nil {
		return "", err
	}
	selection, ok, err := a.Palette.Select(items, SectionOrder)
	if err != nil {
		return "", err
	}
	if !ok {
		if created {
			_ = a.Tmux.KillSession(session)
		}
		return session, nil
	}
	if err := a.HandleSelection(session, selection); err != nil {
		return "", err
	}
	if attach && !a.Tmux.Inside() {
		return session, a.Tmux.Attach(session)
	}
	return session, nil
}

func (a *Application) Task(id string, placement config.Placement, attach bool) error {
	session, _, err := a.sessionForCommand()
	if err != nil {
		return err
	}
	project, ok := a.projectForSession(session)
	if !ok {
		return fmt.Errorf("session %q has no Muxpad project context", session)
	}
	if err := a.launchTask(session, project, id, placement); err != nil {
		return err
	}
	if attach && !a.Tmux.Inside() {
		return a.Tmux.Attach(session)
	}
	return nil
}

func (a *Application) Agent(id string, placement config.Placement, attach bool) error {
	session, _, err := a.sessionForCommand()
	if err != nil {
		return err
	}
	if err := a.launchAgent(session, id, placement); err != nil {
		return err
	}
	if attach && !a.Tmux.Inside() {
		return a.Tmux.Attach(session)
	}
	return nil
}

func (a *Application) ensureSession(project config.Project, hasProject, launchDefaults bool) (string, bool, error) {
	root := ""
	projectID := ""
	if hasProject {
		root = project.Root
		projectID = project.ID
	} else {
		root, _ = os.Getwd()
	}
	name := a.sessionName(project, hasProject)
	if a.Tmux.SessionExists(name) {
		return name, false, nil
	}
	if _, err := a.Tmux.CreateSession(name, root, projectID); err != nil {
		return "", false, err
	}
	if hasProject && launchDefaults {
		for _, id := range project.DefaultTasks {
			if err := a.launchTask(name, project, id, ""); err != nil {
				return "", false, err
			}
		}
	}
	return name, true, nil
}

func (a *Application) sessionName(project config.Project, hasProject bool) string {
	if hasProject {
		return project.ID
	}
	cwd, _ := os.Getwd()
	return a.adhocName(cwd)
}

func (a *Application) adhocName(path string) string {
	root, _ := filepath.Abs(path)
	for _, session := range a.Tmux.Sessions() {
		if a.Tmux.ProjectContext(session) == "" && a.Tmux.ManagedRoot(session) == root {
			return session
		}
	}
	return a.availableName(adhocBase(root))
}

func adhocBase(root string) string {
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	base := strings.Trim(re.ReplaceAllString(strings.ToLower(filepath.Base(root)), "-"), "-")
	if base == "" {
		return "session"
	}
	return base
}

func (a *Application) availableName(base string) string {
	if !a.Tmux.SessionExists(base) {
		return base
	}
	for i := 2; ; i++ {
		name := base + "-" + strconv.Itoa(i)
		if !a.Tmux.SessionExists(name) {
			return name
		}
	}
}

func (a *Application) sessionForCommand() (string, bool, error) {
	if a.Tmux.Inside() {
		session, err := a.Tmux.CurrentSession()
		return session, false, err
	}
	cwd, _ := os.Getwd()
	project, hasProject := a.Config.ProjectFor(cwd)
	return a.ensureSession(project, hasProject, true)
}

func (a *Application) projectForSession(session string) (config.Project, bool) {
	id := a.Tmux.ProjectContext(session)
	if id == "" {
		return config.Project{}, false
	}
	return a.Config.Project(id)
}

func (a *Application) launchTask(session string, project config.Project, id string, placement config.Placement) error {
	definition, ok := project.Task(id)
	if !ok {
		return fmt.Errorf("unknown task %q for %s", id, project.Name)
	}
	panes, err := a.Tmux.Panes(session)
	if err != nil {
		return err
	}
	for _, pane := range panes {
		if pane.Kind == "task" && pane.DefinitionID == id {
			return a.Tmux.Focus(pane)
		}
	}
	if placement == "" {
		placement = definition.Placement
	}
	_, err = a.Tmux.Launch(tmux.LaunchSpec{
		Session:    session,
		Definition: definition,
		Kind:       "task",
		Name:       definition.Name,
		Root:       project.Root,
		Placement:  placement,
		Target:     a.launchTarget(session),
	})
	return err
}

func (a *Application) launchAgent(session, id string, placement config.Placement) error {
	definition, ok := a.Config.Agent(id)
	if !ok {
		return fmt.Errorf("unknown agent: %s", id)
	}
	if !definition.Enabled {
		return fmt.Errorf("%s is disabled", definition.Name)
	}
	if !executable(definition.Executable) {
		return fmt.Errorf("%s is unavailable: missing executable %s", definition.Name, definition.Executable)
	}
	panes, err := a.Tmux.Panes(session)
	if err != nil {
		return err
	}
	used := map[string]bool{}
	for _, pane := range panes {
		if pane.Kind == "agent" && pane.DefinitionID == id {
			used[pane.Name] = true
		}
	}
	instance := 1
	for used[agentInstanceName(definition.Name, instance)] {
		instance++
	}
	name := agentInstanceName(definition.Name, instance)
	root := a.Tmux.SessionRoot(session)
	if placement == "" {
		placement = definition.Placement
	}
	_, err = a.Tmux.Launch(tmux.LaunchSpec{
		Session:    session,
		Definition: agentLaunchDefinition(definition),
		Kind:       "agent",
		Name:       name,
		Root:       root,
		Placement:  placement,
		Target:     a.launchTarget(session),
	})
	return err
}

func (a *Application) launchTarget(session string) string {
	if a.Tmux.Inside() {
		current, _ := a.Tmux.CurrentSession()
		if current == session {
			pane, _ := a.Tmux.CurrentPane()
			return pane
		}
	}
	return session + ":shell"
}

func (a *Application) focusShell(session string) error {
	panes, err := a.Tmux.Panes(session)
	if err != nil {
		return err
	}
	for _, pane := range panes {
		if pane.Name == "" && pane.Window != "" {
			return a.Tmux.Focus(pane)
		}
	}
	return nil
}

func (a *Application) PaletteItems(session string) ([]palette.Item, error) {
	project, hasProject := a.projectForSession(session)
	panes, err := a.Tmux.Panes(session)
	if err != nil {
		return nil, err
	}
	scripts := a.discoveredScripts(session, project, hasProject)
	root := a.Tmux.SessionRoot(session)
	if hasProject {
		root = project.Root
	}
	var items []palette.Item
	if hasProject {
		for _, task := range project.Tasks {
			pane, ok := findManaged(panes, "task", task.ID)
			items = append(items, launchableItem("task:"+task.ID, "Tasks", task, pane, ok, root))
			if ok {
				items = append(items, a.instanceItem("task:"+task.ID, task, pane))
			}
		}
	}
	for _, agent := range a.Config.Agents {
		items = append(items, agentItem(agent, root))
	}
	for _, pane := range panes {
		if pane.Kind == "agent" && !pane.Done() {
			items = append(items, a.runningItem(pane, "window "+pane.WindowIndex+" · "+pane.CurrentCommand))
		}
	}
	items = append(items, a.discoveredAgentItems(panes)...)
	for _, pane := range panes {
		if pane.Kind == "script" && !pane.Done() && findDefinition(scripts, pane.DefinitionID).ID == "" {
			items = append(items, a.runningItem(pane, "removed package script · window "+pane.WindowIndex))
		}
	}
	for _, script := range scripts {
		pane, ok := findManaged(panes, "script", script.ID)
		items = append(items, launchableItem("script:"+script.ID, "Discovered scripts", script, pane, ok, root))
		if ok {
			items = append(items, a.instanceItem("script:"+script.ID, script, pane))
		}
	}
	return items, nil
}

func (a *Application) instanceItem(token string, definition config.Definition, pane tmux.Pane) palette.Item {
	if !pane.Done() {
		return a.runningItem(pane, "window "+pane.WindowIndex+" · "+pane.CurrentCommand)
	}
	return palette.Item{
		Token:       token,
		Section:     palette.RunningSection,
		Name:        pane.Name,
		Description: definition.Description,
		Command:     definition.Command,
		State:       "finished",
		StateKind:   palette.StateFinished,
		Summary:     AgentSummary(pane),
	}
}

func launchableItem(token, section string, definition config.Definition, pane tmux.Pane, hasPane bool, root string) palette.Item {
	state := "not running"
	kind := palette.StateIdle
	if hasPane {
		if pane.Done() {
			state = "finished"
			kind = palette.StateFinished
		} else {
			state = "running"
			kind = palette.StateRunning
		}
	}
	return palette.Item{
		Token:       token,
		Section:     section,
		Name:        definition.Name,
		Description: definition.Description,
		Command:     definition.Command,
		Directory:   resolveDirectory(definition, root),
		State:       state,
		StateKind:   kind,
	}
}

func agentItem(agent config.Definition, root string) palette.Item {
	available := agent.Enabled && executable(agent.Executable)
	state := "available"
	kind := palette.StateAvailable
	if !agent.Enabled {
		state = "disabled"
		kind = palette.StateDisabled
	} else if !available {
		state = "unavailable: missing " + agent.Executable
		kind = palette.StateUnavailable
	}
	return palette.Item{
		Token:       "agent:" + agent.ID,
		Section:     "Agents",
		Name:        agent.Name,
		Description: agent.Description,
		Command:     agent.Command,
		Directory:   root,
		State:       state,
		StateKind:   kind,
	}
}

func (a *Application) runningItem(pane tmux.Pane, description string) palette.Item {
	return palette.Item{
		Token:       "running:" + pane.ID,
		Section:     palette.RunningSection,
		Name:        pane.Name,
		Description: description,
		Command:     pane.CurrentCommand,
		State:       "running",
		StateKind:   palette.StateRunning,
		Summary:     AgentSummary(pane),
	}
}

func (a *Application) HandleSelection(session string, selection palette.Selection) error {
	parts := strings.SplitN(selection.Token, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid selection token: %s", selection.Token)
	}
	kind, id := parts[0], parts[1]
	panes, err := a.Tmux.Panes(session)
	if err != nil {
		return err
	}
	if kind == "running" {
		for _, pane := range panes {
			if pane.ID == id {
				return a.Tmux.Focus(pane)
			}
		}
		return fmt.Errorf("that agent instance is no longer running")
	}
	action := selection.Action
	if action == "tab" {
		chosen, ok, err := a.actionMenu(session, kind, id)
		if err != nil || !ok {
			return err
		}
		action = chosen
	}
	placement := config.Placement("")
	for _, candidate := range config.Placements {
		if action == string(candidate) {
			placement = candidate
		}
	}
	switch kind {
	case "task":
		project, ok := a.projectForSession(session)
		if !ok {
			return fmt.Errorf("session %q has no Muxpad project context", session)
		}
		definition, _ := project.Task(id)
		pane, hasPane := findManaged(panes, "task", id)
		if (action == "ctrl-r" || action == "restart") && hasPane {
			return a.Tmux.Restart(pane, definition)
		}
		return a.launchTask(session, project, id, placement)
	case "script":
		project, hasProject := a.projectForSession(session)
		definition := findDefinition(a.discoveredScripts(session, project, hasProject), id)
		if definition.ID == "" {
			return fmt.Errorf("package script %q is no longer available", id)
		}
		pane, hasPane := findManaged(panes, "script", id)
		if (action == "ctrl-r" || action == "restart") && hasPane {
			return a.Tmux.Restart(pane, definition)
		}
		return a.launchScript(session, definition, placement)
	case "agent":
		if action == "ctrl-r" || action == "restart" {
			return fmt.Errorf("restart applies only to tasks and package scripts")
		}
		return a.launchAgent(session, id, placement)
	default:
		return fmt.Errorf("unknown selection kind: %s", kind)
	}
}

func (a *Application) actionMenu(session, kind, id string) (string, bool, error) {
	options := []palette.Option{
		{Token: "window", Label: "New window"},
		{Token: "vertical", Label: "Vertical split"},
		{Token: "horizontal", Label: "Horizontal split"},
	}
	if kind == "task" || kind == "script" {
		panes, _ := a.Tmux.Panes(session)
		if pane, ok := findManaged(panes, kind, id); ok && pane.Done() {
			options = append(options, palette.Option{Token: "restart", Label: "Restart in existing pane"})
		}
	}
	return a.Palette.Choose(options, "Choose placement or action")
}

func (a *Application) discoveredScripts(session string, project config.Project, hasProject bool) []config.Definition {
	root := a.Tmux.SessionRoot(session)
	excludes := []string(nil)
	if hasProject {
		root = project.Root
		excludes = project.DiscoveryExclude
	}
	scripts := a.Discovery.Scripts(root, excludes)
	if !hasProject {
		return scripts
	}
	var out []config.Definition
	for _, script := range scripts {
		dir := root
		if script.Directory != "" {
			dir = filepath.Join(root, script.Directory)
		}
		duplicate := false
		for _, task := range project.Tasks {
			taskDir := root
			if task.Directory != "" {
				taskDir = filepath.Join(root, task.Directory)
			}
			if taskDir == dir && task.Command == script.Command {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, script)
		}
	}
	return out
}

func (a *Application) launchScript(session string, definition config.Definition, placement config.Placement) error {
	panes, err := a.Tmux.Panes(session)
	if err != nil {
		return err
	}
	for _, pane := range panes {
		if pane.Kind == "script" && pane.DefinitionID == definition.ID {
			return a.Tmux.Focus(pane)
		}
	}
	root := a.Tmux.SessionRoot(session)
	if placement == "" {
		placement = definition.Placement
	}
	_, err = a.Tmux.Launch(tmux.LaunchSpec{
		Session:    session,
		Definition: definition,
		Kind:       "script",
		Name:       definition.Name,
		Root:       root,
		Placement:  placement,
		Target:     a.launchTarget(session),
	})
	return err
}

func resolveDirectory(definition config.Definition, root string) string {
	if definition.Directory != "" {
		return filepath.Join(root, definition.Directory)
	}
	return root
}

func executable(name string) bool {
	if name == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func agentInstanceName(name string, instance int) string {
	if instance == 1 {
		return name
	}
	return fmt.Sprintf("%s %d", name, instance)
}

func agentLaunchDefinition(definition config.Definition) config.Definition {
	if definition.ID != "codex" {
		return definition
	}
	leading := len(definition.Command) - len(strings.TrimLeft(definition.Command, " \t"))
	rest := definition.Command[leading:]
	tokenEnd := strings.IndexAny(rest, " \t")
	if tokenEnd < 0 {
		tokenEnd = len(rest)
	}
	executable := rest[:tokenEnd]
	if filepath.Base(executable) == "codex" {
		definition.Command = definition.Command[:leading+tokenEnd] +
			" -c " + shellwords.Escape(`tui.terminal_title=["thread"]`) +
			rest[tokenEnd:]
	}
	return definition
}

func AgentSummary(pane tmux.Pane) string {
	if pane.DefinitionID != "claude" && pane.DefinitionID != "codex" {
		return ""
	}
	title := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(pane.Title, " "))
	if pane.DefinitionID == "claude" {
		title = strings.TrimPrefix(title, "✳")
		if strings.HasPrefix(pane.Title, "✳") || strings.HasPrefix(strings.TrimSpace(pane.Title), "✳") {
			title = "* " + strings.TrimSpace(title)
		}
	}
	if title == "" {
		return ""
	}
	generic := []string{pane.Name, pane.DefinitionID, "Claude Code", "Codex", "New thread"}
	for _, value := range generic {
		if strings.EqualFold(title, value) {
			return ""
		}
	}
	if pane.DefinitionID == "codex" && regexp.MustCompile(`(?i)^(?:thread\s+)?[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$`).MatchString(title) {
		return ""
	}
	return title
}

func (a *Application) discoveredAgentItems(panes []tmux.Pane) []palette.Item {
	var unmanaged []agent.Pane
	for _, pane := range panes {
		if pane.Kind == "" && !pane.Done() && atoi(pane.PID) > 0 {
			unmanaged = append(unmanaged, agent.Pane{ID: pane.ID, PID: pane.PID})
		}
	}
	detected := a.AgentDiscovery.Detect(unmanaged)
	usedNames := map[string]bool{}
	for _, pane := range panes {
		if pane.Kind == "agent" {
			usedNames[pane.Name] = true
		}
	}
	var items []palette.Item
	for _, pane := range panes {
		provider := detected[pane.ID]
		if provider == "" {
			continue
		}
		instance := 1
		for usedNames[agentInstanceName(provider, instance)] {
			instance++
		}
		name := agentInstanceName(provider, instance)
		usedNames[name] = true
		adopted := pane
		adopted.Kind = "agent"
		adopted.DefinitionID = provider
		adopted.Name = name
		items = append(items, a.runningItem(adopted, "external agent · window "+pane.WindowIndex))
	}
	return items
}

func (a *Application) confirmSwitch(session string) bool {
	fmt.Fprintf(a.Output, "Switch tmux client to %s? [y/N] ", session)
	if flusher, ok := a.Output.(interface{ Flush() error }); ok {
		_ = flusher.Flush()
	}
	var answer string
	_, _ = fmt.Fscanln(a.Input, &answer)
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func findManaged(panes []tmux.Pane, kind, id string) (tmux.Pane, bool) {
	for _, pane := range panes {
		if pane.Kind == kind && pane.DefinitionID == id {
			return pane, true
		}
	}
	return tmux.Pane{}, false
}

func findDefinition(defs []config.Definition, id string) config.Definition {
	for _, def := range defs {
		if def.ID == id {
			return def
		}
	}
	return config.Definition{}
}

func atoi(value string) int {
	i, _ := strconv.Atoi(value)
	return i
}
