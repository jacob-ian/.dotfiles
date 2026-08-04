-- The pre-PR agent review: stage comments on the working tree's uncommitted
-- changes, then hand the batch to the connected claude as a markdown brief.

local state = require "jmux.pr.state"
local util = require "jmux.pr.util"

local M = {}

local HEADER = [[# Code review — working tree

Resolve each comment below. They refer to the current working tree; within a
file, work from the bottom comment upward so your edits don't shift the line
numbers of the ones above. A ```suggestion block is the proposed replacement
for the quoted lines. The quoted code locates each site if lines have moved.]]

-- excerpt returns the commented lines: the working tree for RIGHT, HEAD for LEFT.
local function excerpt(cm)
  local first = cm.start_line or cm.line
  if cm.side == "LEFT" then
    local out = util.sh { "git", "show", "HEAD:" .. cm.path }
    if not out then
      return nil
    end
    local lines = vim.split(out, "\n")
    return vim.list_slice(lines, first, cm.line)
  end
  -- comment paths are repo-relative; nvim's cwd may be a subdirectory.
  local root = util.sh { "git", "rev-parse", "--show-toplevel" }
  local path = root and (root .. "/" .. cm.path) or cm.path
  if vim.fn.filereadable(path) == 0 then
    return nil
  end
  return vim.list_slice(vim.fn.readfile(path), first, cm.line)
end

-- format renders the pending comments as the markdown brief, as single lines
-- (writefile treats embedded newlines as NULs).
local function format(pending)
  local out = { HEADER, "" }
  for _, cm in ipairs(pending) do
    local loc = ("%s:%s"):format(cm.path, util.span_str(cm))
    if cm.side == "LEFT" then
      loc = loc .. " (removed code — line numbers refer to HEAD)"
    end
    out[#out + 1] = "## " .. loc
    local code = excerpt(cm)
    if code then
      local lang = cm.path:match "%.(%w+)$" or ""
      out[#out + 1] = "```" .. lang
      vim.list_extend(out, code)
      out[#out + 1] = "```"
    end
    out[#out + 1] = cm.body
    out[#out + 1] = ""
  end
  return vim.split(table.concat(out, "\n"), "\n")
end

-- review opens the working-tree diff with comment staging.
function M.review()
  require("jmux.pr.diff").review_worktree()
end

-- claude_pane resolves the tmux pane of the claude connected to this nvim —
-- via its socket, not a window-name guess. nil unless exactly one lsof client
-- on the server port resolves to a pane.
local function claude_pane()
  local status = require("claudecode.server.init").get_status()
  if not status.port then
    return nil
  end
  local out = util.sh { "lsof", "-nP", "-iTCP:" .. status.port, "-sTCP:ESTABLISHED", "-Fp" }
  if not out then
    return nil
  end
  local me, clients = vim.fn.getpid(), {}
  for pid in out:gmatch "p(%d+)" do
    if tonumber(pid) ~= me then
      clients[tonumber(pid)] = true
    end
  end
  local client = next(clients)
  if not client or next(clients, client) then
    return nil
  end
  local ps = util.sh { "ps", "-axo", "pid=,ppid=" } or ""
  local parent = {}
  for pid, ppid in ps:gmatch "(%d+)%s+(%d+)" do
    parent[tonumber(pid)] = tonumber(ppid)
  end
  local panes = {}
  local tm = util.sh { "tmux", "list-panes", "-a", "-F", "#{pane_id} #{pane_pid}" } or ""
  for id, pid in tm:gmatch "(%%%d+) (%d+)" do
    panes[tonumber(pid)] = id
  end
  for _ = 1, 32 do
    if not client or client <= 1 then
      return nil
    end
    if panes[client] then
      return panes[client]
    end
    client = parent[client]
  end
end

-- submit types the run instruction + Enter into the connected claude's pane
-- (the claudecode protocol can attach mentions but never submit), deferred
-- past the mention's delivery debounce. Any failure degrades to pressing
-- Enter yourself.
local function submit(file, n)
  local sent_msg = ("sent %d comment(s) to claude"):format(n)
  local manual = sent_msg .. " — press enter in the claude pane to run"
  if not vim.env.TMUX then
    return vim.notify(manual)
  end
  vim.defer_fn(function()
    local pane = claude_pane()
    if not pane then
      return vim.notify(manual)
    end
    -- the trailing space dismisses the file picker that typing @ opens, so
    -- Enter submits instead of selecting from the popup.
    local instruction = ("Resolve each review comment in @%s "):format(file)
    vim.system({ "tmux", "send-keys", "-t", pane, "-l", instruction }, { text = true }, function(res)
      if res.code ~= 0 then
        return vim.schedule(function()
          vim.notify(manual)
        end)
      end
      vim.system({ "tmux", "send-keys", "-t", pane, "Enter" }, {}, function()
        vim.schedule(function()
          vim.notify(sent_msg .. " — running")
        end)
      end)
    end)
  end, 800)
end

-- send writes the brief to the git dir and @-mentions it to the connected
-- claude; the staged review only clears on an accepted send, so failures retry.
function M.send()
  state.use(state.AGENT_STORE)
  state.load()
  if #state.pending == 0 then
    vim.notify("no staged comments — review the working tree first", vim.log.levels.WARN)
    return
  end
  local dir = util.git_dir()
  if not dir then
    return vim.notify("not in a git worktree", vim.log.levels.ERROR)
  end
  -- one file per review: claude may still be reading the previous brief.
  local file = ("%s/jmux-agent-review-%s.md"):format(dir, os.date "%Y%m%d-%H%M%S")
  for i = 2, 99 do
    if vim.fn.filereadable(file) == 0 then
      break
    end
    file = ("%s/jmux-agent-review-%s-%d.md"):format(dir, os.date "%Y%m%d-%H%M%S", i)
  end
  vim.fn.writefile(format(state.pending), file)

  local ok, claudecode = pcall(require, "claudecode")
  if not ok then
    return vim.notify("claudecode.nvim not available — review written to " .. file, vim.log.levels.ERROR)
  end
  -- send_at_mention "succeeds" by queueing when nothing is connected, which
  -- may never deliver — require a live connection.
  if not claudecode.is_claude_connected() then
    return vim.notify("no claude connected — staged review kept; review file at " .. file, vim.log.levels.WARN)
  end
  local sent, err = claudecode.send_at_mention(file, nil, nil, "jmux agent review")
  if not sent then
    return vim.notify(
      ("send to claude failed: %s\nstaged review kept; review file at %s"):format(err or "?", file),
      vim.log.levels.ERROR
    )
  end
  local n = #state.pending
  state.discard()
  submit(file, n)
end

return M
