-- Lightweight GitHub PR review. Stage inline comments from diffview selections
-- into a pending review, then submit once with a verdict. Replaces octo for the
-- review flow. Works in a jmux PR worktree, where the checked-out branch is the
-- PR's head, so `gh` resolves the PR for every call with no number to pass.

local M = {}

-- pending holds staged comments in GitHub's review-comment shape. One nvim
-- instance maps to one PR worktree, so module-level state is sufficient.
M.pending = {}

-- ctx caches the {slug, number, sha} of the current branch's PR.
local ctx

local function sh(cmd)
  local out = vim.fn.system(cmd)
  if vim.v.shell_error ~= 0 then
    return nil, vim.trim(out)
  end
  return vim.trim(out), nil
end

local function resolve()
  if ctx then
    return ctx
  end
  local slug, e1 = sh { "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner" }
  if not slug then
    return nil, "gh repo view: " .. (e1 or "")
  end
  local num, e2 = sh { "gh", "pr", "view", "--json", "number", "-q", ".number" }
  if not num then
    return nil, "gh pr view: " .. (e2 or "")
  end
  local sha = sh { "git", "rev-parse", "HEAD" }
  ctx = { slug = slug, number = tonumber(num), sha = sha }
  return ctx
end

-- target resolves the {path, side, start_line, line} for the current diffview
-- selection, or nil + reason when the cursor isn't in a reviewable diff window.
-- Diffview's right (new) buffer shows the head file, so its line numbers map
-- straight to GitHub's RIGHT side; the left buffer maps to LEFT/oldpath.
local function target()
  local ok, lib = pcall(require, "diffview.lib")
  if not ok then
    return nil, "diffview not loaded"
  end
  local view = lib.get_current_view()
  if not view or not view.cur_entry or not view.cur_layout then
    return nil, "not in a diffview diff"
  end

  local entry = view.cur_entry
  local layout = view.cur_layout
  local curwin = vim.api.nvim_get_current_win()

  local side, path
  local main = layout:get_main_win()
  if main and curwin == main.id then
    side, path = "RIGHT", entry.path
  elseif layout.a and curwin == layout.a.id then
    side, path = "LEFT", entry.oldpath
  else
    return nil, "cursor is not in a diff window"
  end
  if not path or path == "null" then
    return nil, "no file under cursor"
  end

  local sline, eline
  local mode = vim.fn.mode()
  if mode == "v" or mode == "V" or mode == "\22" then
    sline = vim.fn.getpos("v")[2]
    eline = vim.fn.getpos(".")[2]
    if sline > eline then
      sline, eline = eline, sline
    end
    vim.api.nvim_feedkeys(vim.api.nvim_replace_termcodes("<Esc>", true, false, true), "nx", false)
  else
    sline = vim.fn.line "."
    eline = sline
  end

  return { path = path, side = side, start_line = sline, line = eline }
end

-- add_comment stages an inline comment on the selected line(s). Bind in normal
-- and visual mode; a visual selection becomes a multi-line comment.
function M.add_comment()
  local t, reason = target()
  if not t then
    vim.notify(reason, vim.log.levels.WARN)
    return
  end
  vim.ui.input({ prompt = "comment> " }, function(body)
    if not body or body == "" then
      return
    end
    local c = { path = t.path, side = t.side, line = t.line, body = body }
    if t.start_line ~= t.line then
      c.start_line = t.start_line
      c.start_side = t.side
    end
    table.insert(M.pending, c)
    local span = t.start_line == t.line and tostring(t.line) or string.format("%d-%d", t.start_line, t.line)
    vim.notify(string.format("staged %s:%s (%d pending)", t.path, span, #M.pending))
  end)
end

-- submit posts the pending comments as one review with a chosen verdict.
function M.submit()
  local c, err = resolve()
  if not c then
    vim.notify(err, vim.log.levels.ERROR)
    return
  end
  vim.ui.select({ "COMMENT", "APPROVE", "REQUEST_CHANGES" }, { prompt = "submit review as:" }, function(event)
    if not event then
      return
    end
    vim.ui.input({ prompt = "summary (optional)> " }, function(body)
      local payload = vim.json.encode {
        commit_id = c.sha,
        event = event,
        body = (body and body ~= "") and body or nil,
        comments = (#M.pending > 0) and M.pending or nil,
      }
      local out = vim.fn.system({
        "gh",
        "api",
        "--method",
        "POST",
        string.format("repos/%s/pulls/%d/reviews", c.slug, c.number),
        "--input",
        "-",
      }, payload)
      if vim.v.shell_error ~= 0 then
        vim.notify("submit failed:\n" .. vim.trim(out), vim.log.levels.ERROR)
        return
      end
      local n = #M.pending
      M.pending = {}
      vim.notify(string.format("submitted %s with %d comment(s)", event, n))
    end)
  end)
end

local function scratch(name, lines)
  vim.cmd "botright vnew"
  local buf = vim.api.nvim_get_current_buf()
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].buftype = "nofile"
  vim.bo[buf].bufhidden = "wipe"
  vim.bo[buf].filetype = "markdown"
  vim.bo[buf].modifiable = false
  pcall(vim.api.nvim_buf_set_name, buf, name)
end

-- list shows the staged comments in a scratch buffer.
function M.list()
  if #M.pending == 0 then
    vim.notify "no pending comments"
    return
  end
  local lines = {}
  for i, c in ipairs(M.pending) do
    local span = c.start_line and string.format("%d-%d", c.start_line, c.line) or tostring(c.line)
    table.insert(lines, string.format("%d. %s:%s [%s]", i, c.path, span, c.side))
    for _, l in ipairs(vim.split(c.body, "\n")) do
      table.insert(lines, "    " .. l)
    end
  end
  scratch("pending-review", lines)
end

-- discard drops all staged comments.
function M.discard()
  M.pending = {}
  vim.notify "discarded pending comments"
end

-- conversation opens `gh pr view --comments` (description + threads) read-only.
function M.conversation()
  local out = vim.fn.system { "gh", "pr", "view", "--comments" }
  scratch("pr-conversation", vim.split(out, "\n"))
end

-- browser opens the current branch's PR on github.com.
function M.browser()
  local out = vim.fn.system { "gh", "pr", "view", "--web" }
  if vim.v.shell_error ~= 0 then
    vim.notify("gh pr view --web: " .. vim.trim(out), vim.log.levels.ERROR)
  end
end

return M
