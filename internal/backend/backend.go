package backend

import "github.com/Martin-Nyaga/muxpad/internal/config"

type Pane struct {
	ID             string
	Workspace      string
	Tab            string
	TabIndex       string
	Window         string
	WindowIndex    string
	Kind           string
	DefinitionID   string
	Name           string
	Dead           bool
	Finished       bool
	CurrentCommand string
	Title          string
	PID            string
	CurrentPath    string
}

func (p Pane) Done() bool {
	return p.Dead || p.Finished
}

type Workspace struct {
	ID    string
	Label string
	Root  string
}

type LaunchSpec struct {
	Workspace  string
	Definition config.Definition
	Kind       string
	Name       string
	Root       string
	Placement  config.Placement
	Target     string
}

type CreateTabSpec struct {
	Workspace string
	Label     string
	Directory string
	Focus     bool
}

type SplitPaneSpec struct {
	Workspace string
	Target    string
	Direction config.Placement
	Directory string
	Focus     bool
}

type Backend interface {
	Inside() bool
	CurrentWorkspace() (string, error)
	CurrentPane() (string, error)
	WorkspaceExists(string) bool
	Workspaces() []string
	WorkspaceList() ([]Workspace, error)
	CreateWorkspace(name, root, projectID string) (string, error)
	FocusWorkspace(workspace string) error
	ProjectContext(workspace string) string
	WorkspaceRoot(workspace string) string
	ManagedRoot(workspace string) string
	Panes(workspace string) ([]Pane, error)
	Launch(LaunchSpec) (string, error)
	Focus(Pane) error
	Restart(Pane, config.Definition) error
	Attach(workspace string) error
	Switch(workspace string) error
	PopupMenu(program string) error
	KillWorkspace(workspace string) error

	CreateTab(CreateTabSpec) (Pane, error)
	SplitPane(SplitPaneSpec) (Pane, error)
	RunInPane(Pane, string) error
}
