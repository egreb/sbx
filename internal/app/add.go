package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/x/term"

	"sbx/internal/config"
	"sbx/internal/tui"
)

func (app *App) cmdAdd(args []string) error {
	usage := fmt.Errorf("Usage: %s add [--workspace name] [--all | project...]", app.Program)
	workspaceName := ""
	all := false
	var names []string
	for len(args) > 0 {
		switch args[0] {
		case "--workspace":
			if len(args) < 2 {
				return usage
			}
			workspaceName, args = args[1], args[2:]
		case "--all":
			all, args = true, args[1:]
		default:
			if strings.HasPrefix(args[0], "-") {
				return usage
			}
			names, args = append(names, args[0]), args[1:]
		}
	}
	if all && len(names) > 0 {
		return usage
	}
	ctx := context.Background()
	workspacePath := filepath.Join(app.config.WorkspaceRoot, workspaceName)
	if workspaceName == "" {
		current, err := app.currentManagedContext(ctx)
		if err != nil {
			return err
		}
		workspaceName, workspacePath = current.WorkspaceName, current.Workspace
	} else if err := validateName(workspaceName); err != nil {
		return err
	}
	workspace, err := app.assertSafeWorkspace(workspacePath, workspaceName)
	if err != nil {
		return err
	}
	projects, err := app.loadProjects()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		var missing []string
		for _, project := range projects {
			_, err := os.Lstat(filepath.Join(workspace, project.Name))
			if os.IsNotExist(err) {
				missing = append(missing, project.Name)
			} else if err != nil {
				return err
			}
		}
		if len(missing) == 0 {
			app.note("All configured projects are already in this workspace.")
			return nil
		}
		if all {
			names = missing
		} else {
			selection, ok, err := app.chooseWithFZF(ctx, "Add projects > ", missing, true)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			names = selection
		}
	}

	byName := make(map[string]config.Project, len(projects))
	for _, project := range projects {
		byName[project.Name] = project
	}
	var selected []config.Project
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		project, ok := byName[name]
		if !ok {
			return fmt.Errorf("project is not configured: %s", name)
		}
		if err := validateName(name); err != nil {
			return err
		}
		selected = append(selected, project)
	}
	if err := app.checkCommand("git"); err != nil {
		return err
	}

	// Each step reserves its destination atomically, so it never clones over
	// files or symlinks, and records whether it cloned so that the projects
	// that did are still set up when another fails.
	var (
		mutex  sync.Mutex
		cloned = make(map[string]bool, len(selected))
		steps  = make([]tui.CreationStep, 0, len(selected))
	)
	for _, project := range selected {
		destination := filepath.Join(workspace, project.Name)
		steps = append(steps, tui.CreationStep{Name: project.Name, Run: func() error {
			if err := os.Mkdir(destination, 0755); err != nil {
				return fmt.Errorf("cannot add %s (path may already exist): %w", project.Name, err)
			}
			if err := app.cloneProjectFromCache(ctx, project, destination); err != nil {
				// Remove only an empty reservation; retain any partial clone for inspection.
				_ = os.Remove(destination)
				return fmt.Errorf("%s: %w; inspect %s before retrying", project.Name, err, destination)
			}
			mutex.Lock()
			cloned[project.Name] = true
			mutex.Unlock()
			return nil
		}})
	}
	cloneErr := app.runCloneSteps("Adding to "+workspaceName, steps)

	var errs []error
	if cloneErr != nil {
		errs = append(errs, cloneErr)
	}
	var added []string
	for _, project := range selected {
		if !cloned[project.Name] {
			continue
		}
		destination := filepath.Join(workspace, project.Name)
		if err := app.bootstrapProject(ctx, project, destination); err != nil {
			errs = append(errs, fmt.Errorf("project cloned at %s but setup failed: %w", destination, err))
			continue
		}
		added = append(added, project.Name)
	}
	if len(added) > 0 {
		app.note("Added %s to workspace %s.", strings.Join(added, ", "), workspaceName)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	// Open a tab for a single project; several would flood the session, and
	// the workspace project picker opens the others.
	if app.usingRex() && len(added) == 1 {
		return app.rexOpenProject(ctx, filepath.Join(workspace, added[0]))
	}
	return nil
}

// runCloneSteps runs steps in parallel, with a progress view on a terminal.
func (app *App) runCloneSteps(title string, steps []tui.CreationStep) error {
	output, ok := app.Out.(*os.File)
	if !ok || !term.IsTerminal(output.Fd()) {
		return runParallel(steps, tui.CreationParallelism)
	}
	var logs creationLog
	previousRunner, previousErr := app.runner, app.Err
	app.runner.Out, app.runner.Err, app.Err = &logs, &logs, &logs
	err := tui.RunCreation(app.Out, title, steps)
	app.runner, app.Err = previousRunner, previousErr
	if err != nil {
		fmt.Fprint(app.Err, logs.String())
	}
	return err
}
