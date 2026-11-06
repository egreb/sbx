package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"sbx/internal/process"
)

// mountArguments returns the extra workspace paths for sbx create. Mounts are
// read-only unless configured as writable.
func (app *App) mountArguments() ([]string, error) {
	var args []string
	for _, mount := range app.config.Mounts {
		info, err := os.Stat(mount.Path)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("configured mount is not a directory: %s", mount.Path)
		}
		if mount.Writable {
			args = append(args, mount.Path)
		} else {
			args = append(args, mount.Path+":ro")
		}
	}
	return args, nil
}

// mountNotes describes the mounts for the agent's global instructions.
func (app *App) mountNotes() string {
	if len(app.config.Mounts) == 0 {
		return ""
	}
	var text strings.Builder
	text.WriteString("\n## Mounted host directories\n\n")
	text.WriteString("These host directories are available in this sandbox at the same paths. Search them with your file tools when they may hold relevant context.\n\n")
	for _, mount := range app.config.Mounts {
		access := "read-only"
		if mount.Writable {
			access = "writable"
		}
		fmt.Fprintf(&text, "- `%s` (%s)", mount.Path, access)
		if mount.Description != "" {
			text.WriteString(": " + mount.Description)
		}
		text.WriteString("\n")
	}
	return text.String()
}

// Appends to the agent's global instructions file: ~/.claude/CLAUDE.md (or
// $CLAUDE_CONFIG_DIR/CLAUDE.md) for Claude, $CODEX_HOME/AGENTS.md for Codex.
const writeMountNotesScript = `import os, pathlib, sys
agent, text = sys.argv[1:3]
home = pathlib.Path.home()
if agent == "claude":
    directory = pathlib.Path(os.environ.get("CLAUDE_CONFIG_DIR") or home / ".claude")
    path = directory / "CLAUDE.md"
else:
    directory = pathlib.Path(os.environ.get("CODEX_HOME") or home / ".codex")
    path = directory / "AGENTS.md"
directory.mkdir(parents=True, exist_ok=True)
with open(path, "a") as output:
    output.write(text)
`

func (app *App) writeMountNotes(ctx context.Context, name, agent string) error {
	text := app.mountNotes()
	if text == "" {
		return nil
	}
	args := []string{"exec", name, "--", "python3", "-c", writeMountNotesScript, agent, text}
	if err := app.runner.Run(ctx, app.native, args, process.Options{}); err != nil {
		return fmt.Errorf("cannot write mount notes in sandbox %s: %w", name, err)
	}
	return nil
}
