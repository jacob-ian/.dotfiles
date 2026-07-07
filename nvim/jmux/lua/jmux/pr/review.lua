-- The pending review: a two-column preview of staged comments (code left,
-- comment right), verdict selection, and the single GraphQL submit.

local gh = require "jmux.pr.gh"
local state = require "jmux.pr.state"
local ui = require "jmux.pr.ui"
local util = require "jmux.pr.util"

local M = {}

-- CONTEXT is how many lines of code to show either side of a comment's span in
-- the submit preview.
local CONTEXT = 2

-- ADD_REVIEW_MUTATION submits the whole review in one call: the verdict (event),
-- optional summary, and every staged comment as a draft thread. One mutation
-- keeps the submit atomic — there's no intermediate pending review to orphan on
-- a partial failure.
local ADD_REVIEW_MUTATION = [[
mutation($input:AddPullRequestReviewInput!){
  addPullRequestReview(input:$input){ pullRequestReview{ id } }
}]]

-- post_review submits the pending comments as one review with the chosen verdict
-- and optional summary body, then calls on_done(true) on success or on_done(false)
-- after notifying the failure. Async, and the payload is serialized up front, so
-- a later edit to state.pending can't change what's already in flight.
local function post_review(c, event, body, on_done)
  if not c.id then
    vim.notify("submit failed: PR node id not resolved (gh too old?)", vim.log.levels.ERROR)
    on_done(false)
    return
  end
  local threads = {}
  for _, cm in ipairs(state.pending) do
    threads[#threads + 1] = {
      path = cm.path,
      line = cm.line,
      side = cm.side,
      startLine = cm.start_line,
      startSide = cm.start_side,
      body = cm.body,
    }
  end
  gh.graphql(ADD_REVIEW_MUTATION, {
    input = {
      pullRequestId = c.id,
      commitOID = c.sha,
      event = event,
      body = (body and body ~= "") and body or nil,
      threads = (#threads > 0) and threads or nil,
    },
  }, function(data, err)
    if not data then
      vim.notify("submit failed:\n" .. (err or ""), vim.log.levels.ERROR)
      on_done(false)
      return
    end
    on_done(true)
  end)
end

-- build_preview reads each pending comment's surrounding code and returns a list
-- of blocks: { left = {header, rows={num,text,selected} | unavailable}, lang,
-- right = {header, body} }. rows carry ±CONTEXT lines around the comment span,
-- read from the head SHA for RIGHT comments and the base SHA for LEFT.
local function build_preview(c)
  -- cache file content per rev:path so multiple comments on one file read once.
  local cache = {}
  local function rev_lines(rev, path)
    if not rev then
      return nil, "no base ref"
    end
    local key = rev .. ":" .. path
    local hit = cache[key]
    if hit then
      return hit.lines, hit.err
    end
    -- systemlist (not sh) so leading indentation on the first/last line survives.
    local lines = vim.fn.systemlist { "git", "show", key }
    if vim.v.shell_error ~= 0 then
      cache[key] = { err = vim.trim(table.concat(lines, " ")) }
      return nil, cache[key].err
    end
    cache[key] = { lines = lines }
    return lines
  end

  if #state.pending == 0 then
    return {
      {
        left = { header = "▌ no inline comments staged", rows = {} },
        -- mirror the header on the right; there's nothing to pair below it.
        right = { header = "▌ no inline comments staged", body = {} },
      },
    }
  end

  local blocks = {}
  for _, cm in ipairs(state.pending) do
    local rev = cm.side == "LEFT" and c.base or c.sha
    local lines, err = rev_lines(rev, cm.path)
    local first, last = cm.start_line or cm.line, cm.line
    local block = {
      lang = ui.lang_of(cm.path),
      left = {
        header = string.format("▌ %s:%s  [%s]", cm.path, util.span_str(cm), cm.side),
        path = cm.path,
        side = cm.side,
      },
      right = { header = "▌ comment", body = vim.split(cm.body, "\n") },
    }
    if lines then
      local lo, hi = math.max(1, first - CONTEXT), math.min(#lines, last + CONTEXT)
      local rows = {}
      for i = lo, hi do
        rows[#rows + 1] = { num = i, text = lines[i], selected = i >= first and i <= last }
      end
      block.left.rows = rows
    else
      block.left.unavailable = err or "?"
    end
    blocks[#blocks + 1] = block
  end
  return blocks
end

-- render flattens the blocks into the two column buffers and decorates the left
-- one with extmarks: an inline line-number/▸ gutter, full syntax highlighting on
-- the commented lines, and a muted overlay on the surrounding context. Keeping
-- the buffer text as pure code (gutter is virtual) is what lets the treesitter
-- column offsets line up. Sections are padded so both columns stay aligned under
-- scrollbind.
-- render returns locs (left-buffer row → code {path, side, line}, for <CR>) and
-- owner (row → index of the owning comment in state.pending, shared by both
-- columns since they're padded to the same per-block row range, for d).
-- Re-runnable: it clears its own namespace first so it can redraw after a
-- comment is removed.
local function render(blocks, lbuf, rbuf)
  local ns = vim.api.nvim_create_namespace "jmux_pr_preview"
  vim.api.nvim_buf_clear_namespace(lbuf, ns, 0, -1)
  local left, right, meta, owner = {}, {}, {}, {}

  for bi, b in ipairs(blocks) do
    local lsec, lmeta = { b.left.header }, { { header = true } }
    local lbase = #left -- buffer row (0-indexed) the header will land on
    if b.left.unavailable then
      lsec[2] = "  ‹code unavailable: " .. b.left.unavailable .. "›"
      lmeta[2] = { muted = true }
    else
      for _, r in ipairs(b.left.rows) do
        lsec[#lsec + 1] = r.text
        lmeta[#lmeta + 1] = { code = true, num = r.num, selected = r.selected, path = b.left.path, side = b.left.side }
      end
    end

    local rsec = { b.right.header }
    for _, line in ipairs(b.right.body) do
      rsec[#rsec + 1] = "  " .. line
    end

    -- pad the shorter column, then a blank separator row to each.
    local h = math.max(#lsec, #rsec)
    while #lsec < h do
      lsec[#lsec + 1], lmeta[#lmeta + 1] = "", false
    end
    while #rsec < h do
      rsec[#rsec + 1] = ""
    end
    lsec[#lsec + 1], lmeta[#lmeta + 1] = "", false
    rsec[#rsec + 1] = ""

    for i = 1, #lsec do
      left[#left + 1], meta[#left + 1] = lsec[i], lmeta[i]
    end
    vim.list_extend(right, rsec)
    b.lbase = lbase
    -- every row of this block (header, code, padding, separator) belongs to
    -- comment bi, so d works from anywhere in the hunk in either column.
    for row = lbase, #left - 1 do
      owner[row] = bi
    end
  end

  for _, buf in ipairs { lbuf, rbuf } do
    vim.bo[buf].modifiable = true
  end
  vim.api.nvim_buf_set_lines(lbuf, 0, -1, false, left)
  vim.api.nvim_buf_set_lines(rbuf, 0, -1, false, right)
  for _, buf in ipairs { lbuf, rbuf } do
    vim.bo[buf].modifiable = false
  end

  -- per-line gutter + muting (selected lines stay normal for syntax to show).
  local locs = {}
  for row = 0, #left - 1 do
    local m = meta[row + 1]
    if m and m.header then
      vim.api.nvim_buf_set_extmark(lbuf, ns, row, 0, { line_hl_group = "Title" })
    elseif m and m.code then
      locs[row] = { path = m.path, side = m.side, line = m.num }
      vim.api.nvim_buf_set_extmark(lbuf, ns, row, 0, {
        virt_text = { { string.format("%s %4d │ ", m.selected and "▸" or " ", m.num), "LineNr" } },
        virt_text_pos = "inline",
      })
      if not m.selected then
        vim.api.nvim_buf_set_extmark(lbuf, ns, row, 0, { line_hl_group = "Comment" })
      end
    elseif m and m.muted then
      vim.api.nvim_buf_set_extmark(lbuf, ns, row, 0, { line_hl_group = "Comment" })
    end
  end

  -- syntax-highlight each block's commented span (contiguous, so one parse).
  for _, b in ipairs(blocks) do
    if b.lang and b.left.rows then
      local lo, texts
      for j, r in ipairs(b.left.rows) do
        if r.selected then
          lo = lo or j
          texts = texts or {}
          texts[#texts + 1] = r.text
        end
      end
      if lo then
        -- header sits at b.lbase, so row j of rows is at b.lbase + j.
        ui.ts_highlight(lbuf, ns, b.lang, table.concat(texts, "\n"), b.lbase + lo)
      end
    end
  end

  return locs, owner
end

-- review opens a two-column preview of the pending review — code on the left,
-- comments on the right — then a/c/r choose a verdict (with an optional summary)
-- and submit it; q cancels. Shown in a throwaway tab so the diffview stays put.
function M.review()
  state.load()
  -- The preview is built entirely from local git: HEAD for RIGHT-side code, and
  -- base (one gh call) only when a LEFT-side comment actually needs it.
  local sha = util.sh { "git", "rev-parse", "HEAD" }
  local base
  for _, cm in ipairs(state.pending) do
    if cm.side == "LEFT" then
      local pr = gh.pr_info()
      base = pr and pr.base
      break
    end
  end

  local blocks = build_preview { sha = sha, base = base }
  -- root makes <CR> jumps work regardless of cwd; nil falls back to the
  -- repo-relative path, which still resolves when cwd is the worktree root.
  local root = util.sh { "git", "rev-parse", "--show-toplevel" }

  vim.cmd "tabnew"
  local lwin = vim.api.nvim_get_current_win()
  local lbuf = vim.api.nvim_get_current_buf()

  vim.cmd "rightbelow vsplit"
  local rwin = vim.api.nvim_get_current_win()
  local rbuf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_win_set_buf(rwin, rbuf)

  for _, buf in ipairs { lbuf, rbuf } do
    vim.bo[buf].buftype = "nofile"
    vim.bo[buf].bufhidden = "wipe"
  end
  local locs, owner = render(blocks, lbuf, rbuf)

  for _, win in ipairs { lwin, rwin } do
    vim.wo[win].number = false
    vim.wo[win].scrollbind = true
  end
  vim.wo[lwin].wrap = false
  vim.wo[rwin].wrap = true
  vim.wo[rwin].linebreak = true

  -- Row 1 (tabline, full width): title + the review-level commands that work
  -- from either pane.
  local review_tabline = ui.set_tab_title(lbuf, "jmux review", {
    ui.key_hint("a", "approve"),
    ui.key_hint("c", "comment"),
    ui.key_hint("r", "request"),
    ui.key_hint("D", "discard"),
    ui.key_hint("q", "cancel"),
  })

  -- Row 2 (winbar, per pane): the actions specific to each side.
  vim.wo[lwin].winbar = ui.bar("code", { ui.key_hint("⏎", "open"), ui.key_hint("d", "remove") })
  vim.wo[rwin].winbar = ui.bar("comments", { ui.key_hint("d", "remove") })
  vim.api.nvim_set_current_win(lwin)
  vim.cmd "syncbind"

  local closed = false
  -- stop_spin is set while a submit is in flight (it stops the tabline loader);
  -- its presence also marks the preview busy, so a second submit is refused.
  local stop_spin
  local function close()
    if not closed then
      closed = true
      if stop_spin then
        stop_spin()
      end
      pcall(vim.cmd, "tabclose")
    end
  end

  -- goto_code closes the preview and jumps to the file/line under the cursor.
  -- RIGHT lines map to the head, which is the working tree; LEFT (old) lines
  -- have no working-tree counterpart, so they're declined.
  local function goto_code()
    local loc = locs[vim.api.nvim_win_get_cursor(lwin)[1] - 1]
    if not loc then
      return
    end
    if loc.side ~= "RIGHT" then
      vim.notify("base-side line — not in the working tree", vim.log.levels.WARN)
      return
    end
    close()
    vim.cmd("edit " .. vim.fn.fnameescape(root and (root .. "/" .. loc.path) or loc.path))
    pcall(vim.api.nvim_win_set_cursor, 0, { loc.line, 0 })
    vim.cmd "normal! zz"
  end

  -- delete_under_cursor drops the comment whose hunk the cursor is in (either
  -- column), persists, and redraws the remaining hunks in place. Closing once
  -- the last one goes.
  local function delete_under_cursor()
    if #state.pending == 0 then
      return
    end
    local win = vim.api.nvim_get_current_win()
    local row = vim.api.nvim_win_get_cursor(win)[1]
    local idx = owner[row - 1]
    if not idx then
      return
    end
    table.remove(state.pending, idx)
    table.remove(blocks, idx)
    state.save()
    if #blocks == 0 then
      close()
      vim.notify "removed last comment; review is empty"
      return
    end
    locs, owner = render(blocks, lbuf, rbuf)
    local buf = win == rwin and rbuf or lbuf
    pcall(vim.api.nvim_win_set_cursor, win, { math.min(row, vim.api.nvim_buf_line_count(buf)), 0 })
    vim.cmd "syncbind"
    vim.notify(string.format("removed comment (%d left)", #state.pending))
  end

  local function choose(event)
    if stop_spin then
      vim.notify("a submit is already in progress…", vim.log.levels.WARN)
      return
    end
    vim.ui.input({ prompt = ("%s — summary (optional)> "):format(event) }, function(body)
      -- nil body means the input was cancelled; leave the preview open.
      if body == nil then
        return
      end
      -- Resolve the PR identity only now, at the moment of submit — a cache miss
      -- round trip is hidden behind the user having just typed the summary.
      local pr, err = gh.pr_info()
      if not pr then
        vim.notify(err, vim.log.levels.ERROR)
        return
      end
      local n = #state.pending
      vim.notify(("submitting %s…"):format(event))

      -- Animate a loader in the tabline while the submit is in flight (the call
      -- is async, so the editor stays responsive). stop_spin tears it down and is
      -- also the busy flag the guard above checks.
      local spin_stop
      stop_spin = function()
        stop_spin = nil
        if spin_stop then
          spin_stop()
        end
      end
      spin_stop = ui.spinner(function(frame)
        if closed or not stop_spin then
          return
        end
        vim.o.tabline = ("%%=%%#Title# %s submitting %s… %%*%%="):format(frame, event)
      end)

      post_review({ id = pr.id, sha = sha }, event, body, function(ok)
        local was_open = not closed
        if stop_spin then
          stop_spin()
        end
        if not ok then
          -- restore the command bar so the user can retry (unless they bailed).
          if was_open then
            vim.o.tabline = review_tabline
          end
          return
        end
        state.discard()
        -- the submitted review changes the conversation, so drop the pv cache;
        -- the next `pv` refetches rather than painting a copy without this review.
        state.clear_view_cache()
        vim.notify(string.format("submitted %s with %d comment(s)", event, n))
        close()
      end)
    end)
  end

  for _, buf in ipairs { lbuf, rbuf } do
    local opt = { buffer = buf, nowait = true }
    vim.keymap.set("n", "a", function()
      choose "APPROVE"
    end, opt)
    vim.keymap.set("n", "c", function()
      choose "COMMENT"
    end, opt)
    vim.keymap.set("n", "r", function()
      choose "REQUEST_CHANGES"
    end, opt)
    vim.keymap.set("n", "d", delete_under_cursor, opt)
    -- D discards the whole review; confirm since it can't be undone.
    vim.keymap.set("n", "D", function()
      vim.ui.select({ "no", "yes" }, { prompt = "discard all pending comments?" }, function(choice)
        if choice == "yes" then
          M.discard()
          close()
        end
      end)
    end, opt)
    vim.keymap.set("n", "q", close, opt)
  end
  -- only the code column knows a file/line under the cursor.
  vim.keymap.set("n", "<CR>", goto_code, { buffer = lbuf, nowait = true })
end

-- discard drops all staged comments, in memory and on disk.
function M.discard()
  state.discard()
  vim.notify "discarded pending comments"
end

return M
