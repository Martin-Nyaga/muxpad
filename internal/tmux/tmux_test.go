package tmux

import "testing"

func TestCurrentSessionAndPaneTrimTmuxOutput(t *testing.T) {
	client := &Client{
		Prefix: []string{"tmux"},
		Run: func(args ...string) Result {
			if len(args) >= 4 && args[1] == "display-message" && args[3] == "#{session_name}" {
				return Result{Stdout: "demo\n", OK: true}
			}
			if len(args) >= 4 && args[1] == "display-message" && args[3] == "#{pane_id}" {
				return Result{Stdout: "%3\n", OK: true}
			}
			return Result{Stderr: "unexpected command", OK: false}
		},
	}

	session, err := client.CurrentSession()
	if err != nil {
		t.Fatal(err)
	}
	if session != "demo" {
		t.Fatalf("CurrentSession = %q", session)
	}

	pane, err := client.CurrentPane()
	if err != nil {
		t.Fatal(err)
	}
	if pane != "%3" {
		t.Fatalf("CurrentPane = %q", pane)
	}
}
