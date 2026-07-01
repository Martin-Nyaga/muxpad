package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/Martin-Nyaga/muxpad/internal/app"
	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/tmux"
)

type Application interface {
	Start(projectID string, empty, attach bool) (string, error)
	Menu(attach bool) (string, error)
	Task(id string, placement config.Placement, attach bool) error
	Agent(id string, placement config.Placement, attach bool) error
}

type Tmux interface {
	Available() bool
	Inside() bool
	PopupMenu(program string) error
}

type CLI struct {
	Args    []string
	Output  io.Writer
	Error   io.Writer
	App     Application
	Tmux    Tmux
	Program string
}

func Run(args []string, output, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "muxpad: %v\n", err)
		return 1
	}
	tmuxClient := tmux.New()
	application := app.New(cfg, tmuxClient)
	return (&CLI{Args: args, Output: output, Error: stderr, App: application, Tmux: tmuxClient, Program: os.Args[0]}).Run()
}

func (c *CLI) Run() (code int) {
	defer func() {
		if recover() != nil {
			code = 130
		}
	}()
	if c.Output == nil {
		c.Output = os.Stdout
	}
	if c.Error == nil {
		c.Error = os.Stderr
	}
	if c.Program == "" {
		c.Program = os.Args[0]
	}
	if c.Tmux == nil || !c.Tmux.Available() {
		fmt.Fprintln(c.Error, "muxpad: tmux is required")
		return 1
	}
	args := append([]string{}, c.Args...)
	command := ""
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	var err error
	switch command {
	case "start":
		err = c.runStart(args)
	case "menu":
		err = c.runMenu(args)
	case "task":
		err = c.runLaunch(args, "task")
	case "agent":
		err = c.runLaunch(args, "agent")
	case "help", "--help", "-h", "":
		_, err = fmt.Fprint(c.Output, Help())
	default:
		err = fmt.Errorf("unknown command: %s\n\n%s", command, Help())
	}
	if err != nil {
		fmt.Fprintf(c.Error, "muxpad: %v\n", err)
		return 1
	}
	return 0
}

func (c *CLI) runStart(args []string) error {
	empty := false
	var rest []string
	for _, arg := range args {
		if arg == "--empty" {
			empty = true
		} else {
			rest = append(rest, arg)
		}
	}
	if len(rest) > 1 {
		return fmt.Errorf("start accepts at most one project")
	}
	projectID := ""
	if len(rest) == 1 {
		projectID = rest[0]
	}
	_, err := c.App.Start(projectID, empty, true)
	return err
}

func (c *CLI) runLaunch(args []string, kind string) error {
	placement, rest, err := parsePlacement(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("%s requires exactly one name", kind)
	}
	if kind == "task" {
		return c.App.Task(rest[0], placement, true)
	}
	return c.App.Agent(rest[0], placement, true)
}

func parsePlacement(args []string) (config.Placement, []string, error) {
	var placement config.Placement
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--window":
			placement = config.PlacementWindow
		case "--vertical":
			placement = config.PlacementVertical
		case "--horizontal":
			placement = config.PlacementHorizontal
		case "--placement":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("flag needs an argument: --placement")
			}
			i++
			placement = config.Placement(args[i])
			if placement != config.PlacementWindow && placement != config.PlacementVertical && placement != config.PlacementHorizontal {
				return "", nil, fmt.Errorf("invalid placement: %s", placement)
			}
		default:
			rest = append(rest, args[i])
		}
	}
	return placement, rest, nil
}

func (c *CLI) runMenu(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("menu accepts no arguments")
	}
	if c.Tmux.Inside() && os.Getenv("MUXPAD_POPUP") != "1" {
		// Resolve the absolute path of the running binary so the popup can
		// re-exec it regardless of how muxpad was invoked. os.Args[0] is just
		// the bare name when found via PATH, and filepath.Abs would resolve it
		// against the cwd, producing a path that does not exist.
		program, err := os.Executable()
		if err != nil {
			program = c.Program
		}
		return c.Tmux.PopupMenu(program)
	}
	_, err := c.App.Menu(true)
	return err
}

func Help() string {
	return `Usage:
  muxpad start [project] [--empty]
  muxpad menu
  muxpad task <name> [--window|--vertical|--horizontal]
  muxpad agent <name> [--window|--vertical|--horizontal]
`
}
