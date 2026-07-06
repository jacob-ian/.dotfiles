-- Lightweight GitHub PR review. Stage inline comments from diffview selections
-- into a pending review, then submit once with a verdict. Replaces octo for the
-- review flow. Works in a jmux PR worktree, where the checked-out branch is the
-- PR's head, so `gh` resolves the PR for every call with no number to pass.

local M = {}

-- pending holds staged comments in GitHub's review-comment shape. One nvim
-- instance maps to one PR worktree, so module-level state is sufficient.
M.pending = {}

-- info caches the PR's { slug, number, base } from a single `gh pr view`. It is
-- resolved lazily — the preview reads code from the local HEAD and needs none of
-- it, so the (slow) gh round trip stays off the path that opens the preview.
-- Cached for the whole session: one nvim instance maps to one PR worktree (so the
-- branch never changes under us), which is what keeps the cached base/number valid.
local info

-- CONTEXT is how many lines of code to show either side of a comment's span in
-- the submit preview.
local CONTEXT = 2

local function sh(cmd)
  local out = vim.fn.system(cmd)
  if vim.v.shell_error ~= 0 then
    return nil, vim.trim(out)
  end
  return vim.trim(out), nil
end

-- parse_pr_info turns `gh pr view --json number,baseRefOid,url` output into the
-- { slug, number, base } identity. The slug comes from the PR url path
-- (host-agnostic, works on GHE); a parse failure falls back to a repo lookup
-- rather than caching a half-resolved identity that would POST to repos/nil/...
local function parse_pr_info(out)
  local ok, data = pcall(vim.json.decode, out)
  if not ok or type(data) ~= "table" or not data.number then
    return nil, "gh pr view: unexpected output"
  end
  local slug = data.url and data.url:match "/([^/]+/[^/]+)/pull/"
  if not slug then
    local e
    slug, e = sh { "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner" }
    if not slug then
      return nil, "gh repo view: " .. (e or "")
    end
  end
  return { slug = slug, number = data.number, base = data.baseRefOid }
end

-- pr_info resolves the PR identity in one gh round trip, cached for the session.
-- Blocking — callers that must not freeze the editor use pr_info_async.
local function pr_info()
  if info then
    return info
  end
  local out, err = sh { "gh", "pr", "view", "--json", "number,baseRefOid,url" }
  if not out then
    return nil, "gh pr view: " .. (err or "")
  end
  local i, perr = parse_pr_info(out)
  info = i
  return i, perr
end

-- pr_info_async resolves the identity off the main loop: the cached value
-- immediately when present, else via vim.system so the editor stays responsive
-- (e.g. an animated loader keeps spinning) during the round trip.
local function pr_info_async(cb)
  if info then
    cb(info)
    return
  end
  vim.system({ "gh", "pr", "view", "--json", "number,baseRefOid,url" }, { text = true }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        cb(nil, "gh pr view: " .. vim.trim(res.stderr or ""))
        return
      end
      local i, perr = parse_pr_info(res.stdout)
      info = i
      cb(i, perr)
    end)
  end)
end

-- span_str renders a comment's line span as "12" or "12-15". Accepts both the
-- stored comment shape (start_line nil for a single line) and the target shape
-- (start_line always set, == line for a single line).
local function span_str(cm)
  local s = cm.start_line
  return (s and s ~= cm.line) and string.format("%d-%d", s, cm.line) or tostring(cm.line)
end

-- git_dir resolves the worktree's absolute git dir once per session. One nvim
-- instance maps to one PR worktree, so the path never changes under us — the same
-- invariant that lets `info` cache the PR identity. Memoizing it (false sentinel
-- caches a miss too) keeps the per-PR state files off a blocking `git rev-parse`
-- spawn on every read/write/clear, including the view's instant-paint open path.
local git_dir
local function resolve_git_dir()
  if git_dir == nil then
    git_dir = sh { "git", "rev-parse", "--absolute-git-dir" } or false
  end
  return git_dir or nil
end

-- Pending comments persist across sessions in the worktree's own git dir
-- (.git/worktrees/<name>/ for a linked worktree). That scopes the file to the
-- one PR this worktree tracks, hides it from `git status`, and lets `git
-- worktree remove` clean it up with everything else.
local function state_file()
  local dir = resolve_git_dir()
  return dir and (dir .. "/jmux-review.json") or nil
end

-- save writes M.pending through on every mutation (crash-safe; the payload is
-- tiny). An empty review removes the file rather than leaving an empty stub.
local function save()
  local f = state_file()
  if not f then
    return
  end
  if #M.pending == 0 then
    vim.fn.delete(f)
    return
  end
  vim.fn.writefile({ vim.json.encode { sha = sh { "git", "rev-parse", "HEAD" }, comments = M.pending } }, f)
end

-- load restores a persisted review the first time the module is touched in a
-- session. The stored HEAD sha is compared against the current one: comments are
-- anchored to line numbers on a specific commit, so a rebase/amend since the
-- last session may have shifted them, which is worth a warning.
local loaded = false
local function load()
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
  local head = sh { "git", "rev-parse", "HEAD" }
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

-- The PR conversation (pv) is cached to the worktree's git dir so reopening the
-- view paints instantly from disk instead of waiting on the GraphQL round trip;
-- <C-r> in the view refetches and rewrites it. Scoped to this worktree's PR like
-- the pending review, and cleaned up by `git worktree remove`.
local function view_cache_file()
  local dir = resolve_git_dir()
  return dir and (dir .. "/jmux-pr-view.json") or nil
end

-- write_view_cache stores the raw VIEW_QUERY response verbatim, so reading it
-- back decodes exactly what a live fetch would and the render path is identical.
local function write_view_cache(json)
  local f = view_cache_file()
  if f then
    vim.fn.writefile({ json }, f)
  end
end

-- read_view_cache returns the decoded cached response and its age in seconds, or
-- nil when there's no readable, decodable cache. Decoding here (rather than
-- handing finish the raw JSON to re-parse) avoids a second full decode of the
-- conversation payload on the main thread — finish renders the table directly. An
-- undecodable cache returns nil so the open falls through to a live fetch. Age
-- comes from the file mtime and is shown in the view's bar so a stale cache is
-- obvious (and a reminder that <C-r> refreshes it).
local function read_view_cache()
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
-- after submitting a review, which the stale cache wouldn't yet reflect.
local function clear_view_cache()
  local f = view_cache_file()
  if f then
    vim.fn.delete(f)
  end
end

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

-- lang_of maps a file path to a treesitter language (or nil), so each code block
-- can be highlighted with the right parser regardless of the others.
local function lang_of(path)
  local ft = vim.filetype.match { filename = path }
  if not ft then
    return nil
  end
  return vim.treesitter.language.get_lang(ft) or ft
end

-- ts_highlight parses `text` as `lang` and lays its syntax highlights onto the
-- buffer starting at row `base`. Used on a contiguous code span (preview hunk or
-- suggestion body) so node rows map straight to base + row. Best-effort — a
-- missing parser or query just leaves the lines unhighlighted. priority defaults
-- to 100 (over Normal); callers layering over other highlights pass higher.
local function ts_highlight(buf, ns, lang, text, base, priority)
  local ok, parser = pcall(vim.treesitter.get_string_parser, text, lang)
  if not ok or not parser then
    return
  end
  local tree = parser:parse()[1]
  local query = vim.treesitter.query.get(lang, "highlights")
  if not tree or not query then
    return
  end
  for id, node in query:iter_captures(tree:root(), text, 0, -1) do
    local name = query.captures[id]
    -- captures starting with "_" are query internals, not highlight groups.
    if not name:find "^_" then
      local sr, sc, er, ec = node:range()
      pcall(vim.api.nvim_buf_set_extmark, buf, ns, base + sr, sc, {
        end_row = base + er,
        end_col = ec,
        hl_group = "@" .. name,
        priority = priority or 100,
      })
    end
  end
end

-- highlight_suggestion finds the ```suggestion fence in a comment buffer and
-- syntax-highlights its body in the reviewed file's language, so a suggestion
-- reads like code rather than plain markdown. Re-run on edit to stay live; a
-- high priority keeps it above markdown's own raw-block highlighting.
local SUGGEST_NS = vim.api.nvim_create_namespace "jmux_pr_suggestion"
local function highlight_suggestion(buf, lang)
  vim.api.nvim_buf_clear_namespace(buf, SUGGEST_NS, 0, -1)
  if not lang then
    return
  end
  local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  local open, close
  for i, l in ipairs(lines) do
    if not open then
      if l:match "^```suggestion%s*$" then
        open = i
      end
    elseif l:match "^```%s*$" then
      close = i
      break
    end
  end
  if not open or not close or close <= open + 1 then
    return
  end
  local code = {}
  for i = open + 1, close - 1 do
    code[#code + 1] = lines[i]
  end
  -- open is the fence's 1-indexed line, so the first code line is row `open`.
  ts_highlight(buf, SUGGEST_NS, lang, table.concat(code, "\n"), open, 200)
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
  for _, c in ipairs(M.pending) do
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

local function nilable(v)
  if v == nil or v == vim.NIL then
    return nil
  end
  return v
end

-- The local UTC offset, computed once at load. os.time reads a broken-down table
-- as local time, so converting a UTC timestamp needs this constant added back.
-- Taken from "now", so a timestamp in the opposite DST period can be off by up to
-- an hour — fine for the coarse relative ages we render.
local utc_offset = os.difftime(os.time(os.date "*t"), os.time(os.date "!*t"))

-- iso_epoch converts an ISO-8601 UTC timestamp to a Unix epoch. os.time reads a
-- broken-down table as local time, so feeding it the UTC fields yields
-- epoch - offset; adding the offset back recovers the true UTC epoch.
local function iso_epoch(iso)
  local y, mo, d, h, mi, s = tostring(iso):match "(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)"
  if not y then
    return nil
  end
  return os.time {
    year = tonumber(y),
    month = tonumber(mo),
    day = tonumber(d),
    hour = tonumber(h),
    min = tonumber(mi),
    sec = tonumber(s),
  } + utc_offset
end

-- short_age renders a positive seconds delta as "now/5m/3h/2d/4mo".
local function short_age(diff)
  if diff < 60 then
    return "now"
  elseif diff < 3600 then
    return math.floor(diff / 60) .. "m"
  elseif diff < 86400 then
    return math.floor(diff / 3600) .. "h"
  elseif diff < 2592000 then
    return math.floor(diff / 86400) .. "d"
  end
  return math.floor(diff / 2592000) .. "mo"
end

-- reltime renders a timestamp as a short age: "now/5m/3h/2d/4mo".
local function reltime(iso)
  local at = iso_epoch(iso)
  if not at then
    return ""
  end
  return short_age(os.time() - at)
end

-- duration renders the elapsed time between two timestamps as "45s/2m5s/1h3m".
local function duration(from, to)
  local a, b = iso_epoch(from), iso_epoch(to)
  if not a or not b or b < a then
    return ""
  end
  local d = b - a
  if d < 60 then
    return d .. "s"
  elseif d < 3600 then
    local m, s = math.floor(d / 60), d % 60
    return s > 0 and (m .. "m" .. s .. "s") or (m .. "m")
  end
  local h, m = math.floor(d / 3600), math.floor((d % 3600) / 60)
  return m > 0 and (h .. "h" .. m .. "m") or (h .. "h")
end

-- parse_threads reads the GraphQL reviewThreads response into our thread shape.
-- Threads arrive pre-grouped; a null line means the thread no longer maps to the
-- current diff (outdated, usually resolved) and is dropped — pv still has the
-- full conversation.
local function parse_threads(json)
  local ok, data = pcall(vim.json.decode, json)
  if not ok then
    return {}
  end
  local nodes = vim.tbl_get(data, "data", "repository", "pullRequest", "reviewThreads", "nodes")
  if type(nodes) ~= "table" then
    return {}
  end
  local out = {}
  for _, n in ipairs(nodes) do
    local line = nilable(n.line)
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

-- close_float dismisses the open thread peek, if any.
local function close_float()
  if thread_float and vim.api.nvim_win_is_valid(thread_float) then
    vim.api.nvim_win_close(thread_float, true)
  end
  thread_float = nil
end

-- open_thread_float pops a bordered peek of the thread near the cursor: a header
-- per comment (author · age, replies marked ↳) over its markdown body. Focused so
-- a long thread scrolls; q/<Esc> or moving away closes it.
local function open_thread_float(t)
  close_float()
  local lines, headers = {}, {}
  for i, c in ipairs(t.comments) do
    lines[#lines + 1] = (i == 1 and "" or "↳ ") .. c.login .. " · " .. reltime(c.created_at or "")
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
    title = t.resolved and " resolved " or " thread ",
    title_pos = "right",
  })
  vim.wo[thread_float].wrap = true
  vim.wo[thread_float].linebreak = true
  vim.keymap.set("n", "q", close_float, { buffer = buf, nowait = true })
  vim.keymap.set("n", "<Esc>", close_float, { buffer = buf, nowait = true })
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
  local target
  for i = 1, #here do
    local t = dir > 0 and here[i] or here[#here + 1 - i]
    if (dir > 0 and t.line > line) or (dir < 0 and t.line < line) then
      target = t
      break
    end
  end
  if not target then
    return
  end
  vim.api.nvim_win_set_cursor(0, { target.line, 0 })
  vim.cmd "normal! zz"
  open_thread_float(target)
end

-- pd_tab is the tabpage that `pd` opened the review diff in. Signs are scoped to
-- it so PR comments don't bleed into unrelated diffviews (neogit, ad-hoc :Diff…).
-- pv_tab is the `pv` view tab. Both are tracked so re-invoking focuses the open
-- tab instead of stacking a duplicate.
local pd_tab
local pv_tab

-- decorate_diff signs the current diffview file's two windows (RIGHT = new/main,
-- LEFT = old). A no-op unless the pd-opened diff tab is current. Driven by
-- diffview's own events so the signs follow file navigation and reappear when the
-- view is re-entered.
-- decorate_one draws both your pending signs and the existing-thread signs for
-- one diff window, and binds the thread keys on its buffer (<CR> toggle, ]t/[t
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

local function decorate_diff()
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

-- prompt_multiline opens a floating scratch buffer for a multi-line body and
-- behaves like editing a file: :w runs on_save(text) (re-runnable to update),
-- :q closes, :q! discards, and :q on unsaved edits warns as usual. Used because
-- vim.ui.input — and snacks' own input — are single-line. An optional `initial`
-- string prefills the buffer (e.g. a suggestion block to edit); on_open(buf), if
-- given, runs once the buffer exists (used to attach live suggestion highlights).
-- footer overrides the default hint line (e.g. "save" vs "stage").
local function prompt_multiline(title, on_save, initial, on_open, footer)
  require("snacks").win {
    width = 0.6,
    height = 0.35,
    border = "rounded",
    title = " " .. title .. " ",
    footer = footer or " :w stage · :q close ",
    -- acwrite (not nofile) so :w fires BufWriteCmd instead of erroring.
    bo = { filetype = "markdown", buftype = "acwrite", bufhidden = "wipe" },
    wo = { wrap = true },
    -- Drop snacks' default q=close so a stray q can't silently discard the
    -- comment; closing goes through :q, which respects unsaved changes.
    keys = { q = false },
    on_buf = function(self)
      vim.api.nvim_buf_set_name(self.buf, "pr://comment/" .. self.buf)
      if initial then
        vim.api.nvim_buf_set_lines(self.buf, 0, -1, false, vim.split(initial, "\n"))
      end
      if on_open then
        on_open(self.buf)
      end
      vim.api.nvim_create_autocmd("BufWriteCmd", {
        buffer = self.buf,
        callback = function()
          local body = vim.trim(table.concat(vim.api.nvim_buf_get_lines(self.buf, 0, -1, false), "\n"))
          if body ~= "" then
            on_save(body)
          end
          vim.bo[self.buf].modified = false
        end,
      })
    end,
  }
  -- Prefilled prompts open in normal mode so the user can navigate to the line
  -- they want to change; an empty prompt drops straight into insert.
  if not initial then
    vim.cmd "startinsert"
  end
end

-- stage_comment opens the editor for the target span (prefilled with `initial`
-- when given) and stages the result in M.pending, updating in place on re-save
-- rather than queuing a duplicate. When `lang` is set, the prompt's suggestion
-- block is syntax-highlighted live in that language.
local function stage_comment(t, label, initial, lang)
  local span = span_str(t)
  local staged
  local on_open = lang
      and function(buf)
        highlight_suggestion(buf, lang)
        vim.api.nvim_create_autocmd({ "TextChanged", "TextChangedI" }, {
          buffer = buf,
          callback = function()
            highlight_suggestion(buf, lang)
          end,
        })
      end
    or nil
  prompt_multiline(("%s %s:%s"):format(label, t.path, span), function(body)
    if staged then
      staged.body = body
    else
      staged = { path = t.path, side = t.side, line = t.line, body = body }
      if t.start_line ~= t.line then
        staged.start_line = t.start_line
        staged.start_side = t.side
      end
      table.insert(M.pending, staged)
    end
    save()
    decorate_diff()
    vim.notify(("staged %s:%s (%d pending)"):format(t.path, span, #M.pending))
  end, initial, on_open)
end

-- add_comment stages an inline comment on the selected line(s). Bind in normal
-- and visual mode; a visual selection becomes a multi-line comment.
function M.add_comment()
  load()
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
  load()
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
  stage_comment(t, "suggestion", "```suggestion\n" .. table.concat(lines, "\n") .. "\n```", lang_of(t.path))
end

-- post_review POSTs the pending comments as one review with the chosen verdict
-- and optional summary body, then calls on_done(true) on success or on_done(false)
-- after notifying the failure. Async (vim.system) so the network round trip
-- doesn't freeze the editor; the payload is serialized up front, so a later edit
-- to M.pending can't change what's already in flight.
local function post_review(c, event, body, on_done)
  local payload = vim.json.encode {
    commit_id = c.sha,
    event = event,
    body = (body and body ~= "") and body or nil,
    comments = (#M.pending > 0) and M.pending or nil,
  }
  vim.system({
    "gh",
    "api",
    "--method",
    "POST",
    string.format("repos/%s/pulls/%d/reviews", c.slug, c.number),
    "--input",
    "-",
  }, { stdin = payload }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        local msg = res.stderr ~= "" and res.stderr or res.stdout
        vim.notify("submit failed:\n" .. vim.trim(msg or ""), vim.log.levels.ERROR)
        on_done(false)
        return
      end
      on_done(true)
    end)
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

  if #M.pending == 0 then
    return {
      {
        left = { header = "▌ no inline comments staged", rows = {} },
        -- mirror the header on the right; there's nothing to pair below it.
        right = { header = "▌ no inline comments staged", body = {} },
      },
    }
  end

  local blocks = {}
  for _, cm in ipairs(M.pending) do
    local rev = cm.side == "LEFT" and c.base or c.sha
    local lines, err = rev_lines(rev, cm.path)
    local first, last = cm.start_line or cm.line, cm.line
    local block = {
      lang = lang_of(cm.path),
      left = {
        header = string.format("▌ %s:%s  [%s]", cm.path, span_str(cm), cm.side),
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
-- owner (row → index of the owning comment in M.pending, shared by both columns
-- since they're padded to the same per-block row range, for d). Re-runnable: it
-- clears its own namespace first so it can redraw after a comment is removed.
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
        ts_highlight(lbuf, ns, b.lang, table.concat(texts, "\n"), b.lbase + lo)
      end
    end
  end

  return locs, owner
end

-- key_hint renders an emphasized key glyph followed by a muted label for a
-- status bar; stock highlight groups so it tracks whatever colorscheme is active.
local function key_hint(key, label)
  return ("%%#Special#%s %%#Comment#%s"):format(key, label)
end

-- bar builds a statusline-syntax string (for tabline or winbar): a Title-styled
-- label on the left and the hints flush right.
local function bar(label, hints)
  return ("%%#Title# %s %%*%%=%s "):format(label, table.concat(hints, "  "))
end

-- set_tab_title shows a full-width jmux bar (label + hints) in the tabline, but
-- scoped to its own tabpage: the tabline is a global option, so without this it
-- would bleed onto other tabs (e.g. pv left open while you switch to pd). A
-- TabEnter watcher re-applies the bar on this tab and restores the previous
-- tabline on any other; BufWipeout tears it all down. Returns the bar string (for
-- callers that re-show it after a transient overlay) and a restore fn.
local function set_tab_title(buf, label, hints)
  local saved_tabline, saved_showtabline = vim.o.tabline, vim.o.showtabline
  local title = bar(label, hints)
  local tab = vim.api.nvim_get_current_tabpage()
  local group = vim.api.nvim_create_augroup("jmux_tabtitle_" .. tostring(buf), { clear = true })
  local function restore()
    vim.o.tabline, vim.o.showtabline = saved_tabline, saved_showtabline
  end
  local function refresh()
    if vim.api.nvim_tabpage_is_valid(tab) and vim.api.nvim_get_current_tabpage() == tab then
      vim.o.tabline, vim.o.showtabline = title, 2
    else
      restore()
    end
  end
  -- set_hints rebuilds the bar (e.g. to reveal a key once data confirms it
  -- applies) and re-applies it if this tab is current.
  local function set_hints(new_hints)
    title = bar(label, new_hints)
    refresh()
  end
  refresh()
  vim.api.nvim_create_autocmd("TabEnter", { group = group, callback = refresh })
  vim.api.nvim_create_autocmd("BufWipeout", {
    group = group,
    buffer = buf,
    once = true,
    callback = function()
      restore()
      pcall(vim.api.nvim_del_augroup_by_id, group)
    end,
  })
  return title, restore, set_hints
end

-- SPINNER frames for the in-flight submit loader.
local SPINNER = { "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏" }

-- review opens a two-column preview of the pending review — code on the left,
-- comments on the right — then a/c/r choose a verdict (with an optional summary)
-- and POST it; q cancels. Shown in a throwaway tab so the diffview stays put.
function M.review()
  load()
  -- The preview is built entirely from local git: HEAD for RIGHT-side code, and
  -- base (one gh call) only when a LEFT-side comment actually needs it.
  local sha = sh { "git", "rev-parse", "HEAD" }
  local base
  for _, cm in ipairs(M.pending) do
    if cm.side == "LEFT" then
      local pr = pr_info()
      base = pr and pr.base
      break
    end
  end

  local blocks = build_preview { sha = sha, base = base }
  -- root makes <CR> jumps work regardless of cwd; nil falls back to the
  -- repo-relative path, which still resolves when cwd is the worktree root.
  local root = sh { "git", "rev-parse", "--show-toplevel" }

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
  local review_tabline = set_tab_title(lbuf, "jmux review", {
    key_hint("a", "approve"),
    key_hint("c", "comment"),
    key_hint("r", "request"),
    key_hint("D", "discard"),
    key_hint("q", "cancel"),
  })

  -- Row 2 (winbar, per pane): the actions specific to each side.
  vim.wo[lwin].winbar = bar("code", { key_hint("⏎", "open"), key_hint("d", "remove") })
  vim.wo[rwin].winbar = bar("comments", { key_hint("d", "remove") })
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
    if #M.pending == 0 then
      return
    end
    local win = vim.api.nvim_get_current_win()
    local row = vim.api.nvim_win_get_cursor(win)[1]
    local idx = owner[row - 1]
    if not idx then
      return
    end
    table.remove(M.pending, idx)
    table.remove(blocks, idx)
    save()
    if #blocks == 0 then
      close()
      vim.notify "removed last comment; review is empty"
      return
    end
    locs, owner = render(blocks, lbuf, rbuf)
    local buf = win == rwin and rbuf or lbuf
    pcall(vim.api.nvim_win_set_cursor, win, { math.min(row, vim.api.nvim_buf_line_count(buf)), 0 })
    vim.cmd "syncbind"
    vim.notify(string.format("removed comment (%d left)", #M.pending))
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
      -- Resolve the PR identity only now, at the moment of submit — the gh round
      -- trip is hidden behind the user having just typed the summary.
      local pr, err = pr_info()
      if not pr then
        vim.notify(err, vim.log.levels.ERROR)
        return
      end
      local n = #M.pending
      vim.notify(("submitting %s…"):format(event))

      -- Animate a loader in the tabline while the POST is in flight (the call is
      -- async, so the editor stays responsive). stop_spin tears it down and is
      -- also the busy flag the guard above checks.
      local timer, frame = vim.uv.new_timer(), 0
      stop_spin = function()
        stop_spin = nil
        timer:stop()
        timer:close()
      end
      timer:start(
        0,
        80,
        vim.schedule_wrap(function()
          if closed or not stop_spin then
            return
          end
          frame = frame % #SPINNER + 1
          vim.o.tabline = ("%%=%%#Title# %s submitting %s… %%*%%="):format(SPINNER[frame], event)
        end)
      )

      post_review({ slug = pr.slug, number = pr.number, sha = sha }, event, body, function(ok)
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
        M.pending = {}
        save()
        -- the submitted review changes the conversation, so drop the pv cache;
        -- the next `pv` refetches rather than painting a copy without this review.
        clear_view_cache()
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

-- discard drops all staged comments, in memory and on disk. loaded is forced so
-- a later add_comment in the same session can't resurrect the cleared review.
function M.discard()
  loaded = true
  M.pending = {}
  save()
  vim.notify "discarded pending comments"
end

-- VIEW_QUERY pulls the whole conversation for the bespoke `pv`: body, timeline
-- comments, reviews (with bodies), inline review threads, and the latest commit's
-- checks — `gh pr view --comments` omits the inline review comments entirely.
local VIEW_QUERY = [[
query($owner:String!,$repo:String!,$number:Int!){
  viewer{ login }
  repository(owner:$owner,name:$repo){
    viewerDefaultMergeMethod
    pullRequest(number:$number){
      title number state createdAt author{login} body
      viewerDidAuthor assignees(first:20){ nodes{ login } }
      reviewRequests(first:50){ nodes{ requestedReviewer{
        __typename ... on User{ login } ... on Team{ name slug } } } }
      isDraft mergeable reviewDecision mergeStateStatus
      latestOpinionatedReviews(first:50){ nodes{ state } }
      comments(first:100){ nodes{ author{login} body createdAt } }
      reviews(first:100){ nodes{ id author{login} body state createdAt } }
      reviewThreads(first:100){ nodes{ id isResolved path line
        comments(first:100){ nodes{ author{login} body createdAt pullRequestReview{id} } } } }
      commits(last:1){ nodes{ commit{ statusCheckRollup{ contexts(first:100){ nodes{
        __typename
        ... on CheckRun{ name conclusion status startedAt completedAt }
        ... on StatusContext{ context state }
      } } } } } }
    }
  }
}]]

-- check_mark maps a status-check node to (icon, highlight): green tick passed,
-- red cross failed, yellow dot running, muted circle skipped/neutral.
local function check_mark(node)
  local state = nilable(node.conclusion) or nilable(node.state)
  if not state then
    return "●", "DiagnosticWarn"
  end
  state = tostring(state):upper()
  if state == "SUCCESS" then
    return "✓", "DiagnosticOk"
  elseif state == "PENDING" or state == "EXPECTED" then
    return "●", "DiagnosticWarn"
  elseif state == "NEUTRAL" or state == "SKIPPED" or state == "CANCELLED" or state == "STALE" then
    return "○", "Comment"
  end
  return "✗", "DiagnosticError"
end

-- merge_summary distils review + merge state into a one-line readiness summary
-- (icon, highlight, text) for an open PR, or nil for merged/closed/draft-less
-- cases where the status line already says it all.
local function merge_summary(pr)
  if tostring(pr.state or ""):upper() ~= "OPEN" then
    return nil
  end
  -- latestOpinionatedReviews is the latest approve/request-changes per author
  -- (ignoring later plain comments), matching how GitHub tallies approvals.
  local approvals = 0
  for _, r in ipairs(vim.tbl_get(pr, "latestOpinionatedReviews", "nodes") or {}) do
    if tostring(r.state or ""):upper() == "APPROVED" then
      approvals = approvals + 1
    end
  end
  -- unresolved threads block the merge when the branch requires resolution; that
  -- rule isn't readable without push access, so surface the count regardless.
  local unresolved = 0
  for _, t in ipairs(vim.tbl_get(pr, "reviewThreads", "nodes") or {}) do
    if not t.isResolved then
      unresolved = unresolved + 1
    end
  end
  local mss = tostring(nilable(pr.mergeStateStatus) or ""):upper()
  local decision = tostring(nilable(pr.reviewDecision) or ""):upper()
  local mergeable = tostring(nilable(pr.mergeable) or ""):upper()
  local icon, hl, state = "●", "DiagnosticWarn", "pending"
  if pr.isDraft then
    icon, hl, state = "○", "Comment", "draft"
  elseif mergeable == "CONFLICTING" or mss == "DIRTY" then
    icon, hl, state = "✗", "DiagnosticError", "conflicts"
  elseif decision == "CHANGES_REQUESTED" then
    icon, hl, state = "✗", "DiagnosticError", "changes requested"
  elseif mss == "CLEAN" then
    icon, hl, state = "✓", "DiagnosticOk", "ready to merge"
  elseif mss == "BEHIND" then
    icon, hl, state = "●", "DiagnosticWarn", "behind base"
  elseif mss == "UNSTABLE" then
    icon, hl, state = "●", "DiagnosticWarn", "checks failing"
  elseif decision == "REVIEW_REQUIRED" then
    icon, hl, state = "●", "DiagnosticWarn", "review required"
  elseif mss == "BLOCKED" then
    icon, hl, state = "●", "DiagnosticWarn", "blocked"
  elseif decision == "APPROVED" then
    icon, hl, state = "✓", "DiagnosticOk", "approved"
  end
  local parts = { approvals == 1 and "1 approval" or (approvals .. " approvals") }
  if unresolved > 0 then
    parts[#parts + 1] = unresolved .. " unresolved"
  end
  parts[#parts + 1] = state
  return icon, hl, table.concat(parts, " · ")
end

-- build_model turns the VIEW_QUERY response into { header, pr, checks, blocks }:
-- the static title/status header, then collapsible blocks — the PR description,
-- the checks summary, and the time-ordered conversation. Each block is
-- { id, header, body, kids, body_marks? }; a review's inline threads are its kids
-- (each kid { id, header, lines }); body_marks colour the check icons.
local function build_model(data)
  local pr = vim.tbl_get(data, "data", "repository", "pullRequest")
  if type(pr) ~= "table" then
    return { header = { "  no PR data" }, blocks = {} }
  end
  local function who(n)
    return vim.tbl_get(n, "author", "login") or "?"
  end
  -- GitHub returns bodies with CRLF endings; strip the \r so it doesn't render
  -- as a trailing ^M on every line.
  local function split(text)
    return vim.split(vim.trim((text or ""):gsub("\r", "")), "\n")
  end

  local header = {
    ("# %s  #%s"):format(pr.title or "", tostring(pr.number or "")),
    ("`%s` · @%s · %s"):format(tostring(pr.state or ""):lower(), who(pr), reltime(pr.createdAt or "")),
  }
  local header_marks = {}
  local micon, mhl, mtext = merge_summary(pr)
  if micon then
    header[#header + 1] = ("%s %s"):format(micon, mtext)
    header_marks[#header_marks + 1] = { row = #header - 1, col = 0, col_end = #micon, hl = mhl }
  end

  -- requested reviewers the PR is still waiting on (GitHub drops a request from
  -- this list once that reviewer submits). Users render as @login, teams as their
  -- name. A muted hourglass keeps it distinct from the merge-summary line above.
  local requested = {}
  for _, rr in ipairs(vim.tbl_get(pr, "reviewRequests", "nodes") or {}) do
    local r = rr.requestedReviewer
    if type(r) == "table" then
      if r.login then
        requested[#requested + 1] = "@" .. r.login
      elseif r.name then
        requested[#requested + 1] = r.name
      end
    end
  end
  if #requested > 0 then
    local icon = "◷"
    header[#header + 1] = ("%s awaiting %s"):format(icon, table.concat(requested, ", "))
    header_marks[#header_marks + 1] = { row = #header - 1, col = 0, col_end = #icon, hl = "DiagnosticWarn" }
  end

  header[#header + 1] = ""

  -- the PR description is its own collapsible block, shown above the checks.
  local pr_block = {
    id = "pr",
    header = ("@%s · opened · %s"):format(who(pr), reltime(pr.createdAt or "")),
    body = split(pr.body),
    kids = {},
  }

  -- checks are a collapsible block: a one-line summary ("1 failed" / "2 running"
  -- / "all passed") that expands to the per-check list. Icon colours are stored
  -- relative to the body (body_marks) and positioned at render time.
  local checks_block
  local checks = vim.tbl_get(pr, "commits", "nodes", 1, "commit", "statusCheckRollup", "contexts", "nodes")
  if type(checks) == "table" and #checks > 0 then
    local clines, cmarks, fail, run = {}, {}, 0, 0
    for _, c in ipairs(checks) do
      local icon, hl = check_mark(c)
      local name = c.name or c.context or "?"
      local dur = duration(c.startedAt, c.completedAt)
      clines[#clines + 1] = dur ~= "" and ("%s %s · %s"):format(icon, name, dur) or ("%s %s"):format(icon, name)
      cmarks[#cmarks + 1] = { rel = #clines - 1, col = 0, col_end = #icon, hl = hl }
      if hl == "DiagnosticError" then
        fail = fail + 1
      elseif hl == "DiagnosticWarn" then
        run = run + 1
      end
    end
    local summary = (fail > 0 and fail .. " failed") or (run > 0 and run .. " running") or "all passed"
    local icon, hl = "✓", "DiagnosticOk"
    if fail > 0 then
      icon, hl = "✗", "DiagnosticError"
    elseif run > 0 then
      icon, hl = "●", "DiagnosticWarn"
    end
    checks_block = {
      id = "checks",
      header = ("%s Checks · %s"):format(icon, summary),
      header_mark = { col = 0, col_end = #icon, hl = hl },
      body = clines,
      body_marks = cmarks,
      kids = {},
    }
  end

  -- group inline threads under the review that opened them.
  local by_review, orphans = {}, {}
  for _, t in ipairs(vim.tbl_get(pr, "reviewThreads", "nodes") or {}) do
    local rid = vim.tbl_get(t, "comments", "nodes", 1, "pullRequestReview", "id")
    if rid then
      by_review[rid] = by_review[rid] or {}
      table.insert(by_review[rid], t)
    else
      orphans[#orphans + 1] = t
    end
  end
  local function thread_kid(t)
    local loc = t.path or "?"
    if nilable(t.line) then
      loc = loc .. ":" .. tostring(t.line)
    end
    local comments = vim.tbl_get(t, "comments", "nodes") or {}
    -- header: author/time · status; the location sits beneath it, then the body
    -- (replies marked ↳).
    local status = t.isResolved and "✓ resolved" or "● open"
    local lines = { ("`%s`"):format(loc) }
    for i, c in ipairs(comments) do
      if i > 1 then
        lines[#lines + 1] = ("↳ @%s · %s"):format(who(c), reltime(c.createdAt or ""))
      end
      vim.list_extend(lines, split(c.body))
      lines[#lines + 1] = ""
    end
    local root = comments[1]
    return {
      id = t.id or loc,
      header = root and ("@%s · %s · %s"):format(who(root), reltime(root.createdAt or ""), status)
        or (loc .. " · " .. status),
      lines = lines,
    }
  end

  -- time-ordered conversation (ISO-8601 sorts chronologically as a string).
  local blocks = {}
  for _, c in ipairs(vim.tbl_get(pr, "comments", "nodes") or {}) do
    blocks[#blocks + 1] = {
      at = c.createdAt or "",
      id = "c" .. tostring(c.createdAt) .. who(c),
      header = ("@%s · %s"):format(who(c), reltime(c.createdAt or "")),
      body = split(c.body),
      kids = {},
    }
  end
  for _, r in ipairs(vim.tbl_get(pr, "reviews", "nodes") or {}) do
    local rt = (r.id and by_review[r.id]) or {}
    local verdict = tostring(r.state or ""):upper()
    -- show a review when it carries a body, inline threads, or a verdict — the
    -- last so a bare "Approve" / "Request changes" still shows who weighed in.
    if vim.trim(r.body or "") ~= "" or #rt > 0 or verdict == "APPROVED" or verdict == "CHANGES_REQUESTED" then
      local kids = {}
      for _, t in ipairs(rt) do
        kids[#kids + 1] = thread_kid(t)
      end
      blocks[#blocks + 1] = {
        at = r.createdAt or "",
        id = r.id or ("r" .. tostring(r.createdAt)),
        header = ("@%s · %s · %s"):format(who(r), tostring(r.state or ""):lower(), reltime(r.createdAt or "")),
        body = vim.trim(r.body or "") ~= "" and split(r.body) or {},
        kids = kids,
      }
    end
  end
  table.sort(blocks, function(a, b)
    return a.at < b.at
  end)
  -- threads not tied to a shown review become their own top-level blocks.
  for _, t in ipairs(orphans) do
    local k = thread_kid(t)
    blocks[#blocks + 1] = { at = "", id = k.id, header = k.header, body = k.lines, kids = {} }
  end

  return { header = header, header_marks = header_marks, pr = pr_block, checks = checks_block, blocks = blocks }
end

-- REVIEWERS_QUERY lists the repo's assignable users — the set that can be
-- requested for review — for the `R` picker. 100 is plenty for our repos; a
-- larger org would need pagination.
local REVIEWERS_QUERY = [[
query($owner:String!,$repo:String!){
  repository(owner:$owner,name:$repo){
    assignableUsers(first:100){ nodes{ login name } }
  }
}]]

-- TEAMS_QUERY lists the owning org's teams (requestable as reviewers). It's
-- best-effort: a user-owned repo has no organization (and read:org may be
-- missing), so the call can fail — the picker then just offers users.
local TEAMS_QUERY = [[
query($owner:String!){
  organization(login:$owner){ teams(first:100){ nodes{ slug name } } }
}]]

-- gh_graphql_nodes runs a one-variable-or-two GraphQL query and returns the node
-- list at path via cb (the empty list on any failure, so callers can be lax).
local function gh_graphql_nodes(args, path, cb)
  vim.system(args, { text = true }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        cb {}
        return
      end
      local ok, data = pcall(vim.json.decode, res.stdout)
      cb((ok and vim.tbl_get(data, unpack(path))) or {})
    end)
  end)
end

-- request_review lets you pick a reviewer — an assignable user or an org team —
-- and adds them via `gh pr edit`. The author and anyone/any team already
-- requested are filtered out (excluded by login, excludedTeams by slug). on_done
-- runs after success so the caller can refresh. Single-pick: press R to add more.
local function request_review(pr, excluded, excludedTeams, on_done)
  local owner, repo = pr.slug:match "([^/]+)/(.+)"
  vim.notify "fetching reviewers…"

  local function pick(items)
    if #items == 0 then
      vim.notify("no reviewers left to request", vim.log.levels.WARN)
      return
    end
    vim.ui.select(items, {
      prompt = "request review from:",
      format_item = function(it)
        return it.kind == "team" and (it.label .. "  [team]") or it.label
      end,
    }, function(choice)
      if not choice then
        return
      end
      vim.system(
        { "gh", "pr", "edit", tostring(pr.number), "--repo", pr.slug, "--add-reviewer", choice.value },
        { text = true },
        function(r2)
          vim.schedule(function()
            if r2.code ~= 0 then
              vim.notify("request review: " .. vim.trim(r2.stderr or ""), vim.log.levels.ERROR)
              return
            end
            vim.notify("requested review from " .. choice.label)
            if on_done then
              on_done()
            end
          end)
        end
      )
    end)
  end

  -- users first (required), then teams (best effort), then the combined picker.
  gh_graphql_nodes({
    "gh", "api", "graphql",
    "-f", "query=" .. REVIEWERS_QUERY,
    "-f", "owner=" .. (owner or ""),
    "-f", "repo=" .. (repo or ""),
  }, { "data", "repository", "assignableUsers", "nodes" }, function(users)
    local items = {}
    for _, u in ipairs(users) do
      if u.login and not excluded[u.login] then
        local name = nilable(u.name)
        items[#items + 1] = {
          kind = "user",
          value = u.login,
          label = name and name ~= "" and ("%s (%s)"):format(u.login, name) or ("@" .. u.login),
          sort = u.login:lower(),
        }
      end
    end
    gh_graphql_nodes({
      "gh", "api", "graphql",
      "-f", "query=" .. TEAMS_QUERY,
      "-f", "owner=" .. (owner or ""),
    }, { "data", "organization", "teams", "nodes" }, function(teams)
      for _, t in ipairs(teams) do
        if t.slug and not excludedTeams[t.slug] then
          items[#items + 1] = {
            kind = "team",
            value = (owner or "") .. "/" .. t.slug,
            label = nilable(t.name) or t.slug,
            sort = "~" .. t.slug:lower(), -- "~" sorts teams after users
          }
        end
      end
      table.sort(items, function(a, b)
        return a.sort < b.sort
      end)
      pick(items)
    end)
  end)
end

-- edit_description opens the PR body in the multiline editor (prefilled with the
-- current text) and writes it back with `gh pr edit` on :w. on_done runs after a
-- successful save so the caller can refresh. gh rejects the edit if you lack
-- permission. An empty buffer is a no-op (prompt_multiline won't save it), so the
-- description can't be cleared from here — edit on the web for that.
local function edit_description(pr, body, on_done)
  -- GitHub returns the body with CRLF endings; strip the \r so the edit buffer
  -- doesn't show a trailing ^M on every line.
  body = (body or ""):gsub("\r", "")
  prompt_multiline(("edit #%d description"):format(pr.number), function(text)
    vim.system(
      { "gh", "pr", "edit", tostring(pr.number), "--repo", pr.slug, "--body-file", "-" },
      { stdin = text, text = true },
      function(res)
        vim.schedule(function()
          if res.code ~= 0 then
            vim.notify("edit description: " .. vim.trim(res.stderr or ""), vim.log.levels.ERROR)
            return
          end
          vim.notify(("updated #%d description"):format(pr.number))
          if on_done then
            on_done()
          end
        end)
      end
    )
  end, body ~= "" and body or nil, nil, " :w save · :q close ")
end

-- view renders the full PR conversation — body, comments, reviews, inline review
-- threads, and checks — into a markdown tab. Fetched async (a "Loading…" stub
-- shows first) so opening is instant; q closes it.
function M.view()
  if pv_tab and vim.api.nvim_tabpage_is_valid(pv_tab) then
    vim.api.nvim_set_current_tabpage(pv_tab)
    return
  end
  -- Open the tab and paint the loader first; resolving the PR and fetching the
  -- conversation both happen async (pr_info_async + vim.system), so the view
  -- shows instantly and the spinner keeps animating during the round trip.
  vim.cmd "tabnew"
  pv_tab = vim.api.nvim_get_current_tabpage()
  local win = vim.api.nvim_get_current_win()
  local buf = vim.api.nvim_get_current_buf()
  vim.bo[buf].buftype = "nofile"
  vim.bo[buf].bufhidden = "wipe"
  vim.bo[buf].filetype = "markdown"
  vim.wo[win].wrap = true
  vim.wo[win].linebreak = true
  vim.wo[win].breakindent = true
  vim.wo[win].conceallevel = 2
  -- keep markdown concealed even on the cursor line; this is a read-only view, so
  -- revealing raw syntax on hover just makes lines jump around.
  vim.wo[win].concealcursor = "nvic"
  vim.wo[win].number = false
  vim.wo[win].relativenumber = false

  local function fill(lines)
    vim.bo[buf].modifiable = true
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
    vim.bo[buf].modifiable = false
  end

  local _, restore, set_hints =
    set_tab_title(buf, "jmux pr view", { key_hint("⇥", "expand"), key_hint("q", "close") })
  vim.api.nvim_create_autocmd("WinClosed", {
    pattern = tostring(win),
    once = true,
    callback = function()
      restore()
      pv_tab = nil
    end,
  })
  vim.keymap.set("n", "q", "<cmd>tabclose<cr>", { buffer = buf, nowait = true, desc = "close" })

  -- Draw the first frame synchronously and force a redraw so the loader shows the
  -- instant the tab opens, rather than waiting ~100ms for the first timer tick;
  -- the timer below animates it from there.
  local frame = 1
  local function draw_loader()
    vim.bo[buf].modifiable = true
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, { SPINNER[frame] .. " Loading PR…" })
    vim.bo[buf].modifiable = false
  end
  draw_loader()
  vim.cmd "redraw"

  local timer = vim.uv.new_timer()
  local function stop_loader()
    if timer then
      timer:stop()
      timer:close()
      timer = nil
    end
  end
  if timer then
    timer:start(
      100,
      100,
      vim.schedule_wrap(function()
        if not timer or not vim.api.nvim_buf_is_valid(buf) then
          stop_loader()
          return
        end
        frame = frame % #SPINNER + 1
        draw_loader()
      end)
    )
  end

  pr_info_async(function(pr, err)
    if not vim.api.nvim_buf_is_valid(buf) then
      stop_loader()
      return
    end
    if not pr then
      stop_loader()
      fill { "  failed to resolve PR: " .. (err or "") }
      return
    end
    local owner, repo = pr.slug:match "([^/]+)/(.+)"

    -- the PR description starts open; everything else collapsed. Hoisted above
    -- fetch_and_render so expand/collapse state survives a merge-triggered refresh.
    local expanded = { pr = true }

    -- re-runnable so a successful merge can refresh the view in place. stop_spin,
    -- when passed, halts a merge spinner the instant the refreshed data arrives.
    local function fetch_and_render(stop_spin, cached)
      -- remember the cursor line so a refresh keeps your place across the
      -- re-render (expand state is preserved, so the line stays meaningful).
      local prev_line = (vim.api.nvim_win_is_valid(win) and vim.api.nvim_win_get_cursor(win)[1]) or 1

      -- finish renders one response into the view and wires its keymaps. A cached
      -- open passes the already-decoded table as `predecoded` (and a synthesized
      -- { code = 0 } res), so the render path is identical to a live fetch without
      -- re-parsing the payload; `age` (the cache's age in seconds, nil for a live
      -- fetch) is shown in the bar so a stale copy is obvious and signposts the
      -- <C-r> refresh.
      local function finish(res, age, predecoded)
        vim.schedule(function()
          stop_loader()
          if stop_spin then
            stop_spin()
          end
          if not vim.api.nvim_buf_is_valid(buf) then
            return
          end
          if res.code ~= 0 then
            return fill { "", "  failed to load PR:", "", "  " .. vim.trim(res.stderr or "") }
          end
          local data = predecoded
          if not data then
            local ok
            ok, data = pcall(vim.json.decode, res.stdout)
            if not ok then
              return fill { "  failed to parse PR response" }
            end
          end
          -- a live fetch (age nil) refreshes the on-disk cache; a cached open
          -- (age set) renders without rewriting what it just read.
          if not age then
            write_view_cache(res.stdout)
          end

          -- Collapsible tree: header → PR description → checks → conversation. Each
          -- item shows a ▸/▾ header; expanding a review reveals its body and inline
          -- threads, which expand in turn. Re-rendered on toggle; check-icon marks are
          -- repositioned since the description above them changes height.
          local model = build_model(data)
          local ns = vim.api.nvim_create_namespace "jmux_pr_view"
          local rows_by_id, id_by_row = {}, {}

          local function render()
            local lines = vim.list_extend({}, model.header)
            rows_by_id, id_by_row = {}, {}
            local marks = {}
            -- indent body lines under their title (blank lines stay blank so markdown
            -- paragraph breaks survive); any body_marks shift with the indent.
            local function add_body(src, indent, body_marks)
              local start = #lines
              for _, l in ipairs(src) do
                lines[#lines + 1] = l ~= "" and (indent .. l) or ""
              end
              for _, m in ipairs(body_marks or {}) do
                marks[#marks + 1] =
                  { row = start + m.rel, col = m.col + #indent, col_end = m.col_end + #indent, hl = m.hl }
              end
            end
            local function emit(b)
              local row = #lines
              rows_by_id[b.id], id_by_row[row] = row, b.id
              local prefix = expanded[b.id] and "▾ " or "▸ "
              lines[#lines + 1] = prefix .. b.header
              if b.header_mark then
                local pad = #prefix
                marks[#marks + 1] = {
                  row = row,
                  col = pad + b.header_mark.col,
                  col_end = pad + b.header_mark.col_end,
                  hl = b.header_mark.hl,
                }
              end
              if expanded[b.id] then
                add_body(b.body, "  ", b.body_marks)
                for _, k in ipairs(b.kids) do
                  local krow = #lines
                  rows_by_id[k.id], id_by_row[krow] = krow, k.id
                  lines[#lines + 1] = "  " .. (expanded[k.id] and "▾ " or "▸ ") .. k.header
                  if expanded[k.id] then
                    add_body(k.lines, "    ")
                  end
                end
              end
              lines[#lines + 1] = ""
            end

            if model.pr then
              emit(model.pr)
            end
            if model.checks then
              emit(model.checks)
            end
            for _, b in ipairs(model.blocks) do
              emit(b)
            end

            fill(lines)
            vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
            -- header marks sit at fixed top rows; block marks at their rendered rows.
            for _, m in ipairs(model.header_marks or {}) do
              marks[#marks + 1] = m
            end
            for _, m in ipairs(marks) do
              pcall(vim.api.nvim_buf_set_extmark, buf, ns, m.row, m.col, {
                end_col = m.col_end,
                hl_group = m.hl,
                priority = 200,
              })
            end
          end

          -- resolve the window from the keypress, not the captured `win`: the buffer
          -- can outlive its original window (e.g. :split then close the first), and
          -- a stale win id would throw E5108.
          local function toggle()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= buf then
              return
            end
            local id = id_by_row[vim.api.nvim_win_get_cursor(w)[1] - 1]
            if not id then
              return
            end
            expanded[id] = not expanded[id] or nil
            render()
            pcall(vim.api.nvim_win_set_cursor, w, { (rows_by_id[id] or 0) + 1, 0 })
          end

          render()
          for _, key in ipairs { "<Tab>", "<CR>" } do
            vim.keymap.set("n", key, toggle, { buffer = buf, nowait = true, desc = "toggle thread" })
          end

          -- title_spinner animates the tab title (scoped via set_hints) until the
          -- returned stop fn is called; shared by refresh and merge.
          local function title_spinner(label)
            local t, f = vim.uv.new_timer(), 0
            local function stop()
              if t then
                t:stop()
                t:close()
                t = nil
              end
            end
            t:start(
              0,
              80,
              vim.schedule_wrap(function()
                if not t then
                  return
                end
                f = f % #SPINNER + 1
                set_hints { ("%%#Title#%s %s"):format(SPINNER[f], label) }
              end)
            )
            return stop
          end

          vim.keymap.set("n", "<C-r>", function()
            fetch_and_render(title_spinner "refreshing…")
          end, { buffer = buf, nowait = true, desc = "refresh" })

          -- R requests a reviewer: pick from the repo's assignable users, skipping
          -- the author and anyone already requested, then refresh so the new
          -- "awaiting" line shows. gh rejects the request if you lack permission.
          local prnode = vim.tbl_get(data, "data", "repository", "pullRequest")
          local viewer = nilable(vim.tbl_get(data, "data", "viewer", "login"))
          local excluded = { [viewer or ""] = true }
          local excludedTeams = {}
          local author = nilable(vim.tbl_get(prnode or {}, "author", "login"))
          if author then
            excluded[author] = true
          end
          for _, rr in ipairs(vim.tbl_get(prnode or {}, "reviewRequests", "nodes") or {}) do
            local rv = rr.requestedReviewer
            if type(rv) == "table" then
              if rv.login then
                excluded[rv.login] = true
              elseif rv.slug then
                excludedTeams[rv.slug] = true
              end
            end
          end
          vim.keymap.set("n", "R", function()
            request_review(pr, excluded, excludedTeams, function()
              fetch_and_render(title_spinner "refreshing…")
            end)
          end, { buffer = buf, nowait = true, desc = "request review" })

          -- mine: the PR is yours to act on — you authored it or are assigned. Gates
          -- editing the description (E) and merging (m/M) so they only show when
          -- usable.
          local mine = prnode and prnode.viewerDidAuthor
          if prnode and not mine then
            for _, a in ipairs(vim.tbl_get(prnode, "assignees", "nodes") or {}) do
              if a.login == viewer then
                mine = true
                break
              end
            end
          end

          -- E edits the PR description in the multiline editor, then refreshes so the
          -- updated body shows. Dropped then re-added only while mine, like m/M, so a
          -- stale binding can't linger if the PR stops being yours.
          pcall(vim.keymap.del, "n", "E", { buffer = buf })
          if mine then
            vim.keymap.set("n", "E", function()
              edit_description(pr, nilable(prnode and prnode.body) or "", function()
                fetch_and_render(title_spinner "refreshing…")
              end)
            end, { buffer = buf, nowait = true, desc = "edit description" })
          end

          -- m merges — only on open PRs you authored or are assigned to, using the
          -- repo's default merge method. Hints are rebuilt here every render, so the
          -- merge spinner is cleanly replaced once it resolves.
          local hints = { key_hint("<C-r>", "refresh"), key_hint("R", "request") }
          if mine then
            hints[#hints + 1] = key_hint("E", "edit")
          end
          hints[#hints + 1] = key_hint("⇥", "expand")
          hints[#hints + 1] = key_hint("q", "close")
          -- a cached paint flags its age so it's clear the data may be stale and
          -- <C-r> will refetch; a live fetch (age nil) shows no such marker.
          if age then
            hints[#hints + 1] = ("%%#Comment#cached %s ago"):format(short_age(age))
          end
          -- drop any merge bindings from a prior render; re-added below only while
          -- still eligible, so a stale m/M can't merge an already-merged PR.
          pcall(vim.keymap.del, "n", "m", { buffer = buf })
          pcall(vim.keymap.del, "n", "M", { buffer = buf })
          local method = nilable(vim.tbl_get(data, "data", "repository", "viewerDefaultMergeMethod"))
          -- allowlist the method to a literal flag instead of interpolating it into
          -- argv, so crafted API data can never inject an arbitrary `gh` flag.
          local how = method and tostring(method):lower()
          local flag = how and ({ merge = "--merge", squash = "--squash", rebase = "--rebase" })[how]
          if mine and flag and tostring(nilable(prnode.state) or ""):upper() == "OPEN" then
            -- admin bypasses branch protection (failing checks, unresolved threads).
            local function do_merge(admin)
              local label = admin and (how .. ", admin") or how
              vim.ui.select(
                { "no", "yes" },
                { prompt = ("merge #%d via %s?"):format(pr.number, label) },
                function(choice)
                  if choice ~= "yes" then
                    return
                  end
                  -- animate the tab title while merging; it keeps spinning through the
                  -- refresh that follows a successful merge.
                  local stop_spin = title_spinner(("merging #%d…"):format(pr.number))
                  local cmd = { "gh", "pr", "merge", "--repo", pr.slug, flag }
                  if admin then
                    cmd[#cmd + 1] = "--admin"
                  end
                  cmd[#cmd + 1] = "--"
                  cmd[#cmd + 1] = tostring(pr.number)
                  vim.system(cmd, { text = true }, function(res)
                    vim.schedule(function()
                      if res.code ~= 0 then
                        stop_spin()
                        set_hints(hints)
                        vim.notify("merge failed:\n" .. vim.trim(res.stderr or ""), vim.log.levels.ERROR)
                      else
                        vim.notify(("merged #%d (%s)"):format(pr.number, label))
                        -- keep the spinner up until the refreshed view renders.
                        fetch_and_render(stop_spin)
                      end
                    end)
                  end)
                end
              )
            end
            vim.keymap.set("n", "m", function()
              do_merge(false)
            end, { buffer = buf, nowait = true, desc = "merge" })
            vim.keymap.set("n", "M", function()
              do_merge(true)
            end, { buffer = buf, nowait = true, desc = "merge (admin)" })
            table.insert(hints, 1, key_hint("M", "merge (admin)"))
            table.insert(hints, 1, key_hint("m", "merge"))
          end
          set_hints(hints)

          pcall(vim.api.nvim_win_set_cursor, win, { math.min(prev_line, vim.api.nvim_buf_line_count(buf)), 0 })
        end)
      end

      if cached then
        finish({ code = 0 }, cached.age, cached.data)
      else
        vim.system({
          "gh",
          "api",
          "graphql",
          "-f",
          "query=" .. VIEW_QUERY,
          "-f",
          "owner=" .. (owner or ""),
          "-f",
          "repo=" .. (repo or ""),
          "-F",
          "number=" .. pr.number,
        }, { text = true }, function(res)
          finish(res)
        end)
      end
    end

    -- Paint from the on-disk cache when present so the view opens with no network
    -- round trip; <C-r> refetches and rewrites it. Fall through to a live fetch
    -- only when there's no cache yet.
    local cached_data, cached_age = read_view_cache()
    if cached_data then
      fetch_and_render(nil, { data = cached_data, age = cached_age })
    else
      fetch_and_render()
    end
  end)
end

-- browser opens the current branch's PR on github.com. Async so launching the
-- browser doesn't block nvim; only failures are surfaced.
function M.browser()
  vim.system({ "gh", "pr", "view", "--web" }, {}, function(res)
    if res.code ~= 0 then
      local msg = res.stderr ~= "" and res.stderr or res.stdout
      vim.schedule(function()
        vim.notify("gh pr view --web: " .. vim.trim(msg or ""), vim.log.levels.ERROR)
      end)
    end
  end)
end

-- ensure_base makes sure the PR base commit is present in the local object store.
-- baseRefOid is GitHub's current tip of the base branch, which advances whenever
-- anything merges into it; if we haven't fetched since, that SHA is absent and
-- DiffviewOpen fails with "not a valid object name". GitHub serves fetch-by-sha,
-- so pull just that one commit on demand. Returns true once the base is available.
local function ensure_base(base)
  if sh { "git", "cat-file", "-e", base .. "^{commit}" } then
    return true
  end
  local _, err = sh { "git", "fetch", "--no-tags", "origin", base }
  if sh { "git", "cat-file", "-e", base .. "^{commit}" } then
    return true
  end
  return false, err
end

-- diff opens the PR's changes — base...HEAD, the merge-base range GitHub shows —
-- in diffview, where pending comments appear as gutter signs. load() first so a
-- restored review decorates on open. Resolving the base is one gh call, cached.
function M.diff()
  if pd_tab and vim.api.nvim_tabpage_is_valid(pd_tab) then
    vim.api.nvim_set_current_tabpage(pd_tab)
    return
  end
  local pr, err = pr_info()
  if not pr or not pr.base then
    vim.notify(err or "could not resolve PR base", vim.log.levels.ERROR)
    return
  end
  local ok_base, berr = ensure_base(pr.base)
  if not ok_base then
    vim.notify(
      "could not fetch PR base " .. pr.base:sub(1, 7) .. ": " .. (berr or ""),
      vim.log.levels.ERROR
    )
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
    load()
    decorate_diff()
  end)
  -- Existing review threads are fetched fully async (a network call), so they
  -- never gate the diff opening; their signs layer in when the request returns.
  local owner, repo = pr.slug:match "([^/]+)/(.+)"
  vim.system({
    "gh",
    "api",
    "graphql",
    "-f",
    "query=" .. THREADS_QUERY,
    "-f",
    "owner=" .. (owner or ""),
    "-f",
    "repo=" .. (repo or ""),
    "-F",
    "number=" .. pr.number,
  }, { text = true }, function(res)
    if res.code ~= 0 then
      return
    end
    vim.schedule(function()
      threads = parse_threads(res.stdout)
      decorate_diff()
    end)
  end)
end

-- Keep the diff gutter in sync with pending comments as the user navigates
-- diffview files and re-enters the view. A cleared augroup keeps re-sourcing the
-- module (dev reload) idempotent rather than stacking duplicate callbacks.
vim.api.nvim_create_autocmd("User", {
  group = vim.api.nvim_create_augroup("jmux_pr", { clear = true }),
  pattern = { "DiffviewDiffBufWinEnter", "DiffviewViewEnter" },
  callback = vim.schedule_wrap(decorate_diff),
})

return M
