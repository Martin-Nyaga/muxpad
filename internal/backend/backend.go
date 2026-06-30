package backend

type Pane struct {
	ID string
}

type CreateTabSpec struct {
	Label     string
	Directory string
	Focus     bool
}

type Backend interface {
	CreateTab(CreateTabSpec) (Pane, error)
	RunInPane(Pane, string) error
}
