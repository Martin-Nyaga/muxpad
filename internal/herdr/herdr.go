package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/backend"
)

type Result struct {
	Stdout string
	Stderr string
	OK     bool
}

type Runner func(args ...string) Result

type Client struct {
	Bin string
	Run Runner
}

func New() *Client {
	return &Client{Bin: envDefault("HERDR_BIN_PATH", "herdr")}
}

func (c *Client) CreateTab(spec backend.CreateTabSpec) (backend.Pane, error) {
	args := []string{"tab", "create"}
	if spec.Directory != "" {
		args = append(args, "--cwd", spec.Directory)
	}
	if spec.Label != "" {
		args = append(args, "--label", spec.Label)
	}
	if spec.Focus {
		args = append(args, "--focus")
	} else {
		args = append(args, "--no-focus")
	}
	out, err := c.capture(args...)
	if err != nil {
		return backend.Pane{}, err
	}
	if id := paneID(out); id != "" {
		return backend.Pane{ID: id}, nil
	}
	out, err = c.capture("pane", "current", "--current")
	if err != nil {
		return backend.Pane{}, err
	}
	if id := paneID(out); id != "" {
		return backend.Pane{ID: id}, nil
	}
	return backend.Pane{}, errors.New("herdr tab create did not report a pane id")
}

func (c *Client) RunInPane(pane backend.Pane, command string) error {
	if pane.ID == "" {
		return errors.New("pane id is required")
	}
	if strings.TrimSpace(command) == "" {
		return errors.New("command is required")
	}
	_, err := c.capture("pane", "run", pane.ID, command)
	return err
}

func (c *Client) capture(args ...string) (string, error) {
	result := c.run(append([]string{c.bin()}, args...)...)
	if !result.OK {
		return "", fmt.Errorf("herdr %s failed: %s", first(args), strings.TrimSpace(result.Stderr))
	}
	return result.Stdout, nil
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

func (c *Client) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return envDefault("HERDR_BIN_PATH", "herdr")
}

func paneID(output string) string {
	var value any
	if json.Unmarshal([]byte(output), &value) == nil {
		if id := findJSONPaneID(value); id != "" {
			return id
		}
	}
	for _, pattern := range []string{
		`(?m)\bpane_id\b\s*[:=]\s*"?([A-Za-z0-9._:-]+)"?`,
		`(?m)\bpane\b\s*[:=]\s*"?([A-Za-z0-9._:-]+)"?`,
	} {
		if match := regexp.MustCompile(pattern).FindStringSubmatch(output); len(match) == 2 {
			return strings.Trim(match[1], `"'`)
		}
	}
	return ""
}

func findJSONPaneID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if id, ok := typed["pane_id"].(string); ok && id != "" {
			return id
		}
		for _, child := range typed {
			if id := findJSONPaneID(child); id != "" {
				return id
			}
		}
	case []any:
		for _, child := range typed {
			if id := findJSONPaneID(child); id != "" {
				return id
			}
		}
	}
	return ""
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func first(values []string) string {
	if len(values) == 0 {
		return "command"
	}
	return values[0]
}
