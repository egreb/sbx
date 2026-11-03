-- sbx workspaces and project navigation for Rex.
--
-- In ~/.config/rex/init.lua:
--
--   package.path = os.getenv("HOME") .. "/code/sbx/rex/?.lua;" .. package.path
--   require("sbx").setup{ project_roots = { "~/code" } }
--
-- A workspace is one Rex session labelled with the workspace name. Its first
-- tab runs the agent; repositories open in tabs as they are picked. Pickers
-- float over the tab and close when the pick is made. Switching the app to a
-- session or tab is left to sbx, which waits for the app to see it first.

local M = {}

local home = os.getenv("HOME")
local terminal = "com.superlogical.terminal"
local shell_flavor = "com.superlogical.terminal.shell"

local opts = {
  project_roots = { "~/code" },
  project_depth = 3,
  workspace_root = "~/sandboxes",
  sbx = "sbx",
  fzf = "fzf",
  host_shell = os.getenv("SHELL") or "/bin/zsh",
  keys = {
    find_project = "cmd+f",
    new_workspace = "cmd+shift+n",
    open_workspace = "cmd+shift+o",
    workspace_project = "cmd+shift+f",
    delete_workspace = "cmd+shift+d",
    toggle_agent = "cmd+shift+a",
    keybindings = "cmd+shift+/",
  },
  navigate = true,
  -- Floating picker sizes, as fractions of the tab: { width, height }.
  picker_size = { 0.6, 0.6 },
  form_size = { 0.8, 0.8 },
  agent_size = { 0.85, 0.85 },
}

local function expand(path)
  return (path:gsub("^~/", home .. "/"))
end

local function call(method, args)
  local result, err = rex.call(method, args or {})
  if err ~= nil then
    error(err, 0)
  end
  return result
end

local function find_session(label)
  for _, session in ipairs(call("session.list").sessions or {}) do
    if session.label == label then
      return session.session_id
    end
  end
end

local function session_label(session_id)
  for _, session in ipairs(call("session.list").sessions or {}) do
    if session.session_id == session_id then
      return session.label
    end
  end
end

local function project_layout(dir)
  return rex.layout.block{ flavor = shell_flavor, options = { cwd = dir } }
end

local function project_label(dir)
  return dir:match("([^/]+)/?$")
end

-- Runs argv, and on failure keeps its output on screen until Enter.
local function hold_on_failure(argv)
  return { "/bin/sh", "-c", '"$@" || { printf "\\n[exit %s] Press Enter to close" $?; read _; }', "sh", unpack(argv) }
end

-- Runs argv in a floating layer centred over the current tab, closed on
-- exit. size is a fraction of the tab, such as { 0.6, 0.6 }.
local function run_floating(label, argv, size)
  local w, h = size[1], size[2]
  local x, y = (1 - w) / 2, (1 - h) / 2
  local _, err = rex.session.new_layer{
    bounds = { x, y, w, h },
    focus = true,
    layout = rex.layout.block{
      flavor = shell_flavor,
      label = label,
      options = {
        command = hold_on_failure(argv),
        shell = "none",
        exit = { on_completion = true, quick_exit_threshold_ms = 0 },
      },
    },
  }
  if err ~= nil then
    error(err, 0)
  end
end

-- Rex starts commands without the PATH your shell's startup files build, so
-- ask the login shell for it once.
local login_path

local function host_path()
  if login_path == nil then
    local r = rex.exec({ opts.host_shell, "-l", "-c", "printenv PATH" }, { timeout = 10 })
    login_path = r and r.ok and r.stdout:gsub("%s+$", "") or ""
  end
  return login_path
end

-- Runs script with the login PATH after Rex's own, which holds rex. The PATH
-- is passed as $0 so it needs no quoting; arguments start at $1 as usual.
local function sh(script, ...)
  return { "/bin/sh", "-c", 'PATH="$PATH:$0"; export PATH\n' .. script, host_path(), ... }
end

-- Shows its arguments, one per line, until a key is pressed.
local key_sheet = [[
printf '%s\n' "$@"
printf '\nPress any key to close'
old=$(stty -g); stty -icanon -echo; dd bs=1 count=1 >/dev/null 2>&1; stty "$old"
]]

-- Pickers. Each hands its choice to sbx, which opens it and switches to it.

local find_picker = [[
sbx=$1; fzf=$2; depth=$3; shift 3
dir=$(find "$@" -mindepth 1 -maxdepth "$depth" -name .git -prune 2>/dev/null \
  | sed 's|/\.git$||' | sort -u \
  | "$fzf" --layout=reverse --prompt='project > ' --scheme=path) || exit 0
exec "$sbx" __rex-open-project "$dir"
]]

local workspace_project_picker = [[
sbx=$1; fzf=$2; workspace=$3; name=$4
choice=$( { find "$workspace" -mindepth 2 -maxdepth 2 -name .git | sed 's|/\.git$||; s|.*/||' | sort
            echo '+ add project'; } \
  | "$fzf" --layout=reverse --prompt="$name > ") || exit 0
if [ "$choice" = '+ add project' ]; then
  exec "$sbx" add --workspace "$name"
fi
exec "$sbx" __rex-open-project "$workspace/$choice"
]]

local function open_workspace(ctx, args)
  local name = args.name
  local session_id = find_session(name)
  if not session_id then
    local workspace = args.path or (expand(opts.workspace_root) .. "/" .. name)
    -- The agent never shows a host prompt, so report the workspace as the
    -- tab's directory (OSC 7) for splits and new tabs to start in.
    local agent = sh(
      [[printf '\033]7;file://%s%s\007' "$(hostname)" "$5"
"$1" run --name "$2" ${3:+-- "$3"}; exec "$4" -l]],
      opts.sbx, name, args.prompt or "", opts.host_shell, workspace)
    local windows = {
      {
        window_label = "agent",
        layout = rex.layout.block{
          flavor = shell_flavor,
          label = "agent",
          options = { cwd = workspace, command = agent, shell = "none" },
        },
      },
    }
    session_id = call("session.create", { label = name, initial_windows = windows }).session_id
    -- A new session opens on its agent tab; an existing one keeps its own.
    local view = call("session.view", { session_id = session_id })
    for _, window in ipairs(view.windows or {}) do
      if window.label == "agent" then
        return { session_id = session_id, window_id = window.window_id }
      end
    end
  end
  return { session_id = session_id }
end

-- Opens a repository in a tab of the current session, reusing its tab.
local function open_project(ctx, args)
  local dir = expand(args.cwd):gsub("/$", "")
  local label = project_label(dir)
  local view = rex.session.view{}
  for _, window in ipairs(view and view.windows or {}) do
    if window.label == label then
      return { window_id = window.window_id }
    end
  end
  local w, err = rex.session.new_window{ layout = project_layout(dir) }
  if err ~= nil then
    error(err, 0)
  end
  rex.session.set_window_label{ window_id = w.window_id, label = label }
  return { window_id = w.window_id }
end

-- The workspace's agent: the block labelled agent. Sessions made before
-- blocks were labelled are searched for the block running sbx (sbx run), or
-- failing that the agent tab; the block found is labelled for next time.
local function agent_block(session_id)
  local blocks = call("session.list_blocks", { session_id = session_id }).blocks or {}
  for _, block in ipairs(blocks) do
    if block.label == "agent" then
      return block.block_id
    end
  end
  local found
  for _, block in ipairs(blocks) do
    local p = rex.block.call(terminal, "process", { block_id = block.block_id })
    local fg = p and p.foreground
    if fg and (fg.name == "sbx" or (fg.argv0 or ""):match("/?sbx$")) then
      found = block.block_id
      break
    end
  end
  if not found then
    for _, window in ipairs(call("session.view", { session_id = session_id }).windows or {}) do
      if window.label == "agent" then
        local tiled = window.layers and window.layers[1] and window.layers[1].blocks or {}
        found = tiled[1] and tiled[1].block_id
        break
      end
    end
  end
  if found then
    rex.call("session.set_block_label", { session_id = session_id, block_id = found, label = "agent" })
  end
  return found
end

local function floating_in_active_window(view, block_id)
  for _, window in ipairs(view.windows or {}) do
    if window.window_id == view.active_window_id then
      for _, layer in ipairs(window.layers or {}) do
        if layer.kind ~= "tiled" then
          for _, block in ipairs(layer.blocks or {}) do
            if block.block_id == block_id then
              return true
            end
          end
        end
      end
    end
  end
  return false
end

-- Shows the agent floating over the current tab, or hides it again. Hidden,
-- it is detached: still running, shown nowhere, until shown again. The first
-- time it floats, its own tab closes, as the tab is left empty.
local function toggle_agent(ctx)
  local agent = agent_block(ctx.session_id)
  if not agent then
    error("This session has no sbx agent", 0)
  end
  local view = rex.session.view{}
  if floating_in_active_window(view, agent) then
    call("session.detach_block", { session_id = ctx.session_id, block_id = agent })
    return
  end
  if agent ~= ctx.block_id then
    -- Placed beside the focused block first: float_block only takes a block
    -- from the layout of the window it should float in.
    call("session.move_block", { session_id = ctx.session_id, block_id = agent, anchor_block_id = ctx.block_id })
  end
  local w, h = opts.agent_size[1], opts.agent_size[2]
  call("session.float_block", {
    session_id = ctx.session_id,
    block_id = agent,
    bounds = { (1 - w) / 2, (1 - h) / 2, w, h },
  })
  call("session.focus_block", { session_id = ctx.session_id, block_id = agent })
end

local function close_workspace(ctx, args)
  local session_id = find_session(args.name)
  if not session_id then
    return
  end
  local view = call("session.view", { session_id = session_id })
  for _, window in ipairs(view.windows or {}) do
    rex.call("session.close_window", { session_id = session_id, window_id = window.window_id })
  end
end

-- ctrl+hjkl moves between Rex panes, unless the pane runs Neovim; Neovim then
-- moves between its own splits and calls sbx_focus at its edge.
local editors = { vim = true, nvim = true }

local function in_editor(ctx)
  local p = rex.block.call(terminal, "process", { block_id = ctx.block_id })
  local fg = p and p.foreground
  return fg ~= nil and (editors[fg.name] or editors[fg.argv0]) == true
end

local function bind_navigation()
  for key, direction in pairs{ ["ctrl+h"] = "left", ["ctrl+j"] = "down", ["ctrl+k"] = "up", ["ctrl+l"] = "right" } do
    rex.bind(key, function(ctx)
      if in_editor(ctx) then
        return false
      end
      rex.session.focus_direction{ direction = direction }
      return true
    end)
  end
  rex.action{
    name = "sbx_focus",
    title = "Focus Neighbouring Pane",
    args = { direction = "string" },
    run = function(ctx, args)
      rex.session.focus_direction{ direction = args.direction }
    end,
  }
end

function M.setup(user)
  for k, v in pairs(user or {}) do
    if k == "keys" then
      for action, key in pairs(v) do
        opts.keys[action] = key
      end
    else
      opts[k] = v
    end
  end
  local roots = {}
  for i, root in ipairs(opts.project_roots) do
    roots[i] = expand(root)
  end

  rex.action{
    name = "sbx_find_project",
    title = "Find Project…",
    category = "sbx",
    keywords = "project repository open",
    run = function()
      run_floating("find project", sh(find_picker, opts.sbx, opts.fzf, tostring(opts.project_depth), unpack(roots)), opts.picker_size)
    end,
  }
  rex.action{
    name = "sbx_new_workspace",
    title = "New Workspace…",
    category = "sbx",
    run = function()
      run_floating("new workspace", sh('exec "$@"', opts.sbx, "new"), opts.form_size)
    end,
  }
  rex.action{
    name = "sbx_pick_workspace",
    title = "Open Workspace…",
    category = "sbx",
    run = function()
      run_floating("open workspace", sh('exec "$@"', opts.sbx, "open"), opts.picker_size)
    end,
  }
  rex.action{
    name = "sbx_workspace_project",
    title = "Open Project in Workspace…",
    category = "sbx",
    run = function(ctx)
      local name = session_label(ctx.session_id)
      local workspace = expand(opts.workspace_root) .. "/" .. (name or "")
      local marker = name and io.open(workspace .. "/.sbx-managed")
      if not marker then
        error("This session is not an sbx workspace", 0)
      end
      marker:close()
      run_floating("open project", sh(workspace_project_picker, opts.sbx, opts.fzf, workspace, name), opts.picker_size)
    end,
  }
  rex.action{
    name = "sbx_delete_workspace",
    title = "Delete Workspace…",
    category = "sbx",
    run = function()
      run_floating("delete workspace", sh('exec "$@"', opts.sbx, "delete"), opts.picker_size)
    end,
  }

  rex.action{
    name = "sbx_toggle_agent",
    title = "Toggle Agent",
    category = "sbx",
    keywords = "agent claude float popup",
    run = toggle_agent,
  }

  -- Called by sbx and the pickers.
  rex.action{
    name = "sbx_open_workspace",
    title = "Open Workspace by Name",
    category = "sbx",
    args = { name = "string", path = "string?", prompt = "string?" },
    run = open_workspace,
  }
  rex.action{
    name = "sbx_open_project",
    title = "Open Project by Path",
    category = "sbx",
    args = { cwd = "string" },
    run = open_project,
  }
  rex.action{
    name = "sbx_close_workspace",
    title = "Close Workspace Session",
    category = "sbx",
    args = { name = "string" },
    run = close_workspace,
  }

  -- In the order the key sheet lists them.
  local bindings = {
    { "sbx_find_project", opts.keys.find_project, "Find a repository and open it in a tab" },
    { "sbx_new_workspace", opts.keys.new_workspace, "New workspace" },
    { "sbx_pick_workspace", opts.keys.open_workspace, "Switch to a workspace" },
    { "sbx_workspace_project", opts.keys.workspace_project, "Open or add a repository in this workspace" },
    { "sbx_delete_workspace", opts.keys.delete_workspace, "Delete a workspace" },
    { "sbx_toggle_agent", opts.keys.toggle_agent, "Show or hide this workspace's agent, floating" },
    { "sbx_keybindings", opts.keys.keybindings, "Show these keybindings" },
  }
  local sheet = { "sbx keybindings", "" }
  for _, b in ipairs(bindings) do
    if b[2] then
      rex.bind(b[2], b[1])
      sheet[#sheet + 1] = string.format("  %-16s %s", b[2], b[3])
    end
  end
  if opts.navigate then
    bind_navigation()
    sheet[#sheet + 1] = string.format("  %-16s %s", "ctrl+h/j/k/l", "Move between panes (and Neovim splits)")
  end

  rex.action{
    name = "sbx_keybindings",
    title = "Keybindings",
    category = "sbx",
    keywords = "keys shortcuts help cheat sheet",
    run = function()
      run_floating("sbx keys", sh(key_sheet, unpack(sheet)), opts.picker_size)
    end,
  }
end

return M
