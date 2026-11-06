package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func loadWithConfig(t *testing.T, body string) (Config, string, error) {
	t.Helper()
	directory := t.TempDir()
	configDirectory := filepath.Join(directory, "config")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "config.yaml"), []byte("host_shell: /bin/sh\n"+body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", directory)
	t.Setenv("SBX_CONFIG_DIR", configDirectory)
	t.Setenv("SBX_CONFIG_FILE", "")
	loaded, err := Load("/unused")
	return loaded, directory, err
}

func TestLoadMounts(t *testing.T) {
	loaded, home, err := loadWithConfig(t, "mounts:\n  - path: ~/vaults\n    description: Obsidian notes\n  - path: /srv/shared/\n    writable: true\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []Mount{
		{Path: filepath.Join(home, "vaults"), Description: "Obsidian notes"},
		{Path: "/srv/shared", Writable: true},
	}
	if !reflect.DeepEqual(loaded.Mounts, want) {
		t.Fatalf("got %#v, want %#v", loaded.Mounts, want)
	}
}

func TestLoadMountsUnsetIsEmpty(t *testing.T) {
	loaded, _, err := loadWithConfig(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Mounts) != 0 {
		t.Fatalf("expected no mounts, got %#v", loaded.Mounts)
	}
}

func TestLoadMountsRejectsInvalid(t *testing.T) {
	for body, message := range map[string]string{
		"mounts: ~/vaults\n":                          "must be a sequence",
		"mounts:\n  - description: x\n":               "needs a path",
		"mounts:\n  - path: relative\n":               "must be absolute",
		"mounts:\n  - path: /a:b\n":                   "cannot contain",
		"mounts:\n  - path: /a\n    writable: yes\n":  "true or false",
		"mounts:\n  - path: /a\n    readonly: true\n": "unknown key",
		"mounts:\n  - path: /a\n  - path: /a/\n":      "duplicate",
	} {
		if _, _, err := loadWithConfig(t, body); err == nil || !strings.Contains(err.Error(), message) {
			t.Errorf("%q: expected error containing %q, got %v", body, message, err)
		}
	}
}
