-- Per-worktree persistence for the pr modules, kept in the worktree's git dir
-- so it's hidden from `git status` and removed with the worktree.

local util = require "jmux.pr.util"

local M = {}

-- pending holds staged comments in GitHub's review-comment shape.
M.pending = {}

M.PR_STORE = "jmux-review.json"
M.AGENT_STORE = "jmux-agent-review.json"
local store = M.PR_STORE

local function state_file()
  local dir = util.git_dir()
  return dir and (dir .. "/" .. store) or nil
end

-- save writes M.pending to disk; an empty review removes the file.
function M.save()
  local f = state_file()
  if not f then
    return
  end
  if #M.pending == 0 then
    vim.fn.delete(f)
    return
  end
  vim.fn.writefile({ vim.json.encode { sha = util.sh { "git", "rev-parse", "HEAD" }, comments = M.pending } }, f)
end

local loaded = false

-- use switches the active pending store.
function M.use(file)
  if store == file then
    return
  end
  store = file
  M.pending = {}
  loaded = false
end

-- load restores the persisted review once per session, warning when HEAD has
-- moved since it was staged (line anchors may have shifted).
function M.load()
  if loaded then
    return
  end
  loaded = true
  local f = state_file()
  if not f or vim.fn.filereadable(f) == 0 then
    return
  end
  local ok, data = pcall(vim.json.decode, table.concat(vim.fn.readfile(f), "\n"))
  if not ok or type(data) ~= "table" or type(data.comments) ~= "table" or #data.comments == 0 then
    return
  end
  M.pending = data.comments
  local head = util.sh { "git", "rev-parse", "HEAD" }
  if data.sha and head and data.sha ~= head then
    vim.notify(
      string.format(
        "restored %d pending comment(s) staged on %s; HEAD is now %s — line numbers may have shifted",
        #M.pending,
        data.sha:sub(1, 7),
        head:sub(1, 7)
      ),
      vim.log.levels.WARN
    )
  end
end

-- discard drops all staged comments; marking loaded stops a later load from
-- resurrecting them.
function M.discard()
  loaded = true
  M.pending = {}
  M.save()
end

local function view_cache_file()
  local dir = util.git_dir()
  return dir and (dir .. "/jmux-pr-view.json") or nil
end

-- write_view_cache stores the raw conversation response for instant reopen.
function M.write_view_cache(json)
  local f = view_cache_file()
  if f then
    vim.fn.writefile({ json }, f)
  end
end

-- read_view_cache returns the decoded cached response and its age in seconds,
-- or nil.
function M.read_view_cache()
  local f = view_cache_file()
  if not f or vim.fn.filereadable(f) == 0 then
    return nil
  end
  local ok, data = pcall(vim.json.decode, table.concat(vim.fn.readfile(f), "\n"))
  if not ok then
    return nil
  end
  return data, os.time() - vim.fn.getftime(f)
end

-- clear_view_cache forces the next view open to refetch.
function M.clear_view_cache()
  local f = view_cache_file()
  if f then
    vim.fn.delete(f)
  end
end

return M
