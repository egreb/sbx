package app

import (
	"context"
	"fmt"
	"strings"

	"sbx/internal/process"
)

// cmdApprove lists pending policy approvals in fzf and responds to each
// selected one. Docker's table output is used as-is for display; the first
// column is the approval ID.
func (app *App) cmdApprove(args []string) error {
	option := "allow"
	for len(args) > 0 {
		switch args[0] {
		case "--option":
			if len(args) < 2 {
				return fmt.Errorf("--option requires a value")
			}
			option = args[1]
			args = args[2:]
		case "--dismiss":
			option = "dismiss"
			args = args[1:]
		case "-h", "--help":
			fmt.Fprintf(app.Out, "Usage: %s approve [--option ID|--dismiss]\n", app.Program)
			return nil
		default:
			return fmt.Errorf("unknown argument: %s", args[0])
		}
	}
	if err := app.needNative(); err != nil {
		return err
	}
	if err := app.checkCommand("fzf"); err != nil {
		return err
	}

	ctx := context.Background()
	output, err := app.runner.Output(ctx, app.native, []string{"policy", "approval", "ls"}, process.Options{})
	if err != nil {
		return fmt.Errorf("list approvals: %w", err)
	}
	header, rows := parseApprovalTable(output)
	if len(rows) == 0 {
		app.note("No pending approvals")
		return nil
	}

	preview := posixQuote(app.native) + " policy approval inspect {1}"
	fzfArgs := []string{
		"--multi",
		"--header-lines=1",
		"--prompt", option + "> ",
		"--header", "Tab/Space select, Enter " + option + ", Esc cancel",
		"--bind", "space:toggle+down,ctrl-a:select-all",
		"--preview", preview,
		"--preview-window", "down,40%,wrap",
		"--height=100%", "--layout=reverse", "--border",
	}
	input := strings.Join(append([]string{header}, rows...), "\n") + "\n"
	selected, err := app.runner.Output(ctx, "fzf", fzfArgs, process.Options{Stdin: strings.NewReader(input)})
	if err != nil {
		// Esc or no match.
		return nil
	}

	var failed int
	for _, line := range outputLines(selected) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		id := fields[0]
		if err := app.run(ctx, app.native, "policy", "approval", "respond", id, "--option", option); err != nil {
			app.note("%s %s: %v", option, id, err)
			failed++
			continue
		}
		app.note("%s: %s", option, strings.Join(fields[1:min(3, len(fields))], " "))
	}
	if failed > 0 {
		return fmt.Errorf("%d approval(s) failed", failed)
	}
	return nil
}

// parseApprovalTable splits `sbx policy approval ls` output into its header
// line and approval rows, dropping blank lines and leading indentation.
func parseApprovalTable(output string) (string, []string) {
	var header string
	var rows []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if header == "" {
			if !strings.HasPrefix(line, "APPROVAL") {
				// Not a table (e.g. an empty-state message).
				return "", nil
			}
			header = line
			continue
		}
		rows = append(rows, line)
	}
	return header, rows
}

func posixQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
