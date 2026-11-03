package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sbx/internal/process"
	"sort"
	"time"
)

// With multiplexer: rex, workspace sessions are owned by the Rex actions in
// rex/sbx.lua; sbx only asks for them by name.

func (app *App) usingRex() bool {
	return app.config.Multiplexer == "rex"
}

// rexBinary prefers the CLI matching the server of the terminal sbx runs in.
func rexBinary() string {
	if dir := os.Getenv("REX_BIN_DIR"); dir != "" {
		return filepath.Join(dir, "rex")
	}
	return "rex"
}

func rexActionArgs(action string, args map[string]string) []string {
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	argv := []string{"do", action}
	for _, key := range keys {
		argv = append(argv, key+"="+args[key])
	}
	return argv
}

func (app *App) rexAction(ctx context.Context, action string, args map[string]string) error {
	return app.runner.Run(ctx, rexBinary(), rexActionArgs(action, args), process.Options{Stdout: io.Discard})
}

// rexResult runs an action and returns the string fields of its JSON result.
func (app *App) rexResult(ctx context.Context, action string, args map[string]string) (map[string]string, error) {
	out, err := app.runner.Output(ctx, rexBinary(), rexActionArgs(action, args), process.Options{})
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("unexpected result from %s: %q", action, out)
	}
	fields := map[string]string{}
	for key, value := range result {
		if text, ok := value.(string); ok {
			fields[key] = text
		}
	}
	return fields, nil
}

// rexSelectScript asks every connected app to show a session, and the tab
// rex.args.wid when given. Run from the CLI, a queued action would go to the
// CLI's own client, so the app is addressed directly.
const rexSelectScript = `
local args = { session_id = rex.args.sid, window_id = rex.args.wid }
local list, err = rex.call("client.list")
if list == nil then error(err, 0) end
local found = false
for _, client in ipairs(list.clients or {}) do
  if client.info and client.info.kind == "app" then
    local _, e = rex.call("client.perform", { client_id = client.client_id, action = "session.select", args = args })
    if e ~= nil then error(e, 0) end
    found = true
  end
end
if not found then error("no Rex app is connected", 0) end
`

// rexSelect switches the app to a session, and to one of its tabs when
// windowID is set. The app drops a switch to something it has not heard of
// yet, so give it a moment to learn about what was just created.
func (app *App) rexSelect(ctx context.Context, sessionID, windowID string) error {
	time.Sleep(200 * time.Millisecond)
	args := []string{"do", "-e", rexSelectScript, "sid=" + sessionID}
	if windowID != "" {
		args = append(args, "wid="+windowID)
	}
	if err := app.runner.Run(ctx, rexBinary(), args, process.Options{Stdout: io.Discard}); err != nil {
		return fmt.Errorf("could not switch the Rex app (is Remote Control on?): %w", err)
	}
	return nil
}

func (app *App) rexOpenWorkspace(ctx context.Context, name, path, prompt string, open bool) error {
	args := map[string]string{"name": name, "path": path}
	if prompt != "" {
		args["prompt"] = prompt
	}
	result, err := app.rexResult(ctx, "sbx_open_workspace", args)
	if err != nil {
		return err
	}
	if !open {
		return nil
	}
	return app.rexSelect(ctx, result["session_id"], result["window_id"])
}

// rexOpenProject opens a directory in a tab of the Rex session sbx runs in.
func (app *App) rexOpenProject(ctx context.Context, dir string) error {
	session := os.Getenv("REX_SESSION")
	if session == "" {
		return errors.New("this command must run inside Rex")
	}
	result, err := app.rexResult(ctx, "sbx_open_project", map[string]string{"cwd": dir})
	if err != nil {
		return err
	}
	return app.rexSelect(ctx, session, result["window_id"])
}

func (app *App) cmdRexOpenProject(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("Usage: %s __rex-open-project DIRECTORY", app.Program)
	}
	return app.rexOpenProject(context.Background(), args[0])
}

func (app *App) rexCloseWorkspace(ctx context.Context, name string) {
	if err := app.rexAction(ctx, "sbx_close_workspace", map[string]string{"name": name}); err != nil {
		app.note("Could not close Rex session %s: %v", name, err)
	}
}

func (app *App) chooseWorkspace(ctx context.Context, prompt string) (string, bool, error) {
	names, err := app.workspaceCandidates()
	if err != nil {
		return "", false, err
	}
	if len(names) == 0 {
		return "", false, errors.New("no workspaces")
	}
	selection, ok, err := app.chooseWithFZF(ctx, prompt, names, false)
	if err != nil || !ok {
		return "", ok, err
	}
	return selection[0], true, nil
}
