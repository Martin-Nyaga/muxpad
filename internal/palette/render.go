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
	avail := width - 2
	state := Truncate(item.State, min(avail/3, 30))
	name := padRight(Truncate(item.Name, nameWidth), nameWidth)
	descWidth := max(avail-nameWidth-2-len(state)-1, 0)
	desc := padRight(Truncate(item.Description, descWidth), descWidth)
	if selected {
		line := "  " + name + "  " + desc + " " + state
		return Reverse + padRight(slice(line, width), width) + Reset
	}
	color := StateColor[item.StateKind]
	if color == "" {
		color = Dim
	}
	return "  " + name + "  " + Dim + desc + Reset + " " + color + state + Reset
}

func SidebarLines(running []Item, width int, focus bool, cursor int) []string {
	cells := []string{fill(" "+Header+"Running"+Reset, 8, width)}
	for i, item := range running {
		name := Truncate(item.Name, width-3)
		if focus && i == cursor {
			cells = append(cells, Reverse+padRight(slice(" ● "+name, width), width)+Reset)
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
		line := padRight(slice("   "+summary, width), width)
		if focus && i == cursor {
			cells = append(cells, Reverse+line+Reset)
		} else {
			cells = append(cells, Dim+line+Reset)
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
