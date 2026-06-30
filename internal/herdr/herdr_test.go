package herdr

import (
	"reflect"
	"testing"

	"github.com/Martin-Nyaga/muxpad/internal/backend"
)

func TestCreateTabCreatesFocusedTabAndFallsBackToCurrentPane(t *testing.T) {
	var calls [][]string
	client := &Client{
		Bin: "herdr-test",
		Run: func(args ...string) Result {
			calls = append(calls, append([]string{}, args...))
			switch len(calls) {
			case 1:
				return Result{Stdout: "created tab t1\n", OK: true}
			case 2:
				return Result{Stdout: `{"result":{"pane":{"pane_id":"p1"}}}`, OK: true}
			default:
				t.Fatalf("unexpected call: %#v", args)
				return Result{}
			}
		},
	}

	pane, err := client.CreateTab(backend.CreateTabSpec{Label: "muxpad skeleton", Directory: "/tmp/project", Focus: true})
	if err != nil {
		t.Fatal(err)
	}
	if pane.ID != "p1" {
		t.Fatalf("pane.ID = %q, want p1", pane.ID)
	}
	want := [][]string{
		{"herdr-test", "tab", "create", "--cwd", "/tmp/project", "--label", "muxpad skeleton", "--focus"},
		{"herdr-test", "pane", "current", "--current"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestRunInPaneSubmitsCommandToHerdrPaneRun(t *testing.T) {
	var got []string
	client := &Client{
		Bin: "herdr-test",
		Run: func(args ...string) Result {
			got = append([]string{}, args...)
			return Result{OK: true}
		},
	}

	err := client.RunInPane(backend.Pane{ID: "p1"}, "while :; do sleep 60; done")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"herdr-test", "pane", "run", "p1", "while :; do sleep 60; done"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPaneIDParsesNestedJSON(t *testing.T) {
	got := paneID(`{"result":{"created_tab":{"tab_id":"t1"},"root_pane":{"pane_id":"p1"}}}`)
	if got != "p1" {
		t.Fatalf("paneID = %q, want p1", got)
	}
}
