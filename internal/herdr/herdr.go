package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/backend"
	"github.com/Martin-Nyaga/muxpad/internal/config"
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

type invocationContext struct {
	WorkspaceID    string `json:"workspace_id"`
	WorkspaceCWD   string `json:"workspace_cwd"`
	FocusedPaneID  string `json:"focused_pane_id"`
	FocusedPaneCWD string `json:"focused_pane_cwd"`
}

type responseEnvelope struct {
	Result responseResult `json:"result"`
}

type responseResult struct {
	Pane  paneInfo   `json:"pane"`
	Panes []paneInfo `json:"panes"`
}

type paneInfo struct {
	PaneID        string `json:"pane_id"`
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	Focused       bool   `json:"focused"`
	CWD           string `json:"cwd"`
	ForegroundCWD string `json:"foreground_cwd"`
	Label         string `json:"label"`
	Title         string `json:"title"`
}

func New() *Client {
	return &Client{Bin: envDefault("HERDR_BIN_PATH", "herdr")}
}

func (c *Client) Inside() bool {
	return os.Getenv("HERDR_ENV") != "" || os.Getenv("HERDR_SOCKET_PATH") != ""
}

func (c *Client) CurrentWorkspace() (string, error) {
	if ctx := pluginContext(); ctx.WorkspaceID != "" {
		return ctx.WorkspaceID, nil
	}
	if value := os.Getenv("HERDR_WORKSPACE_ID"); value != "" {
		return value, nil
	}
	pane, err := c.currentPaneInfo()
	if err != nil {
		return "", err
	}
	return pane.WorkspaceID, nil
}

func (c *Client) CurrentPane() (string, error) {
	if ctx := pluginContext(); ctx.FocusedPaneID != "" {
		return ctx.FocusedPaneID, nil
	}
	if value := os.Getenv("HERDR_PANE_ID"); value != "" {
		return value, nil
	}
	pane, err := c.currentPaneInfo()
	if err != nil {
		return "", err
	}
	return pane.PaneID, nil
}

func (c *Client) WorkspaceExists(workspace string) bool {
	return workspace != ""
}

func (c *Client) Workspaces() []string {
	if workspace, err := c.CurrentWorkspace(); err == nil && workspace != "" {
		return []string{workspace}
	}
	return nil
}

func (c *Client) CreateWorkspace(name, root, projectID string) (string, error) {
	args := []string{"workspace", "create"}
	if root != "" {
		args = append(args, "--cwd", root)
	}
	if name != "" {
		args = append(args, "--label", name)
	}
	args = append(args, "--focus")
	out, err := c.capture(args...)
	if err != nil {
		return "", err
	}
	if id := workspaceID(out); id != "" {
		return id, nil
	}
	return "", nil
}

func (c *Client) ProjectContext(workspace string) string {
	return ""
}

func (c *Client) WorkspaceRoot(workspace string) string {
	ctx := pluginContext()
	if ctx.FocusedPaneCWD != "" {
		return ctx.FocusedPaneCWD
	}
	if ctx.WorkspaceCWD != "" {
		return ctx.WorkspaceCWD
	}
	panes, err := c.Panes(workspace)
	if err == nil {
		for _, pane := range panes {
			if pane.CurrentPath != "" {
				return pane.CurrentPath
			}
		}
	}
	return ""
}

func (c *Client) ManagedRoot(workspace string) string {
	return c.WorkspaceRoot(workspace)
}

func (c *Client) Panes(workspace string) ([]backend.Pane, error) {
	args := []string{"pane", "list"}
	if workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	out, err := c.capture(args...)
	if err != nil {
		return nil, err
	}
	var envelope responseEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		return nil, err
	}
	panes := make([]backend.Pane, 0, len(envelope.Result.Panes))
	for _, pane := range envelope.Result.Panes {
		panes = append(panes, pane.backendPane())
	}
	return panes, nil
}

func (c *Client) Launch(spec backend.LaunchSpec) (string, error) {
	pane, err := c.CreateTab(backend.CreateTabSpec{
		Workspace: spec.Workspace,
		Label:     spec.Name,
		Directory: resolveDirectory(spec.Root, spec.Definition),
		Focus:     true,
	})
	if err != nil {
		return "", err
	}
	if err := c.RunInPane(pane, spec.Definition.Command); err != nil {
		return "", err
	}
	return pane.ID, nil
}

func (c *Client) Focus(pane backend.Pane) error {
	if pane.ID == "" {
		return errors.New("pane id is required")
	}
	_, err := c.capture("pane", "focus", "--pane", pane.ID)
	return err
}

func (c *Client) Restart(pane backend.Pane, definition config.Definition) error {
	return errors.New("restart is not implemented for herdr")
}

func (c *Client) Attach(workspace string) error {
	return nil
}

func (c *Client) Switch(workspace string) error {
	if workspace == "" {
		return errors.New("workspace id is required")
	}
	_, err := c.capture("workspace", "focus", workspace)
	return err
}

func (c *Client) PopupMenu(program string) error {
	return c.OpenPalette()
}

func (c *Client) KillWorkspace(workspace string) error {
	if workspace == "" {
		return errors.New("workspace id is required")
	}
	_, err := c.capture("workspace", "close", workspace)
	return err
}

func (c *Client) OpenPalette() error {
	args := []string{"plugin", "pane", "open", "--plugin", "muxpad", "--entrypoint", "palette", "--placement", "overlay", "--focus"}
	if ctx := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); ctx != "" {
		args = append(args, "--env", "MUXPAD_HERDR_CONTEXT_JSON="+ctx)
	}
	_, err := c.capture(args...)
	return err
}

func (c *Client) CreateTab(spec backend.CreateTabSpec) (backend.Pane, error) {
	args := []string{"tab", "create"}
	if spec.Workspace != "" {
		args = append(args, "--workspace", spec.Workspace)
	}
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

func (c *Client) currentPaneInfo() (paneInfo, error) {
	out, err := c.capture("pane", "current", "--current")
	if err != nil {
		return paneInfo{}, err
	}
	var envelope responseEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		return paneInfo{}, err
	}
	if envelope.Result.Pane.PaneID == "" {
		return paneInfo{}, errors.New("herdr pane current did not report a pane")
	}
	return envelope.Result.Pane, nil
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

func workspaceID(output string) string {
	var value any
	if json.Unmarshal([]byte(output), &value) == nil {
		if id := findJSONField(value, "workspace_id"); id != "" {
			return id
		}
	}
	return ""
}

func findJSONPaneID(value any) string {
	return findJSONField(value, "pane_id")
}

func findJSONField(value any, field string) string {
	switch typed := value.(type) {
	case map[string]any:
		if id, ok := typed[field].(string); ok && id != "" {
			return id
		}
		for _, child := range typed {
			if id := findJSONField(child, field); id != "" {
				return id
			}
		}
	case []any:
		for _, child := range typed {
			if id := findJSONField(child, field); id != "" {
				return id
			}
		}
	}
	return ""
}

func (p paneInfo) backendPane() backend.Pane {
	cwd := p.ForegroundCWD
	if cwd == "" {
		cwd = p.CWD
	}
	return backend.Pane{
		ID:          p.PaneID,
		Workspace:   p.WorkspaceID,
		Tab:         p.TabID,
		Window:      p.TabID,
		Name:        p.Label,
		Title:       p.Title,
		CurrentPath: cwd,
	}
}

func pluginContext() invocationContext {
	raw := os.Getenv("MUXPAD_HERDR_CONTEXT_JSON")
	if raw == "" {
		raw = os.Getenv("HERDR_PLUGIN_CONTEXT_JSON")
	}
	var ctx invocationContext
	_ = json.Unmarshal([]byte(raw), &ctx)
	return ctx
}

func resolveDirectory(root string, definition config.Definition) string {
	if definition.Directory == "" {
		return root
	}
	return filepath.Join(root, definition.Directory)
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
