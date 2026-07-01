package app

import (
	"fmt"
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/palette"
)

var DeclaredTaskSectionOrder = []string{"Tasks"}

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
	if _, err := a.Tmux.Panes(workspace); err != nil {
		return err
	}
	items := declaredTaskItems(project)
	selection, ok, err := a.Palette.Select(items, DeclaredTaskSectionOrder)
	if err != nil || !ok {
		return err
	}
	parts := strings.SplitN(selection.Token, ":", 2)
	if len(parts) != 2 || parts[0] != "task" {
		return fmt.Errorf("invalid declared task selection: %s", selection.Token)
	}
	task, ok := project.Task(parts[1])
	if !ok {
		return fmt.Errorf("unknown task %q for %s", parts[1], project.Name)
	}
	return a.launchTask(workspace, project, task.ID, "")
}

func declaredTaskItems(project config.Project) []palette.Item {
	items := make([]palette.Item, 0, len(project.Tasks))
	for _, task := range project.Tasks {
		items = append(items, palette.Item{
			Token:       "task:" + task.ID,
			Section:     "Tasks",
			Name:        task.Name,
			Description: task.Description,
			Command:     task.Command,
			Directory:   resolveDirectory(task, project.Root),
			State:       "not running",
			StateKind:   palette.StateIdle,
		})
	}
	return items
}
