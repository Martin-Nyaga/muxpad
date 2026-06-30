package shellwords

import "strings"

func Split(input string) ([]string, error) {
	var out []string
	var b strings.Builder
	var quote rune
	escaped := false
	inWord := false
	for _, r := range input {
		if escaped {
			b.WriteRune(r)
			escaped = false
			inWord = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			inWord = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			inWord = true
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			inWord = true
		case ' ', '\t', '\n', '\r':
			if inWord {
				out = append(out, b.String())
				b.Reset()
				inWord = false
			}
		default:
			b.WriteRune(r)
			inWord = true
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	if inWord {
		out = append(out, b.String())
	}
	return out, nil
}

func Escape(value string) string {
	if value == "" {
		return "''"
	}
	if allSafe(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func allSafe(value string) bool {
	for _, r := range value {
		if !safe(r) {
			return false
		}
	}
	return true
}

func safe(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		strings.ContainsRune("_-.,:+/@%", r)
}
