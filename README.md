# sbx workspace orchestrator

Host-side Go wrapper for Docker Sandboxes, with multi-project workspaces and a
tmux session picker. This source was reconstructed after accidental deletion;
see [RECOVERY.md](RECOVERY.md) for provenance and verification limits.

## Build and install

Requires Go 1.25+, tmux with popup support, Git, and Docker's native `sbx` CLI.
Fish is the default host shell. Node projects use fnm and pnpm; Go projects use Go.
The picker is built in; fzf is used for non-form project selection.

```sh
make check
make install
sbx tmux-reload
```

`make install` builds for the machine running it and atomically replaces
`~/.config/sbx/bin/sbx`. It does not overwrite your configuration, project manifest,
workspaces, or native Docker executable. Keep that directory first in PATH.
Existing `~/.config/sbx/config.yaml` and `projects.yaml` should continue to be used.
The example YAML files are for new installations, not replacements for your manifest.

## Usage

```sh
sbx new                         # form: name, agent, prompt, repository selection
                                # (no repositories creates an empty workspace;
                                # add them later with `sbx add`)
sbx new --all --no-open example
sbx new --agent codex --prompt 'Fix the failing tests' example
sbx open                        # all-workspace picker
sbx open example
sbx project backend
sbx add                         # multi-select picker (Space toggles, Ctrl-A all)
sbx add frontend backend
sbx add --all                   # every configured project not yet cloned
sbx split frontend
sbx list --names
sbx delete                      # workspace picker, then exact-name confirmation
sbx delete example              # requires typing the exact workspace name
sbx delete dotfiles             # sandbox outside ~/sandboxes or with a missing
                                # workspace: removes only the sandbox
sbx pick                        # this wrapper's picker (`sbx tui` is Docker's)
sbx doctor
```

Unrecognized commands forward to Docker's native CLI. `sbx native COMMAND` does
so explicitly. The native executable must differ from this wrapper.

The tool creates one Docker Sandbox per workspace. Project shells and Neovim
run on the host. Project sessions have two windows: Neovim v2 in tab 1 and an
empty shell in tab 2. Exiting Neovim leaves its shell available.
Exiting the agent returns its tab to a host shell, keeping the tmux workspace
available. Run `sbx run --name <workspace-name>` there to start the agent again.
Use `sbx open <workspace-name>` to reconnect after closing the terminal.
Repository mirrors live under `~/.cache/sbx/git`; workspace clones use Git transport and do not hardlink objects to the cache. `sbx new` and `sbx add` verify mirrors before using them and rebuild damaged mirrors from the configured upstream. Replaced mirrors are retained under `.PROJECT-damaged-*/mirror.git` in the cache directory for diagnosis; they can be removed once no longer needed. Cache operations are locked per project; if another command is using the same cache, retry when it finishes.

This repairs future clones only. Existing workspaces with missing objects need a fresh clone; preserve any local changes, commits, and stashes before replacing them.
Node versions are resolved by fnm before configured bootstrap hooks and pnpm install.

## Rex

With `multiplexer: rex` in `config.yaml`, each workspace is a Rex session named
after it, starting with an `agent` tab running `sbx run`. Repositories open in
tabs as you pick them with `Cmd-Shift-f`.
`sbx new`, `open`, `add` and `delete` create, switch to and close those
sessions through the actions in `rex/sbx.lua`. Pickers float over the current
tab; size them with `picker_size` and `form_size` in `setup`. Load them from
`~/.config/rex/init.lua`:

```lua
package.path = os.getenv("HOME") .. "/code/sbx/rex/?.lua;" .. package.path
require("sbx").setup{ project_roots = { "~/code" } }
```

Then run `rex config reload` and turn on Remote Control in the app's settings
(needed to switch sessions from sbx and the pickers).

| Key | Action |
| --- | --- |
| `Cmd-f` | Find a repository under `project_roots`; open it in a tab of this session, reusing its tab |
| `Cmd-Shift-n` | New workspace form |
| `Cmd-Shift-o` | Pick a workspace and switch to its session, recreating it if needed |
| `Cmd-Shift-f` | Pick a repository of this workspace, or add one |
| `Cmd-Shift-d` | Pick a workspace to delete, then type its name to confirm |
| `Cmd-Shift-a` | Show the workspace's agent floating over this tab; again to hide it (it keeps running) |
| `Cmd-Shift-/` | Show these keybindings, as currently configured |
| `Ctrl-h/j/k/l` | Move between panes; passed through to Neovim when it is in front |

Remap any key with `keys = { find_project = "cmd+p" }` in `setup`, or set it to
`false` to leave it unbound.

For Neovim splits and Rex panes to act as one grid, add to the Neovim config:

```lua
if vim.env.REX_SESSION then
  for key, direction in pairs({ h = "left", j = "down", k = "up", l = "right" }) do
    vim.keymap.set("n", "<C-" .. key .. ">", function()
      local win = vim.api.nvim_get_current_win()
      vim.cmd("wincmd " .. key)
      if win == vim.api.nvim_get_current_win() then
        vim.system({ "rex", "do", "sbx_focus", "direction=" .. direction })
      end
    end)
  end
end
```

Workspace sessions are found by label, so renaming one in Rex makes sbx open a
new session for it. Deleting a workspace does not stop host processes started
in its Rex tabs before removing its files, as the tmux mode does.

## tmux

The dedicated server defaults to `sbx-v1`, using your configured tmux prefix.

| Key after prefix | Action |
| --- | --- |
| `i` | New workspace form |
| `o` | All workspace/project sessions |
| `f` | Search current workspace sessions (type to filter) |
| `Meta-k / Meta-j` | Previous/next project in this workspace, wrapping |
| `s` | Open a session beside the current pane |
| `a` | Add a project to the current workspace |
| `Shift-D` | Choose a workspace to delete, then type its name to confirm |
| `n` | Cycle panes |
| `r` | Reload config and reapply sbx bindings |

The project-cycle shortcuts preserve existing bindings on those keys and do not replace
default tmux commands. Meta is usually Alt (Option on macOS when configured as Meta).
They also work as `sbx project --previous` / `sbx project --next`.

Your separate `Shift-n` work-notes popup remains defined by dotfiles.

The picker groups sessions beneath workspace names, expanding the selected workspace.
Use `h/l` (or Left/Right) for previous/next workspace and `j/k` (or Up/Down,
Ctrl-j/k) for sessions within that workspace. Returning to a workspace restores
its selected session. Space folds the current group; moving within it expands it.
The `f` picker starts with fuzzy search focused: type a project name, use Up/Down
to select, and Enter to open. Escape returns to navigation mode.
Press `/` to edit the filter, then Enter or Escape to return to navigation;
Enter opens the selected session. `s` toggles workspace-name and last-used sorting
(default: last used). Recency uses the latest tmux session attachment in each
workspace; unavailable history sorts last and is lost when those sessions are removed.
Tab or Ctrl-h/l controls the initially hidden preview; Ctrl-r refreshes; F1 shows
help; Escape clears a filter or closes the picker. Narrow popups hide the preview.
The full selected workspace/project appears above the list.
Press `D` (or Ctrl-d) on a workspace or any of its sessions to delete that whole
workspace. The picker names the workspace and warns that files, uncommitted
changes, the sandbox, and sessions will be removed; only `y` confirms.
After deletion, `y` opens the new-workspace form; `n`, Enter, or Escape opens the
most recently used remaining workspace. If none remain, it returns to a host
shell. Canceling the deletion itself keeps the selection in the picker.
The separate `sbx delete` command still requires exact-name confirmation.
Sidecars attach to existing sessions; closing a sidecar leaves its target alive.

Only marked direct children of the configured workspace root can be deleted.
Deletion sends SIGTERM to host processes in the workspace’s managed tmux panes,
waits up to two seconds, then sends SIGKILL to survivors before removing the
sandbox and workspace. Processes already detached from those process trees
are not discoverable by this cleanup.
Passive inventory never calls Docker. Creation rollback removes only resources
reserved by that creation attempt.

The delete popup also works from the sandbox agent pane: press your tmux prefix,
then Shift-D. It runs on the host and can delete the workspace you are currently
using; its sessions close after confirmation. Escape cancels the picker.
