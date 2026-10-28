package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInitializeClaudeConfigPreservesConfig(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	for _, initial := range []string{"", `{"theme":"dark","hasCompletedOnboarding":true,"custom":{"keep":42}}`, "invalid JSON"} {
		t.Run(initial, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", dir)
			path := filepath.Join(dir, ".claude.json")
			if initial != "" {
				if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
					t.Fatal(err)
				}
			}
			output, err := exec.Command(python, "-c", initializeClaudeConfigScript, "light", "claude-opus-5-5", "high").CombinedOutput()
			if initial == "invalid JSON" {
				data, _ := os.ReadFile(path)
				if err == nil || string(data) != initial {
					t.Fatal("invalid config should fail without overwriting it")
				}
				return
			}
			if err != nil {
				t.Fatalf("%v: %s", err, output)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got["theme"] != "light" {
				t.Fatalf("theme: %v", got)
			}
			if initial != "" && (got["hasCompletedOnboarding"] != true || got["custom"].(map[string]any)["keep"] != float64(42)) {
				t.Fatalf("lost existing preferences: %v", got)
			}
			data, err = os.ReadFile(filepath.Join(dir, "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			var settings map[string]any
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatal(err)
			}
			if settings["model"] != "claude-opus-5-5" || settings["effortLevel"] != "high" {
				t.Fatalf("settings: %v", settings)
			}
		})
	}
}
