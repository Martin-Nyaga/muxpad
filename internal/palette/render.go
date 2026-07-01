package palette

import (
	"os"
	"strings"
)

func Score(item Item, query string) int {
	if query == "" {
		return 0
	}
	query = strings.ToLower(query)
	score := -1
	for _, field := range []struct {
		text   string
		weight int
	}{
		{item.Name, 3},
		{item.Command, 2},
		{item.Description, 1},
	} {
		if inner := subsequenceScore(strings.ToLower(field.text), query); inner >= 0 {
			score = max(score, field.weight*100000+inner)
		}
	}
	return score
}

func subsequenceScore(text, query string) int {
	first := -1
	from := 0
	for _, char := range query {
		index := strings.IndexRune(text[from:], char)
		if index < 0 {
			return -1
		}
		index += from
		if first < 0 {
			first = index
		}
		from = index + 1
	}
	span := from - 1 - first
	score := 10000 - first*10 - span
	if score < 0 {
		return 0
	}
	return score
}

func RenderItem(item Item, width, nameWidth int, selected bool) string {
	avail := width - 3
	state := Truncate(item.State, min(avail/3, 30))
	name := padRight(Truncate(item.Name, nameWidth), nameWidth)
	descWidth := max(avail-nameWidth-2-len(state)-1, 0)
	desc := padRight(Truncate(item.Description, descWidth), descWidth)
	color := StateColor[item.StateKind]
	if color == "" {
		color = Dim
	}
	if selected {
		// A 3-column gutter: leading space keeps the bar off the pane border, the
		// bar sits in column 1, then a space before the name (column 3) which
		// aligns with unselected rows. Bold, non-dim text instead of a harsh
		// full-width reverse highlight.
		return " " + Accent + "▌" + Reset + " " + Bold + name + Reset + "  " + desc + " " + color + state + Reset
	}
	return "   " + name + "  " + Dim + desc + Reset + " " + color + state + Reset
}

func SidebarLines(running []Item, width int, focus bool, cursor int) []string {
	cells := []string{fill(" "+Header+"Running"+Reset, 8, width)}
	for i, item := range running {
		name := Truncate(item.Name, width-3)
		if focus && i == cursor {
			// Accent bar replaces the status dot in column 1 (with a leading pad),
			// so the name stays aligned with unselected rows in column 3.
			cells = append(cells, fill(" "+Accent+"▌"+Reset+" "+Bold+name+Reset, 3+len([]rune(name)), width))
		} else {
			color := StateColor[item.StateKind]
			if color == "" {
				color = StateColor[StateRunning]
			}
			cells = append(cells, fill(" "+color+"●"+Reset+" "+name, 3+len(name), width))
		}
		if item.Summary == "" {
			continue
		}
		summary := Truncate(item.Summary, width-4)
		if focus && i == cursor {
			cells = append(cells, fill(" "+Accent+"▌"+Reset+" "+Dim+summary+Reset, 3+len([]rune(summary)), width))
		} else {
			cells = append(cells, Dim+padRight(slice("   "+summary, width), width)+Reset)
		}
	}
	return cells
}

func NameWidth(items []Item) int {
	longest := 0
	for _, item := range items {
		if len(item.Name) > longest {
			longest = len(item.Name)
		}
	}
	return clamp(longest, NameMin, NameMax)
}

func Truncate(value string, width int) string {
	value = sanitizeLine(value)
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > width {
		return string(runes[:max(width-1, 0)]) + "…"
	}
	return value
}

func Abbreviate(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home+"/") {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func fill(text string, visible, width int) string {
	if visible >= width {
		return text
	}
	return text + strings.Repeat(" ", width-visible)
}

func padRight(value string, width int) string {
	length := len([]rune(value))
	if length >= width {
		return value
	}
	return value + strings.Repeat(" ", width-length)
}

func slice(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// boxContent right-pads styled content to width display columns, accounting for
// ANSI escapes and wide (emoji) runes so the box border stays aligned.
func boxContent(s string, width int) string {
	visible := visibleWidth(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

// visibleWidth is the terminal column count of s, skipping ANSI CSI escapes and
// counting astral-plane emoji (e.g. 📁) as two columns.
func visibleWidth(s string) int {
	width := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == 0x1b {
			i++
			if i < len(runes) && runes[i] == '[' {
				i++
				for i < len(runes) && !isCSIFinal(runes[i]) {
					i++
				}
			}
			continue
		}
		width += runeWidth(runes[i])
	}
	return width
}

func isCSIFinal(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

// runeWidth treats astral-plane runes (emoji like 📁) as two columns and every
// BMP rune used here (box drawing, geometric shapes, ✓/✗, block bar) as one.
func runeWidth(r rune) int {
	if r >= 0x1F000 {
		return 2
	}
	return 1
}

// sanitizeLine replaces control characters (e.g. a newline baked into a task
// command by a TOML "\n") with spaces so they cannot break the single-line
// layout of list rows and the detail box.
func sanitizeLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

func stateGlyph(kind StateKind) string {
	switch kind {
	case StateRunning:
		return "●"
	case StateFinished:
		return "✓"
	case StateUnavailable:
		return "✗"
	default:
		return "○"
	}
}
