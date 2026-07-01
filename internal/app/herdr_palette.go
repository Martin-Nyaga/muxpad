package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/backend"
	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/palette"
)

var DeclaredTaskSectionOrder = []string{"Tasks", "Discovered scripts"}
var ProjectSectionOrder = []string{"Projects"}

func (a *Application) DeclaredTaskMenu() error {
	workspace, err := a.Tmux.CurrentWorkspace()
	if err != nil {
		return err
	}
	project, ok := a.projectForSession(workspace)
	if !ok {
		root := a.Tmux.WorkspaceRoot(workspace)
		if root == "" {
			root = workspace
		}
		return fmt.Errorf("no muxpad project configured for current herdr workspace (%s)", root)
	}
	items, err := a.herdrTaskItems(workspace, project)
	if err != nil {
		return err
	}
	selection, ok, err := a.Palette.Select(items, DeclaredTaskSectionOrder)
	if err != nil || !ok {
		return err
	}
	return a.HandleSelection(workspace, selection)
}

func (a *Application) herdrTaskItems(workspace string, project config.Project) ([]palette.Item, error) {
	panes, err := a.Tmux.Panes(workspace)
	if err != nil {
		return nil, err
	}
	scripts := a.discoveredScripts(workspace, project, true)
	items := make([]palette.Item, 0, len(project.Tasks)+len(scripts))
	for _, task := range project.Tasks {
		pane, ok := findManaged(panes, "task", task.ID)
		items = append(items, launchableItem("task:"+task.ID, "Tasks", task, pane, ok, project.Root))
		if ok {
			items = append(items, a.instanceItem("task:"+task.ID, task, pane))
		}
	}
	for _, script := range scripts {
		pane, ok := findManaged(panes, "script", script.ID)
		items = append(items, launchableItem("script:"+script.ID, "Discovered scripts", script, pane, ok, project.Root))
		if ok {
			items = append(items, a.instanceItem("script:"+script.ID, script, pane))
		}
	}
	return items, nil
}

func (a *Application) ProjectLauncherMenu() error {
	items, err := a.projectItems()
	if err != nil {
		return err
	}
	selection, ok, err := a.Palette.Select(items, ProjectSectionOrder)
	if err != nil || !ok {
		return err
	}
	parts := strings.SplitN(selection.Token, ":", 2)
	if len(parts) != 2 || parts[0] != "project" {
		return fmt.Errorf("invalid project selection: %s", selection.Token)
	}
	return a.OpenProject(parts[1])
}

func (a *Application) OpenProject(projectID string) error {
	project, ok := a.Config.Project(projectID)
	if !ok {
		return fmt.Errorf("unknown project: %s", projectID)
	}
	workspace, ok, err := a.projectWorkspace(project)
	if err != nil {
		return err
	}
	if ok {
		return a.Tmux.FocusWorkspace(workspace.ID)
	}
	_, err = a.Tmux.CreateWorkspace(project.Name, project.Root, project.ID)
	return err
}

func (a *Application) projectItems() ([]palette.Item, error) {
	workspaces, err := a.Tmux.WorkspaceList()
	if err != nil {
		return nil, err
	}
	items := make([]palette.Item, 0, len(a.Config.Projects))
	for _, project := range a.Config.Projects {
		state := "new workspace"
		stateKind := palette.StateAvailable
		if _, ok := projectWorkspaceFromList(a, project, workspaces); ok {
			state = "workspace open"
			stateKind = palette.StateRunning
		}
		items = append(items, palette.Item{
			Token:       "project:" + project.ID,
			Section:     "Projects",
			Name:        project.Name,
			Description: palette.Abbreviate(project.Root),
			Command:     project.Root,
			Directory:   project.Root,
			State:       state,
			StateKind:   stateKind,
		})
	}
	return items, nil
}

func (a *Application) projectWorkspace(project config.Project) (backend.Workspace, bool, error) {
	workspaces, err := a.Tmux.WorkspaceList()
	if err != nil {
		return backend.Workspace{}, false, err
	}
	workspace, ok := projectWorkspaceFromList(a, project, workspaces)
	return workspace, ok, nil
}

func projectWorkspaceFromList(a *Application, project config.Project, workspaces []backend.Workspace) (backend.Workspace, bool) {
	for _, workspace := range workspaces {
		if workspace.ID == project.ID || a.Tmux.ProjectContext(workspace.ID) == project.ID {
			return workspace, true
		}
		if workspace.Root != "" {
			if samePath(workspace.Root, project.Root) {
				return workspace, true
			}
			if matched, ok := a.Config.ProjectFor(workspace.Root); ok && matched.ID == project.ID {
				return workspace, true
			}
			continue
		}
		if workspace.Label == project.Name || workspace.Label == project.ID {
			return workspace, true
		}
	}
	return backend.Workspace{}, false
}

func samePath(a, b string) bool {
	return cleanPath(a) == cleanPath(b)
}

func cleanPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}
