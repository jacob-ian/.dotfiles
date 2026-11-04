-- The pending review: a two-column preview of staged comments (code left,
-- comment right), verdict selection, and the single GraphQL submit.

local gh = require "jmux.pr.gh"
local state = require "jmux.pr.state"
local ui = require "jmux.pr.ui"
local util = require "jmux.pr.util"

local M = {}

local CONTEXT = 2

-- One mutation keeps the submit atomic: no pending review to orphan on a
-- partial failure.
local ADD_REVIEW_MUTATION = [[
mutation($input:AddPullRequestReviewInput!){
  addPullRequestReview(input:$input){ pullRequestReview{ id } }
}]]

-- post_review submits the pending comments as one review and calls on_done(ok).
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

-- build_preview returns a block per pending comment: the commented code with
-- CONTEXT lines around it on the left, the comment body on the right.
local function build_preview(c)
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
    -- systemlist rather than sh so leading indentation survives.
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

-- render fills both column buffers, padded so they stay aligned under
-- scrollbind. The gutter is virtual text so treesitter column offsets line up.
-- Returns locs (left row → code location) and owner (row → pending index).
local function render(blocks, lbuf, rbuf)
  local ns = vim.api.nvim_create_namespace "jmux_pr_preview"
  vim.api.nvim_buf_clear_namespace(lbuf, ns, 0, -1)
  local left, right, meta, owner = {}, {}, {}, {}

  for bi, b in ipairs(blocks) do
    local lsec, lmeta = { b.left.header }, { { header = true } }
    local lbase = #left
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
        ui.ts_highlight(lbuf, ns, b.lang, table.concat(texts, "\n"), b.lbase + lo)
      end
    end
  end

  return locs, owner
end

-- review opens the two-column preview of the pending review in its own tab;
-- a/c/r choose a verdict and submit, q cancels.
function M.review()
  state.use(state.PR_STORE)
  state.load()
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
  local root = util.sh { "git", "rev-parse", "--show-toplevel" }

  local left = ui.pane { tab = true, wo = { scrollbind = true, wrap = false } }
  local right = ui.pane { split = "right", wo = { scrollbind = true, wrap = true, linebreak = true } }
  local lwin, lbuf, rwin, rbuf = left.win, left.buf, right.win, right.buf
  local group = ui.panegroup { panes = { left, right } }

  local locs, owner = render(blocks, lbuf, rbuf)

  local _, restore, _, bar_spin = ui.set_tab_title(lbuf, "jmux review", {
    ui.key_hint("a", "approve"),
    ui.key_hint("c", "comment"),
    ui.key_hint("r", "request"),
    ui.key_hint("D", "discard"),
    ui.key_hint("q", "cancel"),
  })

  local function winbar(win, wb)
    vim.api.nvim_set_option_value("winbar", wb, { scope = "local", win = win })
  end
  winbar(lwin, ui.bar("code", { ui.key_hint("⏎", "open"), ui.key_hint("d", "remove") }))
  winbar(rwin, ui.bar("comments", { ui.key_hint("d", "remove") }))
  vim.api.nvim_set_current_win(lwin)
  vim.cmd "syncbind"

  -- stop_spin is set while a submit is in flight and doubles as the busy flag.
  local stop_spin
  left.on_close.add(restore)
  left.on_close.add(function()
    if stop_spin then
      stop_spin()
    end
  end)
  local close = group.close

  -- goto_code jumps to the file/line under the cursor. LEFT lines have no
  -- working-tree counterpart, so they're declined.
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
      if body == nil then
        return
      end
      local pr, err = gh.pr_info()
      if not pr then
        vim.notify(err, vim.log.levels.ERROR)
        return
      end
      local n = #state.pending
      vim.notify(("submitting %s…"):format(event))

      local spin_stop = bar_spin(("submitting %s…"):format(event))
      stop_spin = function()
        stop_spin = nil
        spin_stop()
      end

      post_review({ id = pr.id, sha = sha }, event, body, function(ok)
        if stop_spin then
          stop_spin()
        end
        if not ok then
          return
        end
        state.discard()
        state.clear_view_cache()
        vim.notify(string.format("submitted %s with %d comment(s)", event, n))
        if event == "APPROVE" then
          util.prompt_remove_workspace(function()
            close()
            require("jmux.pr.view").view { refresh = true }
          end)
        else
          close()
          require("jmux.pr.view").view { refresh = true }
        end
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
  vim.keymap.set("n", "<CR>", goto_code, { buffer = lbuf, nowait = true })
end

-- discard drops all staged comments, in memory and on disk.
function M.discard()
  state.discard()
  vim.notify "discarded pending comments"
end

return M
