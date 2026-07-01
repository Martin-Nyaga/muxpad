package palette

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// escapeTimeout is how long readKey waits for the rest of an escape sequence
// (e.g. arrow keys send ESC [ A) before treating a bare ESC as "escape". This
// mirrors the Ruby palette's 20ms wait_readable; without it Peek would block on
// a lone ESC until the next keypress, so dismissing the menu took several Escs.
const escapeTimeout = 50 * time.Millisecond

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
	MinList             = 3
	RightPad            = 2
	Reset               = "\033[0m"
	Bold                = "\033[1m"
	Dim                 = "\033[2m"
	Reverse             = "\033[7m"
	Header              = "\033[1;35m"
	// Accent is the ayu theme orange (#ff8f40) as a truecolor foreground, used
	// for the selected-row bar and the detail box border.
	Accent = "\033[38;2;255;143;64m"
	// Muted is the ayu surface_dim (#273747), a subtle grey for the divider
	// between the running sidebar and the searchable list.
	Muted               = "\033[38;2;39;55;71m"
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
		columns, rows := terminalSize(p.Output)
		fmt.Fprint(p.Output, "\033[?25l\033[H")
		for i, line := range model.Render(columns-RightPad, rows) {
			if i > 0 {
				fmt.Fprint(p.Output, "\r\n")
			}
			fmt.Fprintf(p.Output, "%s\033[K", line)
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
	handle := func(key string) (Selection, bool, bool) {
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
				return Selection{}, false, true
			}
			return Selection{Token: options[cursor].Token}, true, true
		case "escape", "cancel":
			return Selection{}, false, true
		}
		return Selection{}, false, false
	}
	keys := p.readKeys()
	resize := watchResize()
	defer signal.Stop(resize)
	selection, ok := runLoop(render, handle, keys, resize)
	return selection.Token, ok, nil
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
	keys := p.readKeys()
	resize := watchResize()
	defer signal.Stop(resize)
	selection, ok := runLoop(render, handle, keys, resize)
	return selection, ok, nil
}

// readKeys pumps decoded keys onto a channel from a background reader. The
// palette process is short-lived, so a reader parked on Stdin after the loop
// returns is harmless.
func (p *Palette) readKeys() <-chan string {
	keys := make(chan string)
	reader := bufio.NewReader(p.Input)
	fd := int(p.Input.Fd())
	go func() {
		for {
			keys <- readKey(reader, fd)
		}
	}()
	return keys
}

// watchResize delivers SIGWINCH so the loop can repaint. Overlay panes (herdr
// overlays, tmux popups) often settle their size just after the child starts;
// without this the first paint is drawn at the wrong size and only corrected on
// the next keypress.
func watchResize() chan os.Signal {
	resize := make(chan os.Signal, 1)
	signal.Notify(resize, unix.SIGWINCH)
	return resize
}

// runLoop renders once, then repaints on every key and on every resize, ending
// when handle reports it is done. Splitting this out keeps the event handling
// testable without a real terminal.
func runLoop(render func(), handle func(string) (Selection, bool, bool), keys <-chan string, resize <-chan os.Signal) (Selection, bool) {
	render()
	for {
		select {
		case <-resize:
			render()
		case key, open := <-keys:
			if !open {
				return Selection{}, false
			}
			selection, ok, done := handle(key)
			if done {
				return selection, ok
			}
			render()
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
	offset       int
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
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
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
	type rankedItem struct {
		item  Item
		score int
		index int
	}
	grouped := map[string][]rankedItem{}
	for index, item := range m.launch {
		if score := Score(item, m.query); score >= 0 {
			grouped[item.Section] = append(grouped[item.Section], rankedItem{item: item, score: score, index: index})
		}
	}
	m.rows = nil
	for _, section := range m.sectionOrder {
		items := grouped[section]
		if len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].score == items[j].score {
				return items[i].index < items[j].index
			}
			return items[i].score > items[j].score
		})
		if len(m.rows) > 0 {
			m.rows = append(m.rows, row{kind: "blank"})
		}
		m.rows = append(m.rows, row{kind: "header", label: section})
		for _, item := range items {
			m.rows = append(m.rows, row{kind: "item", item: item.item})
		}
	}
	m.selectable = nil
	for i, row := range m.rows {
		if row.kind == "item" {
			m.selectable = append(m.selectable, i)
		}
	}
	m.cursor = 0
	m.offset = 0
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

func (m *model) Render(totalWidth, totalRows int) []string {
	if totalWidth <= 0 {
		totalWidth = 80
	}
	top := m.topLines()
	footer := m.footerLines(totalWidth)
	listHeight := max(totalRows-len(top)-len(footer), MinList)
	body := m.BodyLines(totalWidth, listHeight)
	pad := max(totalRows-len(top)-len(body)-len(footer), 0)
	lines := append([]string{}, top...)
	lines = append(lines, body...)
	for range pad {
		lines = append(lines, "")
	}
	lines = append(lines, footer...)
	return lines
}

// topLines renders the filter input. The overlay pane already draws a titled
// border, so there is no in-palette heading (which used to duplicate it).
func (m *model) topLines() []string {
	input := "  " + Bold + "❯" + Reset + " "
	if m.query == "" {
		input += Dim + "Type to filter" + Reset
	} else {
		input += m.query
	}
	return []string{"", input, ""}
}

// footerLines renders the detail box followed by the key hint.
func (m *model) footerLines(totalWidth int) []string {
	lines := append([]string{""}, m.detailBox(totalWidth)...)
	return append(lines, "", "  "+Dim+m.hint()+Reset)
}

func (m *model) BodyLines(totalWidth, listHeight int) []string {
	if totalWidth <= 0 {
		totalWidth = 80
	}
	launch := m.launchLines(totalWidth, listHeight)
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
	launch = m.launchLines(totalWidth-sideWidth-1, listHeight)
	side := SidebarLines(m.running, sideWidth, m.focusRunning, m.runCursor)
	height := min(max(len(launch), len(side)), listHeight)
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
		out = append(out, left+Muted+"│"+Reset+right)
	}
	return out
}

func (m *model) launchLines(width, height int) []string {
	m.clampOffset(height)
	last := min(m.offset+height, len(m.rows))
	out := make([]string, 0, last-m.offset)
	for _, row := range m.rows[m.offset:last] {
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

func (m *model) clampOffset(height int) {
	if len(m.selectable) == 0 {
		m.offset = 0
		return
	}
	line := m.selectable[m.cursor]
	if line < m.offset {
		m.offset = line
	}
	if line > m.offset+height-1 {
		m.offset = line - height + 1
	}
	m.offset = clamp(m.offset, 0, max(len(m.rows)-1, 0))
}

// detailBox renders a rounded, accent-bordered box previewing the current item.
// The box title is the verb (Run / Open / Focus); the body shows the command
// (for tasks/scripts) and the directory, each with an icon.
func (m *model) detailBox(totalWidth int) []string {
	item, ok := m.current()
	if !ok {
		return nil
	}
	boxWidth := max(totalWidth-2, 12)
	inner := boxWidth - 4
	var content []string
	if item.Command != "" && item.Command != item.Directory {
		content = append(content, boxContent(Accent+"▶"+Reset+" "+Truncate(item.Command, inner-2), inner))
	}
	if item.Directory != "" {
		content = append(content, boxContent("📁 "+Dim+Truncate(Abbreviate(item.Directory), inner-3)+Reset, inner))
	}
	if item.State != "" {
		color := StateColor[item.StateKind]
		if color == "" {
			color = Dim
		}
		content = append(content, boxContent(color+stateGlyph(item.StateKind)+" "+Truncate(item.State, inner-2)+Reset, inner))
	}
	if len(content) == 0 {
		content = append(content, boxContent("", inner))
	}
	title := " " + m.verb() + " "
	dashes := max(boxWidth-3-visibleWidth(title), 0)
	top := "  " + Accent + "╭─" + Bold + title + Reset + Accent + strings.Repeat("─", dashes) + "╮" + Reset
	bottom := "  " + Accent + "╰" + strings.Repeat("─", max(boxWidth-2, 0)) + "╯" + Reset
	lines := []string{top}
	for _, c := range content {
		lines = append(lines, "  "+Accent+"│"+Reset+" "+c+" "+Accent+"│"+Reset)
	}
	return append(lines, bottom)
}

func (m *model) verb() string {
	item, ok := m.current()
	if !ok {
		return "Run"
	}
	if item.StateKind == StateRunning || item.StateKind == StateFinished {
		return "Focus"
	}
	// Project entries carry a directory but no command; they open a workspace.
	if item.Command == "" && item.Directory != "" {
		return "Open"
	}
	return "Run"
}

func (m *model) hint() string {
	switcher := ""
	if len(m.running) > 0 {
		switcher = "←/→ switch · "
	}
	current, ok := m.current()
	restartable := ok && current.StateKind == StateFinished
	if m.focusRunning {
		actions := []string{"enter focus"}
		if restartable {
			actions = append(actions, "ctrl-r restart")
		}
		return switcher + strings.Join(actions, " · ") + " · esc close"
	}
	actions := []string{"enter " + strings.ToLower(m.verb())}
	// Placement actions apply to launchable items (which carry a command), not
	// to project entries.
	if ok && current.Command != "" {
		actions = append(actions, "tab actions")
	}
	if restartable {
		actions = append(actions, "ctrl-r restart")
	}
	return switcher + strings.Join(actions, " · ") + " · esc close"
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

func readKey(reader *bufio.Reader, fd int) string {
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
		// Wait briefly for the rest of an escape sequence. A bare ESC has no
		// follow-up bytes, so the poll times out and we treat it as "escape"
		// without blocking on the next keypress.
		if !waitReadable(reader, fd, escapeTimeout) {
			return "escape"
		}
		if b, err := reader.ReadByte(); err != nil || b != '[' {
			return "escape"
		}
		dir, err := reader.ReadByte()
		if err != nil {
			return "escape"
		}
		switch dir {
		case 'A':
			return "up"
		case 'B':
			return "down"
		case 'C':
			return "right"
		case 'D':
			return "left"
		}
		return "ignore"
	default:
		if r >= 0x20 && r < 0x7f {
			return "char:" + string(r)
		}
		return "ignore"
	}
}

// waitReadable reports whether more input is available within timeout, either
// already buffered by the reader or arriving on fd. It retries on EINTR so a
// signal does not spuriously report "no input".
func waitReadable(reader *bufio.Reader, fd int, timeout time.Duration) bool {
	if reader.Buffered() > 0 {
		return true
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	ms := int(timeout.Milliseconds())
	for {
		n, err := unix.Poll(fds, ms)
		if err == unix.EINTR {
			continue
		}
		return err == nil && n > 0
	}
}

func terminalSize(output io.Writer) (int, int) {
	if file, ok := output.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		w, h, err := term.GetSize(int(file.Fd()))
		if err == nil {
			return w, h
		}
	}
	return 80, 24
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
