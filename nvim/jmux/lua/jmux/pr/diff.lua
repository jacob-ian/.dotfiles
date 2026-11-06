-- The diffview tab: GitHub-side mapping, gutter signs, thread peeks, comment
-- staging, and opening the PR diff.

local gh = require "jmux.pr.gh"
local state = require "jmux.pr.state"
local ui = require "jmux.pr.ui"
local util = require "jmux.pr.util"

local M = {}

-- pd_tab scopes signs and keys to our diffview tab, not neogit's or ad-hoc
-- ones. pd_mode is "pr" or "agent"; the two share the tab.
local pd_tab
local pd_mode
local title = ui.title_slot()

local refresh_threads, open_thread_under_cursor, goto_thread, goto_base

local function refresh()
  if pd_mode == "agent" then
    util.sh { "git", "add", "--intent-to-add", "--all" }
  else
    refresh_threads()
  end
  vim.cmd "DiffviewRefresh"
end

-- refresh_if_open re-reads the review diff from another tab; DiffviewRefresh
-- only sees the current one.
function M.refresh_if_open()
  if not (pd_tab and vim.api.nvim_tabpage_is_valid(pd_tab)) then
    return
  end
  local ok, lib = pcall(require, "diffview.lib")
  for _, view in ipairs(ok and lib.views or {}) do
    if view.tabpage == pd_tab and view.update_files then
      view:update_files()
    end
  end
end

-- KEYS is the diff windows' review keys; panel = true entries also bind in
-- the file panel.
local KEYS = ui.keyspec {
  {
    mode = "n",
    lhs = "K",
    rhs = function()
      open_thread_under_cursor()
    end,
  },
  {
    mode = "n",
    lhs = "]t",
    rhs = function()
      goto_thread(1)
    end,
  },
  {
    mode = "n",
    lhs = "[t",
    rhs = function()
      goto_thread(-1)
    end,
  },
  {
    mode = { "n", "x" },
    lhs = "c",
    rhs = function()
      M.add_comment()
    end,
    hint = "comment",
  },
  {
    mode = { "n", "x" },
    lhs = "s",
    rhs = function()
      M.suggest()
    end,
    hint = "suggest",
  },
  {
    mode = "n",
    lhs = "<C-s>",
    panel = true,
    rhs = function()
      if pd_mode == "agent" then
        require("jmux.pr.agent").send()
      else
        require("jmux.pr.review").review()
      end
    end,
    hint = function()
      return pd_mode == "agent" and "send to claude" or "submit review"
    end,
  },
  { mode = "n", lhs = "<C-r>", rhs = refresh, hint = "refresh", panel = true },
  { mode = "n", lhs = "q", rhs = "<cmd>DiffviewClose<cr>", hint = "close", panel = true },
  {
    mode = "n",
    lhs = "<CR>",
    rhs = function()
      goto_base()
    end,
    desc = "open file in base tab",
  },
}

-- diff_view returns the active diffview's { entry, layout }, or nil + reason.
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

-- target resolves the current diffview selection to {path, side, start_line,
-- line}, or nil + reason. Diffview's right buffer is GitHub's RIGHT side.
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

-- file_tab picks the tab goto_base edits in: the last-used tab, else the first
-- other one, provided it shows a plain file buffer (jmux and neogit tabs hold
-- scratch buffers an :edit would replace).
local function file_tab()
  local function plain(tab)
    local win = vim.api.nvim_tabpage_get_win(tab)
    return vim.bo[vim.api.nvim_win_get_buf(win)].buftype == ""
  end
  local tabs = vim.api.nvim_list_tabpages()
  local last = tabs[vim.fn.tabpagenr "#"]
  if last and plain(last) then
    return last
  end
  local cur = vim.api.nvim_get_current_tabpage()
  for _, tab in ipairs(tabs) do
    if tab ~= cur and plain(tab) then
      return tab
    end
  end
end

-- goto_base opens the diffed file in a base tab at the cursor line. LEFT
-- lines don't map to the working tree, so they're declined.
goto_base = function()
  local t, err = target()
  if not t then
    return vim.notify(err or "no file under cursor", vim.log.levels.WARN)
  end
  if t.side ~= "RIGHT" then
    return vim.notify("base-side line — open from the right pane", vim.log.levels.WARN)
  end
  local base = file_tab()
  if not base then
    return vim.notify("no base tab to open in", vim.log.levels.WARN)
  end
  local root = util.sh { "git", "rev-parse", "--show-toplevel" }
  vim.api.nvim_set_current_tabpage(base)
  vim.cmd("edit " .. vim.fn.fnameescape(root and (root .. "/" .. t.path) or t.path))
  pcall(vim.api.nvim_win_set_cursor, 0, { t.line, 0 })
  vim.cmd "normal! zz"
end

-- cursor_diff resolves the current diff window to (path, side, buf, line).
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

-- Diff windows are real file buffers, so decorations go through a lease and
-- are revoked when the view closes.
local lease = ui.lease()

local SIGN_NS = vim.api.nvim_create_namespace "jmux_pr_signs"
local SIGN = vim.fn.nr2char(0xf075)

-- place_signs redraws the pending-comment signs for one diff buffer.
local function place_signs(buf, path, side)
  if not (buf and vim.api.nvim_buf_is_valid(buf)) then
    return
  end
  vim.api.nvim_buf_clear_namespace(buf, SIGN_NS, 0, -1)
  if not path then
    return
  end
  lease.ns(buf, SIGN_NS)
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

local THREAD_NS = vim.api.nvim_create_namespace "jmux_pr_threads"
local THREAD_SIGN = vim.fn.nr2char(0xf086)
local RESOLVED_SIGN = vim.fn.nr2char(0xf00c)
local threads = {}
local FLOAT_NS = vim.api.nvim_create_namespace "jmux_pr_thread_float"
local thread_float = ui.slot(function(w)
  w:close()
end)

-- GraphQL, since REST's pulls/comments carries no resolution state.
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

-- parse_threads reads the reviewThreads response into our thread shape. A null
-- line means the thread is outdated for this diff, so it's dropped.
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

local decorate_diff

-- refresh_threads refetches the PR's review threads and redraws the signs.
refresh_threads = function()
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

-- pullRequestReviewId is omitted so the reply posts immediately; GitHub still
-- attaches it to a pending review the viewer already has open, which the
-- returned comment state exposes.
local REPLY_MUTATION = [[
mutation($thread:ID!,$body:String!){
  addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$thread,body:$body}){
    comment{ state }
  }
}]]

local RESOLVE_MUTATION = [[
mutation($thread:ID!){
  resolveReviewThread(input:{threadId:$thread}){ thread{ isResolved } }
}]]
local UNRESOLVE_MUTATION = [[
mutation($thread:ID!){
  unresolveReviewThread(input:{threadId:$thread}){ thread{ isResolved } }
}]]

-- resolve_thread toggles t's resolved state and runs on_done on success.
function M.resolve_thread(t, on_done)
  ui.act(t.resolved and "unresolving…" or "resolving…", function(done)
    gh.graphql(t.resolved and UNRESOLVE_MUTATION or RESOLVE_MUTATION, { thread = t.id }, done)
  end, function()
    state.clear_view_cache()
    if on_done then
      on_done()
    end
  end)
end

-- reply_to_thread opens the editor for a reply to t and posts it on :w; the
-- prompt stays open on failure so the text survives a retry.
function M.reply_to_thread(t, on_done)
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
      local stop = ui.spin "posting reply…"
      gh.graphql(REPLY_MUTATION, { thread = t.id, body = body }, function(data, err)
        stop()
        if not data then
          posting = false
          vim.notify("reply failed:\n" .. (err or ""), vim.log.levels.ERROR)
          return
        end
        if pbuf and vim.api.nvim_buf_is_valid(pbuf) then
          pcall(vim.api.nvim_buf_delete, pbuf, { force = true })
        end
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

-- open_thread_float peeks the thread in a float near the cursor.
local function open_thread_float(t)
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

  local width = math.min(64, math.floor(vim.o.columns * 0.8))
  local rows = 0
  for _, l in ipairs(lines) do
    rows = rows + math.max(1, math.ceil(vim.fn.strdisplaywidth(l) / width))
  end
  local win = require("snacks").win {
    relative = "cursor",
    row = 1,
    col = 0,
    width = width,
    height = math.min(rows, math.floor(vim.o.lines * 0.4)),
    border = "rounded",
    backdrop = false,
    title = (t.resolved and " resolved" or " thread")
      .. " · r reply · x "
      .. (t.resolved and "unresolve " or "resolve "),
    title_pos = "right",
    text = lines,
    bo = { filetype = "markdown", bufhidden = "wipe" },
    wo = { wrap = true, linebreak = true },
    keys = {
      q = thread_float.clear,
      ["<Esc>"] = thread_float.clear,
      r = function()
        thread_float.clear()
        M.reply_to_thread(t, refresh_threads)
      end,
      x = function()
        thread_float.clear()
        M.resolve_thread(t, refresh_threads)
      end,
    },
  }
  thread_float.set(win)
  vim.bo[win.buf].modifiable = false
  for _, row in ipairs(headers) do
    vim.api.nvim_buf_set_extmark(win.buf, FLOAT_NS, row, 0, { line_hl_group = t.resolved and "Comment" or "Title" })
  end
  vim.api.nvim_create_autocmd("WinLeave", { buffer = win.buf, once = true, callback = thread_float.clear })
end

-- place_threads redraws this buffer's thread signs.
local function place_threads(buf, path, side)
  if not (buf and vim.api.nvim_buf_is_valid(buf)) then
    return
  end
  vim.api.nvim_buf_clear_namespace(buf, THREAD_NS, 0, -1)
  if not path then
    return
  end
  lease.ns(buf, THREAD_NS)
  local last = vim.api.nvim_buf_line_count(buf)
  for _, t in ipairs(threads) do
    if t.path == path and t.side == side and t.line and t.line >= 1 and t.line <= last then
      pcall(vim.api.nvim_buf_set_extmark, buf, THREAD_NS, t.line - 1, 0, {
        sign_text = t.resolved and RESOLVED_SIGN or THREAD_SIGN,
        sign_hl_group = t.resolved and "DiagnosticSignOk" or "DiagnosticSignWarn",
      })
    end
  end
end

open_thread_under_cursor = function()
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

-- goto_thread moves to the next (dir 1) or previous (dir -1) thread in the file.
goto_thread = function(dir)
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

-- decorate_one signs one diff window and binds the review keys on its buffer.
local function decorate_one(win, path, side)
  if not (win and vim.api.nvim_win_is_valid(win)) then
    return
  end
  local buf = vim.api.nvim_win_get_buf(win)
  place_signs(buf, path, side)
  place_threads(buf, path, side)
  -- diff windows split from pv inherit its local number settings.
  for _, opt in ipairs { "number", "relativenumber" } do
    vim.api.nvim_set_option_value(opt, vim.api.nvim_get_option_value(opt, { scope = "global" }), { win = win })
  end
  KEYS.apply(buf, lease.map)
end

-- decorate_diff signs the current file's two windows; a no-op unless our tab
-- is current.
decorate_diff = function()
  if not pd_tab or vim.api.nvim_get_current_tabpage() ~= pd_tab then
    return
  end
  local ok, lib = pcall(require, "diffview.lib")
  local view = ok and lib.get_current_view() or nil
  local pbuf = view and view.panel and view.panel.bufid
  if pbuf and vim.api.nvim_buf_is_valid(pbuf) then
    for _, k in ipairs(KEYS.entries) do
      if k.panel then
        lease.map(k.mode or "n", k.lhs, k.rhs, { buffer = pbuf, nowait = true })
      end
    end
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

-- stage_comment opens the editor for the target span and stages the result,
-- updating in place on re-save.
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

-- add_comment stages an inline comment on the selected line(s).
function M.add_comment()
  state.load()
  local t, reason = target()
  if not t then
    vim.notify(reason, vim.log.levels.WARN)
    return
  end
  stage_comment(t, "comment")
end

-- suggest stages a GitHub suggestion on the selected line(s).
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

-- ensure_base fetches the PR base commit when it's missing locally; GitHub's
-- base tip advances with every merge, and GitHub serves fetch-by-sha.
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

-- focus_main puts the cursor in the head-side diff window, polling briefly
-- since diffview loads entries async; it never steals focus from another tab.
local function focus_main(tries)
  if not pd_tab or vim.api.nvim_get_current_tabpage() ~= pd_tab then
    return
  end
  local v = diff_view()
  local main = v and v.layout:get_main_win()
  if main and vim.api.nvim_win_is_valid(main.id) then
    vim.api.nvim_set_current_win(main.id)
    return
  end
  tries = tries or 20
  if tries > 0 then
    vim.defer_fn(function()
      focus_main(tries - 1)
    end, 50)
  end
end

-- claim_tab takes over the diff tab for mode: false when it already holds
-- that review, else closes the other review's tab.
local function claim_tab(mode, store)
  state.use(store)
  if pd_tab and vim.api.nvim_tabpage_is_valid(pd_tab) then
    vim.api.nvim_set_current_tabpage(pd_tab)
    if pd_mode == mode then
      return false
    end
    vim.cmd "tabclose"
  end
  pd_mode = mode
  return true
end

-- diff opens the PR's changes (merge-base...HEAD, as GitHub shows them).
function M.diff(selected)
  if not claim_tab("pr", state.PR_STORE) then
    return
  end
  local pr, err = gh.pr_info()
  if not pr or not pr.base then
    vim.notify(err or "could not resolve PR base", vim.log.levels.ERROR)
    return
  end
  -- not pr.base: the cached base-tip sha can sit behind a local rebase's fork
  -- point, pulling upstream commits into the diff.
  local base = pr.baseRef and util.sh { "git", "merge-base", "HEAD", "origin/" .. pr.baseRef } or nil
  if not base then
    base = pr.base
    local ok_base, berr = ensure_base(base)
    if not ok_base then
      vim.notify("could not fetch PR base " .. base:sub(1, 7) .. ": " .. (berr or ""), vim.log.levels.ERROR)
      return
    end
  end
  local cmd = "DiffviewOpen " .. base .. "...HEAD"
  if type(selected) == "string" and selected ~= "" then
    cmd = cmd .. " --selected-file=" .. vim.fn.fnameescape(selected)
  end
  vim.cmd(cmd)
  pd_tab = vim.api.nvim_get_current_tabpage()
  title.set(vim.api.nvim_get_current_buf(), "jmux pr diff", KEYS.hints())
  vim.schedule(function()
    state.load()
    decorate_diff()
  end)
  refresh_threads()
  focus_main()
end

-- review_worktree opens the working tree against HEAD with comment staging,
-- for the agent review.
function M.review_worktree()
  if not util.git_dir() then
    vim.notify("not in a git repo — agent review needs a HEAD to diff against", vim.log.levels.WARN)
    return
  end
  if not claim_tab("agent", state.AGENT_STORE) then
    return
  end
  threads = {}
  -- intent-to-add so untracked files show in the HEAD diff.
  util.sh { "git", "add", "--intent-to-add", "--all" }
  vim.cmd "DiffviewOpen HEAD"
  pd_tab = vim.api.nvim_get_current_tabpage()
  title.set(vim.api.nvim_get_current_buf(), "jmux agent review", KEYS.hints())
  vim.schedule(function()
    state.load()
    decorate_diff()
  end)
  focus_main()
end

-- jump_to_line places the cursor in path's head-side window, polling since
-- diffview loads entries async; the last window showing path is the head side.
local function jump_to_line(path, line, tries)
  if not line then
    return
  end
  local target
  if pd_tab and vim.api.nvim_tabpage_is_valid(pd_tab) then
    for _, w in ipairs(vim.api.nvim_tabpage_list_wins(pd_tab)) do
      local name = vim.api.nvim_buf_get_name(vim.api.nvim_win_get_buf(w))
      if name ~= "" and (name == path or name:sub(-#("/" .. path)) == "/" .. path) then
        target = w
      end
    end
  end
  if target then
    pcall(vim.api.nvim_win_set_cursor, target, { line, 0 })
    vim.api.nvim_win_call(target, function()
      vim.cmd "normal! zz"
    end)
    return
  end
  tries = tries or 20
  if tries > 0 then
    vim.defer_fn(function()
      jump_to_line(path, line, tries - 1)
    end, 50)
  end
end

-- open_at opens or focuses the PR diff at path:line.
function M.open_at(path, line)
  if pd_tab and vim.api.nvim_tabpage_is_valid(pd_tab) then
    vim.api.nvim_set_current_tabpage(pd_tab)
    local view = require("diffview.lib").get_current_view()
    if view and view.set_file_by_path then
      view:set_file_by_path(path, true)
    end
  else
    M.diff(path)
  end
  jump_to_line(path, line)
end

local group = vim.api.nvim_create_augroup("jmux_pr", { clear = true })
vim.api.nvim_create_autocmd("User", {
  group = group,
  pattern = { "DiffviewDiffBufWinEnter", "DiffviewViewEnter" },
  callback = vim.schedule_wrap(decorate_diff),
})

-- Diffview fires this for every view it closes, neogit's included, so only act
-- once our tab is gone (diffview closes the tab before emitting).
vim.api.nvim_create_autocmd("User", {
  group = group,
  pattern = "DiffviewViewClosed",
  callback = function()
    if not pd_tab or vim.api.nvim_tabpage_is_valid(pd_tab) then
      return
    end
    pd_tab = nil
    lease.revoke()
    title.clear()
  end,
})

return M
