package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Martin-Nyaga/muxpad/internal/backend"
	"github.com/Martin-Nyaga/muxpad/internal/config"
)

const stateFileName = "state.json"

type pluginState struct {
	Panes map[string]paneState `json:"panes"`
}

type paneState struct {
	DefinitionID string          `json:"definition_id"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name,omitempty"`
	Command      string          `json:"command,omitempty"`
	Directory    string          `json:"directory,omitempty"`
	ExitMode     config.ExitMode `json:"exit_mode,omitempty"`
	Workspace    string          `json:"workspace,omitempty"`
}

func (c *Client) backendPanes(workspace string, infos []paneInfo) ([]backend.Pane, error) {
	live := make(map[string]bool, len(infos))
	for _, pane := range infos {
		if pane.PaneID != "" {
			live[pane.PaneID] = true
		}
	}

	state, err := c.loadState()
	if err != nil {
		return nil, err
	}
	if state.reconcile(live) {
		if err := c.saveState(state); err != nil {
			return nil, err
		}
	}

	panes := make([]backend.Pane, 0, len(infos))
	for _, pane := range infos {
		if workspace != "" && pane.WorkspaceID != workspace {
			continue
		}
		item := pane.backendPane()
		if meta, ok := state.Panes[item.ID]; ok {
			meta.apply(&item)
		}
		panes = append(panes, item)
	}
	return panes, nil
}

func (c *Client) recordLaunch(spec backend.LaunchSpec, paneID, directory string) error {
	if paneID == "" {
		return nil
	}
	state, err := c.loadState()
	if err != nil {
		return err
	}
	state.Panes[paneID] = paneState{
		DefinitionID: spec.Definition.ID,
		Kind:         spec.Kind,
		Name:         spec.Name,
		Command:      spec.Definition.Command,
		Directory:    directory,
		ExitMode:     spec.Definition.ExitMode,
		Workspace:    spec.Workspace,
	}
	return c.saveState(state)
}

func (state *pluginState) reconcile(live map[string]bool) bool {
	if state.Panes == nil {
		state.Panes = map[string]paneState{}
	}
	changed := false
	for paneID := range state.Panes {
		if !live[paneID] {
			delete(state.Panes, paneID)
			changed = true
		}
	}
	return changed
}

func (meta paneState) apply(pane *backend.Pane) {
	pane.Kind = meta.Kind
	pane.DefinitionID = meta.DefinitionID
	if meta.Name != "" {
		pane.Name = meta.Name
	}
	if meta.Command != "" {
		pane.CurrentCommand = meta.Command
	}
	if pane.CurrentPath == "" && meta.Directory != "" {
		pane.CurrentPath = meta.Directory
	}
	if pane.Workspace == "" && meta.Workspace != "" {
		pane.Workspace = meta.Workspace
	}
}

func (c *Client) loadState() (*pluginState, error) {
	path := c.statePath()
	state := &pluginState{Panes: map[string]paneState{}}
	if path == "" {
		return state, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read herdr plugin state: %w", err)
	}
	if len(data) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("parse herdr plugin state: %w", err)
	}
	if state.Panes == nil {
		state.Panes = map[string]paneState{}
	}
	return state, nil
}

func (c *Client) saveState(state *pluginState) error {
	path := c.statePath()
	if path == "" {
		return nil
	}
	if state.Panes == nil {
		state.Panes = map[string]paneState{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create herdr plugin state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode herdr plugin state: %w", err)
	}
	data = append(data, '\n')
	tmp := path + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write herdr plugin state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace herdr plugin state: %w", err)
	}
	return nil
}

func (c *Client) statePath() string {
	dir := c.StateDir
	if dir == "" {
		dir = os.Getenv("HERDR_PLUGIN_STATE_DIR")
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, stateFileName)
}
