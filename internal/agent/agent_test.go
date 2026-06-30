package agent

import (
	"reflect"
	"testing"
)

func TestDetectsInteractiveClaudeAndNodeWrappedCodexDescendants(t *testing.T) {
	discovery := discoveryFor(`100 1 zsh zsh
101 100 claude /home/me/.local/bin/claude
200 1 zsh zsh
201 200 MainThread node /opt/node_modules/@openai/codex/bin/codex.js -c tui.theme=dark
202 201 codex /opt/codex -c tui.theme=dark
`)
	got := discovery.Detect([]Pane{{ID: "%1", PID: "100"}, {ID: "%2", PID: "200"}})
	want := map[string]string{"%1": "claude", "%2": "codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Detect = %#v", got)
	}
}

func TestDetectsResumedCodexAfterGlobalOptions(t *testing.T) {
	discovery := discoveryFor("300 1 codex /opt/codex -c model=gpt-5 resume abc123\n")
	got := discovery.Detect([]Pane{{ID: "%3", PID: "300"}})
	if !reflect.DeepEqual(got, map[string]string{"%3": "codex"}) {
		t.Fatalf("Detect = %#v", got)
	}
}

func TestIgnoresNoninteractiveAndUnrelatedProcesses(t *testing.T) {
	discovery := discoveryFor(`100 1 claude /opt/claude --print hello
200 1 codex /opt/codex app-server
300 1 node node server.js
400 1 codex /opt/codex exec run-tests
500 1 node node server.js codex
`)
	panes := []Pane{{ID: "%100", PID: "100"}, {ID: "%200", PID: "200"}, {ID: "%300", PID: "300"}, {ID: "%400", PID: "400"}, {ID: "%500", PID: "500"}}
	if got := discovery.Detect(panes); len(got) != 0 {
		t.Fatalf("Detect = %#v", got)
	}
}

func TestReturnsNothingWhenPsFails(t *testing.T) {
	discovery := Discovery{Capture: func() (string, string, bool) { return "", "denied", false }}
	if got := discovery.Detect([]Pane{{ID: "%1", PID: "100"}}); len(got) != 0 {
		t.Fatalf("Detect = %#v", got)
	}
}

func discoveryFor(output string) Discovery {
	return Discovery{Capture: func() (string, string, bool) { return output, "", true }}
}
