package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/shellwords"
)

type Pane struct {
	ID             string
	Session        string
	Window         string
	WindowIndex    string
	Kind           string
	DefinitionID   string
	Name           string
	Dead           bool
	Finished       bool
	CurrentCommand string
	Title          string
	PID            string
	CurrentPath    string
}

func (p Pane) Done() bool {
	return p.Dead || p.Finished
}

type LaunchSpec struct {
	Session    string
	Definition config.Definition
	Kind       string
	Name       string
	Root       string
	Placement  config.Placement
	Target     string
}

type Result struct {
	Stdout string
	Stderr string
	OK     bool
}

type Runner func(args ...string) Result

type Client struct {
	Prefix []string
	Run    Runner
}

const format = "#{pane_id}\t#{session_name}\t#{window_id}\t#{window_index}\t#{@muxpad_kind}\t#{@muxpad_id}\t#{@muxpad_name}\t#{pane_dead}\t#{@muxpad_finished}\t#{pane_current_command}\t#{pane_title}\t#{pane_pid}\t#{pane_current_path}"

func New() *Client {
	prefix := []string{envDefault("MUXPAD_TMUX", "tmux")}
	if socket := os.Getenv("MUXPAD_TMUX_SOCKET"); socket != "" {
		prefix = append(prefix, "-L", socket)
	}
	return &Client{Prefix: prefix}
}

func (c *Client) Available() bool {
	return c.run(append(c.Prefix, "-V")...).OK
}

func (c *Client) Inside() bool {
	return os.Getenv("TMUX") != ""
}

func (c *Client) CurrentSession() (string, error) {
	out, err := c.capture("display-message", "-p", "#{session_name}")
	return strings.TrimSpace(out), err
}

func (c *Client) CurrentPane() (string, error) {
	out, err := c.capture("display-message", "-p", "#{pane_id}")
	return strings.TrimSpace(out), err
}

func (c *Client) SessionExists(name string) bool {
	result := c.run(append(c.Prefix, "has-session", "-t", "="+name)...)
	return result.OK
}

func (c *Client) Sessions() []string {
	out, err := c.captureAllow("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil
	}
	return lines(out)
}

func (c *Client) CreateSession(name, root, projectID string) (string, error) {
	pane, err := c.capture("new-session", "-d", "-P", "-F", "#{pane_id}", "-s", name, "-c", root, "-n", "shell")
	if err != nil {
		return "", err
	}
	pane = strings.TrimSpace(pane)
	if err := c.runBang("set-option", "-t", name, "@muxpad_root", root); err != nil {
		return "", err
	}
	if err := c.runBang("set-option", "-t", name, "@muxpad_project", projectID); err != nil {
		return "", err
	}
	if err := c.runBang("set-option", "-w", "-t", name+":shell", "automatic-rename", "off"); err != nil {
		return "", err
	}
	if err := c.runBang("select-pane", "-t", pane, "-T", "shell"); err != nil {
		return "", err
	}
	return pane, nil
}

func (c *Client) ProjectContext(session string) string {
	out, _ := c.captureAllow("show-options", "-qv", "-t", session, "@muxpad_project")
	return strings.TrimSpace(out)
}

func (c *Client) ManagedRoot(session string) string {
	out, _ := c.captureAllow("show-options", "-qv", "-t", session, "@muxpad_root")
	return strings.TrimSpace(out)
}

func (c *Client) SessionRoot(session string) string {
	if root := c.ManagedRoot(session); root != "" {
		return root
	}
	out, _ := c.capture("display-message", "-p", "-t", session, "#{pane_current_path}")
	return strings.TrimSpace(out)
}

func (c *Client) Panes(session string) ([]Pane, error) {
	out, err := c.capture("list-panes", "-s", "-t", session, "-F", format)
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range lines(out) {
		fields := strings.Split(line, "\t")
		if len(fields) < 13 {
			continue
		}
		panes = append(panes, Pane{
			ID:             fields[0],
			Session:        fields[1],
			Window:         fields[2],
			WindowIndex:    fields[3],
			Kind:           fields[4],
			DefinitionID:   fields[5],
			Name:           fields[6],
			Dead:           fields[7] == "1",
			Finished:       fields[8] == "1",
			CurrentCommand: fields[9],
			Title:          fields[10],
			PID:            fields[11],
			CurrentPath:    fields[12],
		})
	}
	return panes, nil
}

func (c *Client) Launch(spec LaunchSpec) (string, error) {
	if err := c.syncPath(spec.Session); err != nil {
		return "", err
	}
	directory := spec.Root
	if spec.Definition.Directory != "" {
		directory = filepath.Join(spec.Root, spec.Definition.Directory)
	}
	command := c.WrappedCommand(spec.Definition.Command, spec.Definition.ExitMode)
	placeholder := "sh -c 'while :; do sleep 60; done'"
	var args []string
	if spec.Placement == config.PlacementWindow {
		target := fmt.Sprintf("%s:%d", spec.Session, c.nextWindowIndex(spec.Session))
		args = []string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-t", target, "-n", spec.Name, "-c", directory, placeholder}
	} else {
		flag := "-v"
		if spec.Placement == config.PlacementHorizontal {
			flag = "-h"
		}
		target := spec.Target
		if target == "" {
			target = spec.Session + ":shell"
		}
		args = []string{"split-window", "-d", "-P", "-F", "#{pane_id}", flag, "-t", target, "-c", directory, placeholder}
	}
	pane, err := c.capture(args...)
	if err != nil {
		return "", err
	}
	pane = strings.TrimSpace(pane)
	if spec.Placement == config.PlacementWindow {
		if err := c.runBang("set-option", "-w", "-t", pane, "automatic-rename", "off"); err != nil {
			return "", err
		}
	}
	options := map[string]string{
		"remain-on-exit":    "off",
		"@muxpad_kind":      spec.Kind,
		"@muxpad_id":        spec.Definition.ID,
		"@muxpad_name":      spec.Name,
		"@muxpad_command":   spec.Definition.Command,
		"@muxpad_directory": directory,
		"@muxpad_exit_mode": string(spec.Definition.ExitMode),
		"@muxpad_finished":  "0",
	}
	for key, value := range options {
		if err := c.runBang("set-option", "-p", "-t", pane, key, value); err != nil {
			return "", err
		}
	}
	if err := c.runBang("select-pane", "-t", pane, "-T", spec.Name); err != nil {
		return "", err
	}
	if panes, err := c.Panes(spec.Session); err == nil {
		for _, item := range panes {
			if item.ID == pane {
				_ = c.Focus(item)
			}
		}
	}
	if err := c.runBang("respawn-pane", "-k", "-t", pane, "-c", directory, command); err != nil {
		return "", err
	}
	return pane, nil
}

func (c *Client) Focus(pane Pane) error {
	if err := c.runBang("select-window", "-t", pane.Window); err != nil {
		return err
	}
	return c.runBang("select-pane", "-t", pane.ID)
}

func (c *Client) Restart(pane Pane, definition config.Definition) error {
	if !pane.Done() {
		return fmt.Errorf("%s is still running", pane.Name)
	}
	directory, err := c.capture("show-options", "-pqv", "-t", pane.ID, "@muxpad_directory")
	if err != nil {
		return err
	}
	directory = strings.TrimSpace(directory)
	for _, args := range [][]string{
		{"set-option", "-p", "-t", pane.ID, "remain-on-exit", "off"},
		{"set-option", "-p", "-t", pane.ID, "@muxpad_command", definition.Command},
		{"set-option", "-p", "-t", pane.ID, "@muxpad_exit_mode", string(definition.ExitMode)},
		{"set-option", "-p", "-t", pane.ID, "@muxpad_finished", "0"},
	} {
		if err := c.runBang(args...); err != nil {
			return err
		}
	}
	if err := c.runBang("respawn-pane", "-k", "-t", pane.ID, "-c", directory, c.WrappedCommand(definition.Command, definition.ExitMode)); err != nil {
		return err
	}
	if err := c.runBang("select-pane", "-t", pane.ID, "-T", pane.Name); err != nil {
		return err
	}
	return c.Focus(pane)
}

func (c *Client) Attach(session string) error {
	args := append(c.Prefix, "attach-session", "-t", session)
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, args, os.Environ())
}

func (c *Client) Switch(session string) error {
	return c.runBang("switch-client", "-t", session)
}

func (c *Client) PopupMenu(program string) error {
	command := "MUXPAD_POPUP=1 " + shellwords.Escape(program) + " menu"
	return c.runBang("display-popup", "-E", "-w", "96", "-h", "22", "-T", " Muxpad ", command)
}

func (c *Client) KillSession(session string) error {
	return c.runBang("kill-session", "-t", session)
}

func (c *Client) WrappedCommand(command string, exitMode config.ExitMode) string {
	if exitMode == config.ExitClose {
		return "sh -c " + shellwords.Escape("exec "+command)
	}
	parts := make([]string, 0, len(c.Prefix))
	for _, part := range c.Prefix {
		parts = append(parts, shellwords.Escape(part))
	}
	tmux := strings.Join(parts, " ")
	drop := `muxpad_seed_history() {
  [ -n "$muxpad_command" ] || return 0
  case "${SHELL##*/}" in
    zsh) printf ': %s:0;%s\n' "$(date +%s 2>/dev/null || echo 0)" "$muxpad_command" >> "${HISTFILE:-$HOME/.zsh_history}" 2>/dev/null ;;
    bash) printf '%s\n' "$muxpad_command" >> "${HISTFILE:-$HOME/.bash_history}" 2>/dev/null ;;
  esac
}
muxpad_drop() {
  ` + tmux + ` set-option -p -t "$TMUX_PANE" @muxpad_finished 1
  muxpad_seed_history
  exec "${SHELL:-/bin/sh}"
}
trap 'status=$?; muxpad_drop' INT TERM`
	tail := "muxpad_drop"
	if exitMode == config.ExitKeepOnError {
		tail = `if [ $status -eq 0 ]; then ` + tmux + ` kill-pane -t "$TMUX_PANE"; else muxpad_drop; fi`
	}
	inner := "muxpad_command=" + shellwords.Escape(command) + "\n" + drop + "\n( " + command + "\n); status=$?\n" + tail
	return "sh -c " + shellwords.Escape(inner)
}

func (c *Client) syncPath(session string) error {
	return c.runBang("set-environment", "-t", session, "PATH", os.Getenv("PATH"))
}

func (c *Client) nextWindowIndex(session string) int {
	out, err := c.capture("list-windows", "-t", session, "-F", "#{window_index}")
	if err != nil {
		return 1
	}
	max := 0
	for _, line := range lines(out) {
		n, _ := strconv.Atoi(strings.TrimSpace(line))
		if n > max {
			max = n
		}
	}
	return max + 1
}

func (c *Client) capture(args ...string) (string, error) {
	result := c.run(append(c.Prefix, args...)...)
	if !result.OK {
		return "", fmt.Errorf("tmux %s failed: %s", first(args), strings.TrimSpace(result.Stderr))
	}
	return result.Stdout, nil
}

func (c *Client) captureAllow(args ...string) (string, error) {
	result := c.run(append(c.Prefix, args...)...)
	if !result.OK {
		return result.Stdout, nil
	}
	return result.Stdout, nil
}

func (c *Client) runBang(args ...string) error {
	result := c.run(append(c.Prefix, args...)...)
	if !result.OK {
		return fmt.Errorf("tmux %s failed: %s", first(args), strings.TrimSpace(result.Stderr))
	}
	return nil
}

func (c *Client) run(args ...string) Result {
	if c.Run != nil {
		return c.Run(args...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err == nil {
		return Result{Stdout: string(out), OK: true}
	}
	stderr := ""
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = string(exitErr.Stderr)
	}
	return Result{Stdout: string(out), Stderr: stderr, OK: false}
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func lines(value string) []string {
	value = strings.TrimRight(value, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
