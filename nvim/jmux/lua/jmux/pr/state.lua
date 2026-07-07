-- Per-worktree persistence for the pr modules. Everything lives in the
-- worktree's own git dir (.git/worktrees/<name>/ for a linked worktree): that
-- scopes each file to the one PR this worktree tracks, hides it from
-- `git status`, and lets `git worktree remove` clean it up with everything else.

local util = require "jmux.pr.util"

local M = {}

-- pending holds staged comments in GitHub's review-comment shape. One nvim
-- instance maps to one PR worktree, so module-level state is sufficient.
M.pending = {}

local function state_file()
  local dir = util.git_dir()
  return dir and (dir .. "/jmux-review.json") or nil
end

-- save writes M.pending through on every mutation (crash-safe; the payload is
-- tiny). An empty review removes the file rather than leaving an empty stub.
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

-- load restores a persisted review the first time the module is touched in a
-- session. The stored HEAD sha is compared against the current one: comments are
-- anchored to line numbers on a specific commit, so a rebase/amend since the
-- last session may have shifted them, which is worth a warning.
local loaded = false
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
  -- Only speak up when it matters: comments are anchored to line numbers on a
  -- specific commit, so a rebase/amend since they were staged may have shifted
  -- them. A clean restore is silent — the count shows on the next stage/submit.
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

-- discard drops all staged comments, in memory and on disk. The loaded flag is
-- forced so a later stage in the same session can't resurrect the cleared review.
function M.discard()
  loaded = true
  M.pending = {}
  M.save()
end

-- The PR conversation (pv) is cached here too, so reopening the view paints
-- instantly from disk instead of waiting on the GraphQL round trip; <C-r> in the
-- view refetches and rewrites it.
local function view_cache_file()
  local dir = util.git_dir()
  return dir and (dir .. "/jmux-pr-view.json") or nil
end

-- write_view_cache stores the VIEW_QUERY response as JSON, so reading it back
-- decodes to exactly what a live fetch would and the render path is identical.
function M.write_view_cache(json)
  local f = view_cache_file()
  if f then
    vim.fn.writefile({ json }, f)
  end
end

-- read_view_cache returns the decoded cached response and its age in seconds, or
-- nil when there's no readable, decodable cache. Decoding here (rather than
-- handing the render path raw JSON to re-parse) avoids a second full decode of
-- the conversation payload on the main thread. An undecodable cache returns nil
-- so the open falls through to a live fetch. Age comes from the file mtime and
-- is shown in the view's bar so a stale cache is obvious (and a reminder that
-- <C-r> refreshes it).
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

-- clear_view_cache drops the cached conversation so the next pv refetches — used
-- after posting a reply or submitting a review, which the stale cache wouldn't
-- yet reflect.
function M.clear_view_cache()
  local f = view_cache_file()
  if f then
    vim.fn.delete(f)
  end
end

return M
