package app

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sbx/internal/config"
)

func TestMountArgumentsAndNotes(t *testing.T) {
	directory := t.TempDir()
	app := &App{config: config.Config{Mounts: []config.Mount{
		{Path: directory, Description: "Obsidian notes"},
		{Path: "/", Writable: true},
	}}}
	args, err := app.mountArguments()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{directory + ":ro", "/"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("got %q, want %q", args, want)
	}
	notes := app.mountNotes()
	for _, want := range []string{"`" + directory + "` (read-only): Obsidian notes\n", "`/` (writable)\n"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("notes missing %q:\n%s", want, notes)
		}
	}
}

func TestMountArgumentsRejectMissingDirectory(t *testing.T) {
	app := &App{config: config.Config{Mounts: []config.Mount{{Path: filepath.Join(t.TempDir(), "missing")}}}}
	if _, err := app.mountArguments(); err == nil {
		t.Fatal("expected an error for a missing mount directory")
	}
	if (&App{}).mountNotes() != "" {
		t.Fatal("expected no notes without mounts")
	}
}
