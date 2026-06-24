package palette

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type StateKind string

const (
	StateRunning     StateKind = "running"
	StateIdle        StateKind = "idle"
	StateFinished    StateKind = "finished"
	StateAvailable   StateKind = "available"
	StateDisabled    StateKind = "disabled"
	StateUnavailable StateKind = "unavailable"
)

type Item struct {
	Token       string
	Section     string
	Name        string
	Description string
	Command     string
	Directory   string
	State       string
	StateKind   StateKind
	Summary     string
}

type Selection struct {
	Action string
	Token  string
}

type Option struct {
	Token string
	Label string
}

const (
	Reset               = "\033[0m"
	Bold                = "\033[1m"
	Dim                 = "\033[2m"
	Reverse             = "\033[7m"
	Header              = "\033[1;35m"
	RunningSection      = "Running"
	SidebarWidth        = 20
	SummarySidebarWidth = 30
	NameMin             = 10
	NameMax             = 24
)

var StateColor = map[StateKind]string{
	StateRunning:     "\033[32m",
	StateFinished:    "\033[33m",
	StateAvailable:   "\033[32m",
	StateDisabled:    Dim,
	StateUnavailable: "\033[31m",
	StateIdle:        Dim,
}

type Palette struct {
	Input  *os.File
	Output io.Writer
	Prompt string
}

func New() *Palette {
	return &Palette{Input: os.Stdin, Output: os.Stdout, Prompt: "Muxpad"}
}

func (p *Palette) Select(items []Item, sectionOrder []string) (Selection, bool, error) {
	if p.Input == nil {
		p.Input = os.Stdin
	}
	if p.Output == nil {
		p.Output = os.Stdout
	}
	if !term.IsTerminal(int(p.Input.Fd())) {
		return Selection{}, false, fmt.Errorf("the Muxpad palette requires an interactive terminal")
	}
	running, launch := splitRunning(items)
	model := newModel(launch, running, sectionOrder)
	render := func() {
		fmt.Fprint(p.Output, "\033[?25l\033[H")
		for _, line := range model.BodyLines(width(p.Output)) {
			fmt.Fprintf(p.Output, "%s\033[K\r\n", line)
		}
		fmt.Fprint(p.Output, "\033[J")
	}
	return p.interact(render, model.handle)
}

func (p *Palette) Choose(options []Option, title string) (string, bool, error) {
	if p.Input == nil {
		p.Input = os.Stdin
	}
	if p.Output == nil {
		p.Output = os.Stdout
	}
	if !term.IsTerminal(int(p.Input.Fd())) {
		return "", false, fmt.Errorf("the Muxpad palette requires an interactive terminal")
	}
	cursor := 0
	render := func() {
		fmt.Fprint(p.Output, "\033[?25l\033[H")
		fmt.Fprintf(p.Output, "%s  %s%s\033[K\r\n\r\n", Bold, title, Reset)
		for i, option := range options {
			line := "  " + option.Label
			if i == cursor {
				line = Reverse + line + Reset
			}
			fmt.Fprintf(p.Output, "%s\033[K\r\n", line)
		}
		fmt.Fprintf(p.Output, "\r\n  %senter select · esc cancel%s\033[J", Dim, Reset)
	}
	oldState, err := term.MakeRaw(int(p.Input.Fd()))
	if err != nil {
		return "", false, err
	}
	defer func() {
		_ = term.Restore(int(p.Input.Fd()), oldState)
		fmt.Fprint(p.Output, "\033[?25h\033[2J\033[H")
	}()
	reader := bufio.NewReader(p.Input)
	for {
		render()
		key := readKey(reader)
		switch key {
		case "up":
			if cursor > 0 {
				cursor--
			}
		case "down":
			if cursor < len(options)-1 {
				cursor++
			}
		case "enter":
			if len(options) == 0 {
				return "", false, nil
			}
			return options[cursor].Token, true, nil
		case "escape", "cancel":
			return "", false, nil
		}
	}
}

func (p *Palette) interact(render func(), handle func(string) (Selection, bool, bool)) (Selection, bool, error) {
	oldState, err := term.MakeRaw(int(p.Input.Fd()))
	if err != nil {
		return Selection{}, false, err
	}
	defer func() {
		_ = term.Restore(int(p.Input.Fd()), oldState)
		fmt.Fprint(p.Output, "\033[?25h\033[2J\033[H")
	}()
	reader := bufio.NewReader(p.Input)
	for {
		render()
		key := readKey(reader)
		selection, ok, done := handle(key)
		if done {
			return selection, ok, nil
		}
	}
}

type model struct {
	running      []Item
	launch       []Item
	sectionOrder []string
	query        string
	focusRunning bool
	cursor       int
	runCursor    int
	rows         []row
	selectable   []int
	nameWidth    int
}

type row struct {
	kind  string
	label string
	item  Item
}

func newModel(launch, running []Item, sectionOrder []string) *model {
	m := &model{launch: launch, running: running, sectionOrder: sectionOrder, nameWidth: NameWidth(launch)}
	m.refilter()
	return m
}

func (m *model) handle(key string) (Selection, bool, bool) {
	switch key {
	case "up":
		m.move(-1)
	case "down":
		m.move(1)
	case "left":
		if len(m.running) > 0 {
			m.focusRunning = true
		}
	case "right":
		m.focusRunning = false
	case "backspace":
		if m.query != "" {
			m.query = m.query[:len(m.query)-1]
			m.focusRunning = false
			m.refilter()
		}
	case "clear":
		m.query = ""
		m.focusRunning = false
		m.refilter()
	case "enter":
		if current, ok := m.current(); ok {
			return Selection{Action: "enter", Token: current.Token}, true, true
		}
	case "tab":
		if !m.focusRunning {
			if current, ok := m.current(); ok {
				return Selection{Action: "tab", Token: current.Token}, true, true
			}
		}
	case "restart":
		if current, ok := m.current(); ok && current.StateKind == StateFinished {
			return Selection{Action: "ctrl-r", Token: current.Token}, true, true
		}
	case "escape", "cancel":
		return Selection{}, false, true
	default:
		if strings.HasPrefix(key, "char:") {
			m.query += strings.TrimPrefix(key, "char:")
			m.focusRunning = false
			m.refilter()
		}
	}
	return Selection{}, false, false
}

func (m *model) refilter() {
	grouped := map[string][]Item{}
	for _, item := range m.launch {
		if Score(item, m.query) >= 0 {
			grouped[item.Section] = append(grouped[item.Section], item)
		}
	}
	m.rows = nil
	for _, section := range m.sectionOrder {
		items := grouped[section]
		if len(items) == 0 {
			continue
		}
		if len(m.rows) > 0 {
			m.rows = append(m.rows, row{kind: "blank"})
		}
		m.rows = append(m.rows, row{kind: "header", label: section})
		for _, item := range items {
			m.rows = append(m.rows, row{kind: "item", item: item})
		}
	}
	m.selectable = nil
	for i, row := range m.rows {
		if row.kind == "item" {
			m.selectable = append(m.selectable, i)
		}
	}
	m.cursor = 0
}

func (m *model) current() (Item, bool) {
	if m.focusRunning {
		if len(m.running) == 0 {
			return Item{}, false
		}
		return m.running[m.runCursor], true
	}
	if len(m.selectable) == 0 {
		return Item{}, false
	}
	return m.rows[m.selectable[m.cursor]].item, true
}

func (m *model) move(delta int) {
	if m.focusRunning {
		m.runCursor = clamp(m.runCursor+delta, 0, len(m.running)-1)
		return
	}
	m.cursor = clamp(m.cursor+delta, 0, len(m.selectable)-1)
}

func (m *model) BodyLines(totalWidth int) []string {
	if totalWidth <= 0 {
		totalWidth = 80
	}
	launch := m.launchLines(totalWidth)
	if len(m.running) == 0 {
		return launch
	}
	sideWidth := SidebarWidth
	for _, item := range m.running {
		if item.Summary != "" {
			sideWidth = SummarySidebarWidth
			break
		}
	}
	launch = m.launchLines(totalWidth - sideWidth - 1)
	side := SidebarLines(m.running, sideWidth, m.focusRunning, m.runCursor)
	height := max(len(launch), len(side))
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		left := strings.Repeat(" ", sideWidth)
		if i < len(side) {
			left = side[i]
		}
		right := ""
		if i < len(launch) {
			right = launch[i]
		}
		out = append(out, left+Dim+"│"+Reset+right)
	}
	return out
}

func (m *model) launchLines(width int) []string {
	out := make([]string, 0, len(m.rows))
	for _, row := range m.rows {
		switch row.kind {
		case "blank":
			out = append(out, "")
		case "header":
			out = append(out, "  "+Header+row.label+Reset)
		case "item":
			selected := false
			if current, ok := m.current(); ok && !m.focusRunning && current.Token == row.item.Token {
				selected = true
			}
			out = append(out, RenderItem(row.item, width, m.nameWidth, selected))
		}
	}
	return out
}

func splitRunning(items []Item) ([]Item, []Item) {
	var running, launch []Item
	for _, item := range items {
		if item.Section == RunningSection {
			running = append(running, item)
		} else {
			launch = append(launch, item)
		}
	}
	return running, launch
}

func readKey(reader *bufio.Reader) string {
	r, _, err := reader.ReadRune()
	if err != nil {
		return "escape"
	}
	switch r {
	case '\r', '\n':
		return "enter"
	case '\t':
		return "tab"
	case 0x7f, '\b':
		return "backspace"
	case 0x03:
		return "cancel"
	case 0x12:
		return "restart"
	case 0x0e:
		return "down"
	case 0x10:
		return "up"
	case 0x15:
		return "clear"
	case 0x1b:
		next, _ := reader.Peek(2)
		if len(next) == 2 && next[0] == '[' {
			_, _ = reader.Discard(2)
			switch next[1] {
			case 'A':
				return "up"
			case 'B':
				return "down"
			case 'C':
				return "right"
			case 'D':
				return "left"
			}
		}
		return "escape"
	default:
		if r >= 0x20 && r < 0x7f {
			return "char:" + string(r)
		}
		return "ignore"
	}
}

func width(output io.Writer) int {
	if file, ok := output.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		w, _, err := term.GetSize(int(file.Fd()))
		if err == nil {
			return w - 2
		}
	}
	return 78
}

func clamp(value, low, high int) int {
	if high < low {
		return low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
