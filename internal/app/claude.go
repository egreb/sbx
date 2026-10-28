package app

import (
	"context"
	"fmt"

	"sbx/internal/process"
)

// Runs only during creation, before Claude can read or write its config.
// Theme is a user preference in .claude.json; model and effort are defaults in
// .claude/settings.json, so /model and /effort can still change them later.
// Empty arguments leave the corresponding value untouched.
const initializeClaudeConfigScript = `import json, os, pathlib, sys, tempfile
theme, model, effort = sys.argv[1:4]
configured = os.environ.get("CLAUDE_CONFIG_DIR")
home = pathlib.Path.home()
def update(directory, name, values):
    values = {key: value for key, value in values.items() if value}
    if not values:
        return
    path = directory / name
    directory.mkdir(parents=True, exist_ok=True)
    data = json.loads(path.read_text()) if path.exists() else {}
    data.update(values)
    fd, temporary = tempfile.mkstemp(prefix="." + name + "-", dir=directory)
    try:
        with os.fdopen(fd, "w") as output:
            json.dump(data, output, indent=2)
            output.write("\n")
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
update(pathlib.Path(configured) if configured else home, ".claude.json", {"theme": theme})
update(pathlib.Path(configured) if configured else home / ".claude", "settings.json", {"model": model, "effortLevel": effort})
`

func (app *App) initializeClaudeConfig(ctx context.Context, name string) error {
	args := []string{"exec", name, "--", "python3", "-c", initializeClaudeConfigScript, app.config.ClaudeTheme, app.config.ClaudeModel, app.config.ClaudeEffort}
	if err := app.runner.Run(ctx, app.native, args, process.Options{}); err != nil {
		return fmt.Errorf("cannot initialize Claude config in sandbox %s: %w", name, err)
	}
	return nil
}
