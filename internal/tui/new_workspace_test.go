package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func formKey(m workspaceForm, key tea.KeyPressMsg) workspaceForm {
	next, _ := m.Update(key)
	return next.(workspaceForm)
}
func TestWorkspaceFormRevisitAndSelect(t *testing.T) {
	m := newWorkspaceForm(WorkspaceForm{Name: "test", Agent: "claude", Repositories: []Repository{{Name: "api"}, {Name: "web"}}})
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyRight})
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = formKey(m, keyPress("Fix checkout"))
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = formKey(m, keyPress(" "))
	if !m.data.Selected["api"] {
		t.Fatal("Space did not select repository")
	}
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.data.Selected["web"] {
		t.Fatal("Tab toggled a repository")
	}
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focus != 3 || m.data.Prompt != "Fix checkout" || m.data.Agent != "codex" {
		t.Fatalf("lost edits: %#v", m)
	}
	m = formKey(m, keyPress("web"))
	m = formKey(m, keyPress(" "))
	if !m.data.Selected["api"] || !m.data.Selected["web"] {
		t.Fatal("filter lost selection")
	}
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.accepted {
		t.Fatal("valid form did not submit")
	}
}
func TestWorkspaceFormValidationAndCancel(t *testing.T) {
	m := newWorkspaceForm(WorkspaceForm{Agent: "claude", ValidateName: func(s string) error {
		if s == "" {
			return errors.New("Name required")
		}
		return nil
	}, Repositories: []Repository{{Name: "api"}}})
	m.focus = 4
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.accepted || m.focus != 0 || m.err == "" {
		t.Fatal("invalid name submitted")
	}
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.accepted {
		t.Fatal("cancel submitted")
	}
}
func TestWorkspaceFormWithoutRepositories(t *testing.T) {
	m := newWorkspaceForm(WorkspaceForm{Name: "valid", Agent: "claude", Repositories: []Repository{{Name: "api"}}})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "None (empty workspace)") {
		t.Fatal(view)
	}
	m.focus = 4
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.accepted || m.err != "" || len(m.selectedRepositories()) != 0 {
		t.Fatalf("form without repositories did not submit: %#v", m)
	}
}
func TestWorkspaceFormPasteAndEdit(t *testing.T) {
	m := newWorkspaceForm(WorkspaceForm{Agent: "claude"})
	m.focus = 2
	next, _ := m.Update(tea.PasteMsg{Content: "Fix\n日本語"})
	m = next.(workspaceForm)
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m = formKey(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.data.Prompt != "Fix\n日語" {
		t.Fatal(m.data.Prompt)
	}
}
func TestWorkspaceFormFitsPopup(t *testing.T) {
	m := newWorkspaceForm(WorkspaceForm{Agent: "claude", Repositories: []Repository{{Name: "api"}, {Name: "web"}, {Name: "cli"}, {Name: "docs"}, {Name: "other"}, {Name: "last"}}})
	m.width = 70
	m.height = 30
	view := ansi.Strip(m.View().Content)
	if strings.Count(view, "\n")+1 > m.height {
		t.Fatalf("form exceeds height: %d\n%s", strings.Count(view, "\n")+1, view)
	}
	if !strings.Contains(view, "selected to clone") {
		t.Fatal(view)
	}
}

func keyPress(s string) tea.KeyPressMsg { r := []rune(s); return tea.KeyPressMsg{Code: r[0], Text: s} }

func TestWorkspaceFormPromptEditing(t *testing.T) {
	m := newWorkspaceForm(WorkspaceForm{Agent: "claude", Prompt: "first line\nfix the  checkout", Editor: "true"})
	m.focus = 2
	m = formKey(m, tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if m.data.Prompt != "first line\nfix the  " {
		t.Fatalf("ctrl+w: %q", m.data.Prompt)
	}
	m = formKey(m, tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if m.data.Prompt != "first line\nfix " {
		t.Fatalf("ctrl+w over spaces: %q", m.data.Prompt)
	}
	m = formKey(m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if m.data.Prompt != "first line\n" || m.promptCursor != len([]rune("first line\n")) {
		t.Fatalf("ctrl+u: %q", m.data.Prompt)
	}

	m.focus = 3
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+g did not open the editor")
	}
	m = next.(workspaceForm)
	path := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(path, []byte("Fix the checkout\n\nThen add tests\x07\n"), 0600); err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(promptEdited{path: path})
	m = next.(workspaceForm)
	if m.data.Prompt != "Fix the checkout\n\nThen add tests" || m.focus != 2 || m.promptCursor != len([]rune(m.data.Prompt)) {
		t.Fatalf("edited prompt: %q focus=%d", m.data.Prompt, m.focus)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("prompt file was not removed")
	}
	next, _ = m.Update(promptEdited{path: path, err: errors.New("exit status 1")})
	if m = next.(workspaceForm); m.err == "" || m.data.Prompt != "Fix the checkout\n\nThen add tests" {
		t.Fatalf("failed edit changed the prompt or hid the error: %#v", m)
	}
	if view := m.View().Content; !strings.Contains(view, "Ctrl-G") {
		t.Fatal("prompt hint missing")
	}
}
