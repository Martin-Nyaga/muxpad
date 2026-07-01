package app

import (
	"fmt"

	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/palette"
)

var DeclaredTaskSectionOrder = []string{"Tasks", "Discovered scripts"}

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
