package app

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sbx/internal/process"
)

func TestCloneBranchFallsBackToExistingBranch(t *testing.T) {
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mirror := func(name, defaultBranch string, others ...string) string {
		upstream := filepath.Join(root, name)
		git(root, "init", "-q", "-b", defaultBranch, upstream)
		git(upstream, "-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "-q", "--allow-empty", "-m", "init")
		for _, branch := range others {
			git(upstream, "branch", branch)
		}
		cache := upstream + ".git"
		git(root, "clone", "-q", "--mirror", upstream, cache)
		return cache
	}
	app := &App{Err: io.Discard, runner: process.Runner{Out: io.Discard, Err: io.Discard}}
	for _, test := range []struct {
		cache, configured, want string
	}{
		{mirror("master-only", "master"), "main", "master"},
		{mirror("has-both", "master", "main"), "main", "main"},
		{mirror("configured", "main", "develop"), "develop", "develop"},
		{mirror("trunk", "trunk"), "main", "trunk"},
	} {
		got, err := app.cloneBranch(context.Background(), test.cache, test.configured)
		if err != nil || got != test.want {
			t.Errorf("%s: got %q, %v; want %q", filepath.Base(test.cache), got, err, test.want)
		}
	}
	empty := filepath.Join(root, "empty.git")
	git(root, "init", "-q", "--bare", empty)
	if _, err := app.cloneBranch(context.Background(), empty, "main"); err == nil || !strings.Contains(err.Error(), "main, master") {
		t.Errorf("empty mirror: %v", err)
	}
}
