package app

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sbx/internal/config"
	"sbx/internal/process"
)

func TestAddClonesSeveralProjects(t *testing.T) {
	root := t.TempDir()
	var manifest strings.Builder
	manifest.WriteString("projects:\n")
	for _, name := range []string{"api", "web", "docs"} {
		upstream := filepath.Join(root, "upstream", name)
		for _, args := range [][]string{
			{"init", "-q", "-b", "main", upstream},
			{"-C", upstream, "-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "-q", "--allow-empty", "-m", "init"},
		} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		fmt.Fprintf(&manifest, "  - name: %s\n    repo: %s\n", name, upstream)
	}
	projectsFile := filepath.Join(root, "projects.yaml")
	if err := os.WriteFile(projectsFile, []byte(manifest.String()), 0600); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(root, "workspaces")
	workspace := filepath.Join(workspaceRoot, "example")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeMarker(workspace, "example"); err != nil {
		t.Fatal(err)
	}
	var notes bytes.Buffer
	app := &App{Program: "sbx", Out: io.Discard, Err: &notes, runner: process.Runner{Out: io.Discard, Err: io.Discard}, config: config.Config{
		WorkspaceRoot: workspaceRoot, GitCacheRoot: filepath.Join(root, "cache"), ProjectsFile: projectsFile,
	}}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(workspace, name, ".git"))
		return err == nil
	}

	if err := app.cmdAdd([]string{"--workspace", "example", "web", "api", "web"}); err != nil {
		t.Fatal(err)
	}
	if !exists("api") || !exists("web") || exists("docs") {
		t.Fatal("named projects were not the ones cloned")
	}
	if err := app.cmdAdd([]string{"--workspace", "example", "api"}); err == nil {
		t.Fatal("cloned over an existing project")
	}
	if err := app.cmdAdd([]string{"--workspace", "example", "--all"}); err != nil {
		t.Fatal(err)
	}
	if !exists("docs") {
		t.Fatal("--all skipped a missing project")
	}
	notes.Reset()
	if err := app.cmdAdd([]string{"--workspace", "example", "--all"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notes.String(), "already in this workspace") {
		t.Fatalf("notes = %q", notes.String())
	}
	for _, args := range [][]string{{"--workspace", "example", "--all", "api"}, {"--workspace", "example", "--bogus"}, {"--workspace", "example", "missing"}} {
		if err := app.cmdAdd(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
