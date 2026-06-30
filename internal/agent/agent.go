package agent

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/shellwords"
)

type Pane struct {
	ID  string
	PID string
}

type ProcessInfo struct {
	PID       int
	ParentPID int
	Command   string
	Arguments string
}

type CaptureFunc func() (stdout string, stderr string, ok bool)

type Discovery struct {
	Capture CaptureFunc
}

var codexNoninteractive = map[string]bool{
	"app": true, "app-server": true, "apply": true, "archive": true, "cloud": true,
	"completion": true, "debug": true, "delete": true, "doctor": true, "exec": true,
	"mcp": true, "mcp-server": true, "plugin": true, "remote-control": true, "sandbox": true,
	"unarchive": true, "update": true,
}

var optionsWithValues = map[string]bool{
	"-a": true, "--ask-for-approval": true, "-C": true, "--cd": true, "-c": true, "--config": true,
	"-i": true, "--image": true, "-m": true, "--model": true, "-p": true, "--profile": true,
	"-s": true, "--sandbox": true, "--add-dir": true, "--disable": true, "--enable": true,
	"--remote": true, "--remote-auth-token-env": true,
}

func (d Discovery) Detect(panes []Pane) map[string]string {
	processes := d.processTable()
	if len(processes) == 0 {
		return map[string]string{}
	}
	children := map[int][]ProcessInfo{}
	for _, process := range processes {
		children[process.ParentPID] = append(children[process.ParentPID], process)
	}
	out := map[string]string{}
	for _, pane := range panes {
		pid, _ := strconv.Atoi(pane.PID)
		if provider := detectTree(pid, processes, children); provider != "" {
			out[pane.ID] = provider
		}
	}
	return out
}

func (d Discovery) processTable() map[int]ProcessInfo {
	capture := d.Capture
	if capture == nil {
		capture = func() (string, string, bool) {
			cmd := exec.Command("ps", "-A", "-ww", "-o", "pid=,ppid=,comm=,args=")
			out, err := cmd.Output()
			return string(out), "", err == nil
		}
	}
	stdout, _, ok := capture()
	if !ok {
		return nil
	}
	linePattern := regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+(\S+)\s+(.*)$`)
	processes := map[int]ProcessInfo{}
	for _, line := range strings.Split(stdout, "\n") {
		match := linePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		pid, _ := strconv.Atoi(match[1])
		ppid, _ := strconv.Atoi(match[2])
		processes[pid] = ProcessInfo{
			PID:       pid,
			ParentPID: ppid,
			Command:   filepath.Base(match[3]),
			Arguments: match[4],
		}
	}
	return processes
}

func detectTree(rootPID int, processes map[int]ProcessInfo, children map[int][]ProcessInfo) string {
	queue := []int{rootPID}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		process, ok := processes[pid]
		if ok && claude(process) {
			return "claude"
		}
		if ok && codex(process) {
			return "codex"
		}
		for _, child := range children[pid] {
			queue = append(queue, child.PID)
		}
	}
	return ""
}

func claude(process ProcessInfo) bool {
	return process.Command == "claude" &&
		!regexp.MustCompile(`(?:^|\s)(?:-p|--print)(?:\s|=|$)`).MatchString(process.Arguments)
}

func codex(process ProcessInfo) bool {
	args, err := shellwords.Split(process.Arguments)
	if err != nil {
		return false
	}
	executable := -1
	if process.Command == "codex" {
		executable = 0
		for i, arg := range args {
			if filepath.Base(arg) == "codex" {
				executable = i
				break
			}
		}
	} else if process.Command == "node" || process.Command == "MainThread" {
		for i, arg := range args {
			if strings.HasSuffix(arg, "/@openai/codex/bin/codex.js") {
				executable = i
				break
			}
		}
	}
	if executable < 0 {
		return false
	}
	first := firstCodexArgument(args[executable+1:])
	return !codexNoninteractive[first]
}

func firstCodexArgument(args []string) string {
	for index := 0; index < len(args); {
		arg := args[index]
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
		if optionsWithValues[arg] && !strings.Contains(arg, "=") {
			index += 2
		} else {
			index++
		}
	}
	return ""
}
