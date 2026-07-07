-- Everything in the diffview tab: mapping diffview windows to GitHub sides,
-- gutter signs for pending comments and existing review threads, the thread
-- peek float with immediate replies, staging comments/suggestions, and opening
-- the PR diff itself.

local gh = require "jmux.pr.gh"
local state = require "jmux.pr.state"
local ui = require "jmux.pr.ui"
local util = require "jmux.pr.util"

local M = {}

-- diff_view returns the active diffview's { entry, layout }, or nil + reason when
-- diffview isn't loaded or no diff is open. Shared by everything that needs to map
-- diffview windows to GitHub sides (target's selection, decorate_diff's signs).
local function diff_view()
  local ok, lib = pcall(require, "diffview.lib")
  if not ok then
    return nil, "diffview not loaded"
  end
  local view = lib.get_current_view()
  if not view or not view.cur_entry or not view.cur_layout then
    return nil, "not in a diffview diff"
  end
  return { entry = view.cur_entry, layout = view.cur_layout }
end

-- target resolves the {path, side, start_line, line} for the current diffview
-- selection, or nil + reason when the cursor isn't in a reviewable diff window.
-- Diffview's right (new) buffer shows the head file, so its line numbers map
-- straight to GitHub's RIGHT side; the left buffer maps to LEFT/oldpath.
local function target()
  local v, err = diff_view()
  if not v then
    return nil, err
  end

  local entry = v.entry
  local layout = v.layout
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

-- cursor_diff resolves the current diff window to (path, side, buf, line), or nil
-- when the cursor isn't in a reviewable diff window.
local function cursor_diff()
  local v = diff_view()
  if not v then
    return nil
  end
  local win = vim.api.nvim_get_current_win()
  local main = v.layout:get_main_win()
  if main and win == main.id then
    return v.entry.path, "RIGHT", vim.api.nvim_win_get_buf(win), vim.api.nvim_win_get_cursor(win)[1]
  elseif v.layout.a and win == v.layout.a.id then
    return v.entry.oldpath, "LEFT", vim.api.nvim_win_get_buf(win), vim.api.nvim_win_get_cursor(win)[1]
  end
  return nil
end

-- Pending comments mark themselves in the diff gutter with a sign at the line
-- each one starts on. SIGN is the glyph (nf-fa-comment); change the codepoint to
-- taste. Built via nr2char so the source stays plain ASCII.
local SIGN_NS = vim.api.nvim_create_namespace "jmux_pr_signs"
local SIGN = vim.fn.nr2char(0xf075)

-- place_signs (re)draws the signs for one diff buffer: clears ours, then marks
-- every pending comment whose path/side match this buffer. Diffview shows the
-- whole file, so a comment's line maps straight to the buffer line.
local function place_signs(buf, path, side)
  if not (buf and vim.api.nvim_buf_is_valid(buf)) then
    return
  end
  vim.api.nvim_buf_clear_namespace(buf, SIGN_NS, 0, -1)
  if not path then
    return
  end
  local last = vim.api.nvim_buf_line_count(buf)
  for _, c in ipairs(state.pending) do
    if c.path == path and c.side == side then
      local l = c.start_line or c.line
      if l >= 1 and l <= last then
        pcall(vim.api.nvim_buf_set_extmark, buf, SIGN_NS, l - 1, 0, {
          sign_text = SIGN,
          sign_hl_group = "DiagnosticSignInfo",
        })
      end
    end
  end
end

-- threads holds existing review threads fetched for the PR; K on a thread sign
-- floats it. THREAD_SIGN marks an open discussion in the gutter (distinct from
-- your own SIGN); RESOLVED_SIGN dims resolved ones to a tick.
local THREAD_NS = vim.api.nvim_create_namespace "jmux_pr_threads"
local THREAD_SIGN = vim.fn.nr2char(0xf086)
local RESOLVED_SIGN = vim.fn.nr2char(0xf00c)
local threads = {}
local FLOAT_NS = vim.api.nvim_create_namespace "jmux_pr_thread_float"
local thread_float

-- THREADS_QUERY pulls review threads via GraphQL — REST's pulls/comments carries
-- no resolution state. reviewThreads are already grouped and flag isResolved; a
-- null line means the thread no longer maps to the current diff (outdated).
local THREADS_QUERY = [[
query($owner:String!,$repo:String!,$number:Int!){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$number){
      reviewThreads(first:100){
        nodes{
          id isResolved path line diffSide
          comments(first:100){ nodes{ author{login} body createdAt } }
        }
      }
    }
  }
}]]

-- parse_threads reads the decoded reviewThreads response into our thread shape.
-- Threads arrive pre-grouped; a null line means the thread no longer maps to the
-- current diff (outdated, usually resolved) and is dropped — pv still has the
-- full conversation.
local function parse_threads(data)
  local nodes = vim.tbl_get(data, "data", "repository", "pullRequest", "reviewThreads", "nodes")
  if type(nodes) ~= "table" then
    return {}
  end
  local out = {}
  for _, n in ipairs(nodes) do
    local line = util.nilable(n.line)
    if line then
      local comments = {}
      for _, c in ipairs(vim.tbl_get(n, "comments", "nodes") or {}) do
        comments[#comments + 1] = {
          login = (type(c.author) == "table" and c.author.login) or "?",
          body = type(c.body) == "string" and c.body or "",
          created_at = c.createdAt,
        }
      end
      out[#out + 1] = {
        id = n.id,
        path = n.path,
        side = n.diffSide == "LEFT" and "LEFT" or "RIGHT",
        line = math.floor(line),
        resolved = n.isResolved == true,
        comments = comments,
      }
    end
  end
  return out
end

-- decorate_diff is defined at the end of the decoration chain but used by
-- refresh_threads, so it's forward-declared.
local decorate_diff

-- refresh_threads refetches the PR's review threads and redraws the diff signs.
-- Fully async, so it never gates anything — signs layer in when the request
-- returns. Used on diff open and after posting a reply.
local function refresh_threads()
  local pr = gh.pr_info()
  if not pr then
    return
  end
  local owner, repo = gh.owner_repo(pr.slug)
  gh.graphql(THREADS_QUERY, { owner = owner, repo = repo, number = pr.number }, function(data)
    if not data then
      return
    end
    threads = parse_threads(data)
    decorate_diff()
  end)
end

-- REPLY_MUTATION posts a reply to a review thread. The thread node id is
-- globally unique, so no owner/repo/number is needed. pullRequestReviewId is
-- deliberately omitted: without it the reply posts immediately rather than
-- joining a pending review — except when the viewer already has one open
-- (e.g. started on the web), which GitHub attaches to; the returned comment
-- state exposes that so it can be surfaced.
local REPLY_MUTATION = [[
mutation($thread:ID!,$body:String!){
  addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$thread,body:$body}){
    comment{ state }
  }
}]]

-- reply_to_thread opens the editor for a reply to thread t and posts it on :w.
-- Unlike comments, replies don't stage: they're conversational, so they go up
-- immediately. The prompt closes itself on success and stays open on failure so
-- the text survives a retry; on_done runs after success so callers refresh what
-- they show. spin, when given, is a fn(label)→stop that animates a loader while
-- the post is in flight (pv passes its title spinner); without it a notify marks
-- the start.
function M.reply_to_thread(t, on_done, spin)
  if not t.id then
    vim.notify("thread has no id to reply to", vim.log.levels.WARN)
    return
  end
  local who = t.comments[1] and t.comments[1].login or "?"
  local loc = t.line and ("%s:%d"):format(t.path, t.line) or t.path
  local pbuf, posting
  ui.prompt_multiline(
    ("reply @%s · %s"):format(who, loc),
    function(body)
      if posting then
        vim.notify("reply already in flight…", vim.log.levels.WARN)
        return
      end
      posting = true
      local stop = spin and spin "posting reply…" or nil
      if not stop then
        vim.notify "posting reply…"
      end
      gh.graphql(REPLY_MUTATION, { thread = t.id, body = body }, function(data, err)
        if stop then
          stop()
        end
        if not data then
          posting = false
          vim.notify("reply failed:\n" .. (err or ""), vim.log.levels.ERROR)
          return
        end
        if pbuf and vim.api.nvim_buf_is_valid(pbuf) then
          pcall(vim.api.nvim_buf_delete, pbuf, { force = true })
        end
        -- the reply changes the conversation, so a cached pv would be stale.
        state.clear_view_cache()
        local comment_state = vim.tbl_get(data, "data", "addPullRequestReviewThreadReply", "comment", "state")
        if comment_state == "PENDING" then
          vim.notify("reply joined your pending review — it posts when that review is submitted", vim.log.levels.WARN)
        else
          vim.notify(("replied to @%s"):format(who))
        end
        if on_done then
          on_done()
        end
      end)
    end,
    nil,
    function(buf)
      pbuf = buf
    end,
    " :w reply · :q cancel "
  )
end

-- close_float dismisses the open thread peek, if any.
local function close_float()
  if thread_float and vim.api.nvim_win_is_valid(thread_float) then
    vim.api.nvim_win_close(thread_float, true)
  end
  thread_float = nil
end

-- open_thread_float pops a bordered peek of the thread near the cursor: a header
-- per comment (author · age, replies marked ↳) over its markdown body. Focused so
-- a long thread scrolls; q/<Esc> or moving away closes it, r replies.
local function open_thread_float(t)
  close_float()
  local lines, headers = {}, {}
  for i, c in ipairs(t.comments) do
    lines[#lines + 1] = (i == 1 and "" or "↳ ") .. c.login .. " · " .. util.reltime(c.created_at or "")
    headers[#headers + 1] = #lines - 1
    for _, bl in ipairs(vim.split(vim.trim((c.body or ""):gsub("\r", "")), "\n")) do
      lines[#lines + 1] = bl
    end
    if i < #t.comments then
      lines[#lines + 1] = ""
    end
  end

  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].filetype = "markdown"
  vim.bo[buf].modifiable = false
  for _, row in ipairs(headers) do
    vim.api.nvim_buf_set_extmark(buf, FLOAT_NS, row, 0, { line_hl_group = t.resolved and "Comment" or "Title" })
  end

  -- a comfortable reading column; wrap long bodies rather than widen the float.
  local width = math.min(64, math.floor(vim.o.columns * 0.8))
  local rows = 0
  for _, l in ipairs(lines) do
    rows = rows + math.max(1, math.ceil(vim.fn.strdisplaywidth(l) / width))
  end
  thread_float = vim.api.nvim_open_win(buf, true, {
    relative = "cursor",
    row = 1,
    col = 0,
    width = width,
    height = math.min(rows, math.floor(vim.o.lines * 0.4)),
    border = "rounded",
    style = "minimal",
    title = (t.resolved and " resolved" or " thread") .. " · r reply ",
    title_pos = "right",
  })
  vim.wo[thread_float].wrap = true
  vim.wo[thread_float].linebreak = true
  vim.keymap.set("n", "q", close_float, { buffer = buf, nowait = true })
  vim.keymap.set("n", "<Esc>", close_float, { buffer = buf, nowait = true })
  vim.keymap.set("n", "r", function()
    close_float()
    M.reply_to_thread(t, refresh_threads)
  end, { buffer = buf, nowait = true })
  vim.api.nvim_create_autocmd("WinLeave", { buffer = buf, once = true, callback = close_float })
end

-- place_threads (re)draws this buffer's thread gutter signs; K on one opens the
-- thread in a float.
local function place_threads(buf, path, side)
  if not (buf and vim.api.nvim_buf_is_valid(buf)) then
    return
  end
  vim.api.nvim_buf_clear_namespace(buf, THREAD_NS, 0, -1)
  if not path then
    return
  end
  local last = vim.api.nvim_buf_line_count(buf)
  for _, t in ipairs(threads) do
    if t.path == path and t.side == side and t.line and t.line >= 1 and t.line <= last then
      -- traffic-light vs your own Info-blue pending signs: open threads warn
      -- (need attention), resolved go green-tick (settled).
      pcall(vim.api.nvim_buf_set_extmark, buf, THREAD_NS, t.line - 1, 0, {
        sign_text = t.resolved and RESOLVED_SIGN or THREAD_SIGN,
        sign_hl_group = t.resolved and "DiagnosticSignOk" or "DiagnosticSignWarn",
      })
    end
  end
end

-- open_thread_under_cursor floats the thread anchored on the cursor line, or does
-- nothing when there isn't one (K's only job in the diff is to peek a thread).
local function open_thread_under_cursor()
  local path, side, _, line = cursor_diff()
  if not path then
    return
  end
  for _, t in ipairs(threads) do
    if t.path == path and t.side == side and t.line == line then
      open_thread_float(t)
      return
    end
  end
end

-- goto_thread moves to the next (dir 1) or previous (dir -1) thread in the current
-- file, floating it on arrival.
local function goto_thread(dir)
  local path, side, _, line = cursor_diff()
  if not path then
    return
  end
  local here = {}
  for _, t in ipairs(threads) do
    if t.path == path and t.side == side and t.line then
      here[#here + 1] = t
    end
  end
  table.sort(here, function(a, b)
    return a.line < b.line
  end)
  local target_t
  for i = 1, #here do
    local t = dir > 0 and here[i] or here[#here + 1 - i]
    if (dir > 0 and t.line > line) or (dir < 0 and t.line < line) then
      target_t = t
      break
    end
  end
  if not target_t then
    return
  end
  vim.api.nvim_win_set_cursor(0, { target_t.line, 0 })
  vim.cmd "normal! zz"
  open_thread_float(target_t)
end

-- pd_tab is the tabpage that `pd` opened the review diff in. Signs are scoped to
-- it so PR comments don't bleed into unrelated diffviews (neogit, ad-hoc :Diff…).
-- Tracked so re-invoking focuses the open tab instead of stacking a duplicate.
local pd_tab

-- decorate_one draws both your pending signs and the existing-thread signs for
-- one diff window, and binds the thread keys on its buffer (K peek, ]t/[t
-- navigate). Idempotent — re-run on every diffview event.
local function decorate_one(win, path, side)
  if not (win and vim.api.nvim_win_is_valid(win)) then
    return
  end
  local buf = vim.api.nvim_win_get_buf(win)
  place_signs(buf, path, side)
  place_threads(buf, path, side)
  local opt = { buffer = buf, nowait = true }
  vim.keymap.set("n", "K", open_thread_under_cursor, opt)
  vim.keymap.set("n", "]t", function()
    goto_thread(1)
  end, opt)
  vim.keymap.set("n", "[t", function()
    goto_thread(-1)
  end, opt)
end

-- decorate_diff signs the current diffview file's two windows (RIGHT = new/main,
-- LEFT = old). A no-op unless the pd-opened diff tab is current. Driven by
-- diffview's own events so the signs follow file navigation and reappear when the
-- view is re-entered.
decorate_diff = function()
  if not pd_tab or vim.api.nvim_get_current_tabpage() ~= pd_tab then
    return
  end
  local v = diff_view()
  if not v then
    return
  end
  local main = v.layout:get_main_win()
  if main then
    decorate_one(main.id, v.entry.path, "RIGHT")
  end
  if v.layout.a then
    decorate_one(v.layout.a.id, v.entry.oldpath, "LEFT")
  end
end

-- stage_comment opens the editor for the target span (prefilled with `initial`
-- when given) and stages the result in state.pending, updating in place on
-- re-save rather than queuing a duplicate. When `lang` is set, the prompt's
-- suggestion block is syntax-highlighted live in that language.
local function stage_comment(t, label, initial, lang)
  local span = util.span_str(t)
  local staged
  local on_open = lang
      and function(buf)
        ui.highlight_suggestion(buf, lang)
        vim.api.nvim_create_autocmd({ "TextChanged", "TextChangedI" }, {
          buffer = buf,
          callback = function()
            ui.highlight_suggestion(buf, lang)
          end,
        })
      end
    or nil
  ui.prompt_multiline(("%s %s:%s"):format(label, t.path, span), function(body)
    if staged then
      staged.body = body
    else
      staged = { path = t.path, side = t.side, line = t.line, body = body }
      if t.start_line ~= t.line then
        staged.start_line = t.start_line
        staged.start_side = t.side
      end
      table.insert(state.pending, staged)
    end
    state.save()
    decorate_diff()
    vim.notify(("staged %s:%s (%d pending)"):format(t.path, span, #state.pending))
  end, initial, on_open)
end

-- add_comment stages an inline comment on the selected line(s). Bind in normal
-- and visual mode; a visual selection becomes a multi-line comment.
function M.add_comment()
  state.load()
  local t, reason = target()
  if not t then
    vim.notify(reason, vim.log.levels.WARN)
    return
  end
  stage_comment(t, "comment")
end

-- suggest stages a GitHub suggestion on the selected line(s): a comment prefilled
-- with a ```suggestion block holding the current code, ready to edit into the
-- proposed change. Suggestions only apply to the new side, so LEFT is declined.
function M.suggest()
  state.load()
  local t, reason = target()
  if not t then
    vim.notify(reason, vim.log.levels.WARN)
    return
  end
  if t.side ~= "RIGHT" then
    vim.notify("suggestions apply to the new side only", vim.log.levels.WARN)
    return
  end
  local lines = vim.api.nvim_buf_get_lines(0, t.start_line - 1, t.line, false)
  stage_comment(t, "suggestion", "```suggestion\n" .. table.concat(lines, "\n") .. "\n```", ui.lang_of(t.path))
end

-- ensure_base makes sure the PR base commit is present in the local object store.
-- baseRefOid is GitHub's current tip of the base branch, which advances whenever
-- anything merges into it; if we haven't fetched since, that SHA is absent and
-- DiffviewOpen fails with "not a valid object name". GitHub serves fetch-by-sha,
-- so pull just that one commit on demand. Returns true once the base is available.
local function ensure_base(base)
  if util.sh { "git", "cat-file", "-e", base .. "^{commit}" } then
    return true
  end
  local _, err = util.sh { "git", "fetch", "--no-tags", "origin", base }
  if util.sh { "git", "cat-file", "-e", base .. "^{commit}" } then
    return true
  end
  return false, err
end

-- diff opens the PR's changes — base...HEAD, the merge-base range GitHub shows —
-- in diffview, where pending comments appear as gutter signs. load() first so a
-- restored review decorates on open.
function M.diff()
  if pd_tab and vim.api.nvim_tabpage_is_valid(pd_tab) then
    vim.api.nvim_set_current_tabpage(pd_tab)
    return
  end
  local pr, err = gh.pr_info()
  if not pr or not pr.base then
    vim.notify(err or "could not resolve PR base", vim.log.levels.ERROR)
    return
  end
  local ok_base, berr = ensure_base(pr.base)
  if not ok_base then
    vim.notify("could not fetch PR base " .. pr.base:sub(1, 7) .. ": " .. (berr or ""), vim.log.levels.ERROR)
    return
  end
  vim.cmd("DiffviewOpen " .. pr.base .. "...HEAD")
  -- :DiffviewOpen lands us in the new diff tab; remember it so only this view
  -- gets comment signs.
  pd_tab = vim.api.nvim_get_current_tabpage()
  -- Defer restoring the saved review (its git calls) off the open path, then
  -- draw signs. place_signs is idempotent, so any event-driven decorate that
  -- raced ahead with an empty review is simply corrected here.
  vim.schedule(function()
    state.load()
    decorate_diff()
  end)
  refresh_threads()
end

-- Keep the diff gutter in sync with pending comments as the user navigates
-- diffview files and re-enters the view. A cleared augroup keeps re-sourcing the
-- module (dev reload) idempotent rather than stacking duplicate callbacks.
vim.api.nvim_create_autocmd("User", {
  group = vim.api.nvim_create_augroup("jmux_pr", { clear = true }),
  pattern = { "DiffviewDiffBufWinEnter", "DiffviewViewEnter" },
  callback = vim.schedule_wrap(function()
    decorate_diff()
  end),
})

return M
