-- Shared UI atoms for the pr modules.

local M = {}

M.SPINNER = { "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏" }

-- spinner fires on_frame every 80ms until the returned stop fn runs; no
-- frame fires after stop.
function M.spinner(on_frame)
  local timer, i = vim.uv.new_timer(), 0
  local function stop()
    if timer then
      timer:stop()
      timer:close()
      timer = nil
    end
  end
  timer:start(
    0,
    80,
    vim.schedule_wrap(function()
      if not timer then
        return
      end
      i = i % #M.SPINNER + 1
      on_frame(M.SPINNER[i])
    end)
  )
  return stop
end

-- key_hint renders an emphasised key followed by a muted label.
function M.key_hint(key, label)
  return ("%%#Special#%s %%#Comment#%s"):format(key, label)
end

-- bar builds a statusline-syntax string: label left, hints right.
function M.bar(label, hints)
  return ("%%#Title# %s %%*%%=%s "):format(label, table.concat(hints, "  "))
end

-- bars maps a tabpage to its title bar's spinner, so spin() finds it without
-- callers threading one.
local bars = {}
local function prune_bars()
  for t in pairs(bars) do
    if not vim.api.nvim_tabpage_is_valid(t) then
      bars[t] = nil
    end
  end
end

-- set_tab_title shows a jmux bar in the tabline scoped to this tabpage: the
-- tabline is a global option, so a TabEnter watcher re-applies it here and
-- restores the previous tabline elsewhere.
function M.set_tab_title(buf, label, hints)
  local saved_tabline, saved_showtabline = vim.o.tabline, vim.o.showtabline
  local title = M.bar(label, hints)
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
  local function apply(hs)
    local new = M.bar(label, hs)
    -- an unchanged bar skips the write so cursor-move refreshes don't redraw.
    if new == title and vim.o.tabline == new then
      return
    end
    title = new
    refresh()
  end
  -- while a spin animates, set_hints only records the hints; applying them
  -- would fight the frames.
  local current_hints = hints
  local spinning = false
  local function set_hints(new_hints)
    current_hints = new_hints
    if not spinning then
      apply(new_hints)
    end
  end
  -- tab-scoped, unlike M.spin, so frames never bleed onto other tabs.
  local function spin(spin_label)
    spinning = true
    local stop = M.spinner(function(frame)
      apply { ("%%#Title#%s %s"):format(frame, spin_label) }
    end)
    return function()
      stop()
      spinning = false
      apply(current_hints)
    end
  end
  refresh()
  prune_bars()
  bars[tab] = spin
  vim.api.nvim_create_autocmd("TabEnter", { group = group, callback = refresh })
  vim.api.nvim_create_autocmd("BufWipeout", {
    group = group,
    buffer = buf,
    once = true,
    callback = function()
      -- a replacement bar on the same tab may already own the slot.
      if bars[tab] == spin then
        bars[tab] = nil
      end
      restore()
      pcall(vim.api.nvim_del_augroup_by_id, group)
    end,
  })
  return title, restore, set_hints, spin
end

-- prompt_multiline opens a floating buffer that behaves like a file: :w runs
-- on_save(text), :q closes, :q! discards. vim.ui.input is single-line.
function M.prompt_multiline(title, on_save, initial, on_open, footer)
  require("snacks").win {
    width = 0.6,
    height = 0.35,
    border = "rounded",
    title = " " .. title .. " ",
    footer = footer or " :w stage · :q close ",
    -- acwrite (not nofile) so :w fires BufWriteCmd instead of erroring.
    bo = { filetype = "markdown", buftype = "acwrite", bufhidden = "wipe" },
    wo = { wrap = true },
    -- snacks' default q=close would silently discard the comment.
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
  if not initial then
    vim.cmd "startinsert"
  end
end

-- keyspec declares a buffer's keys once: apply registers the active entries
-- and deletes those whose `when` turned false; hints renders the bar labels.
function M.keyspec(entries)
  local ks = { entries = entries }
  function ks.apply(buf, map, filter)
    map = map or vim.keymap.set
    for _, k in ipairs(entries) do
      if not filter or filter(k) then
        local mode = k.mode or "n"
        if not k.when or k.when() then
          map(mode, k.lhs, k.rhs, { buffer = buf, nowait = true, desc = k.desc })
        else
          pcall(vim.keymap.del, mode, k.lhs, { buffer = buf })
        end
      end
    end
  end
  function ks.hints()
    local out = {}
    for _, k in ipairs(entries) do
      if k.hint and (not k.when or k.when()) then
        out[#out + 1] = M.key_hint(k.lhs_hint or k.lhs, type(k.hint) == "function" and k.hint() or k.hint)
      end
    end
    return out
  end
  -- ctx_hints renders entries whose ctx applies at the cursor; a string ctx
  -- overrides the label.
  function ks.ctx_hints()
    local out = {}
    for _, k in ipairs(entries) do
      if k.ctx and (not k.when or k.when()) then
        local r = k.ctx()
        if r then
          local label = type(r) == "string" and r or k.hint
          if type(label) == "function" then
            label = label()
          end
          if label then
            out[#out + 1] = M.key_hint(k.lhs_hint or k.lhs, label)
          end
        end
      end
    end
    return out
  end
  return ks
end

-- disposer collects cleanup functions and runs them once, in reverse order.
function M.disposer()
  local fns = {}
  local d = {}
  function d.add(fn)
    fns[#fns + 1] = fn
  end
  function d.run()
    for i = #fns, 1, -1 do
      pcall(fns[i])
    end
    fns = {}
  end
  return d
end

local PANE_NS = vim.api.nvim_create_namespace "jmux_pane"

-- pane creates a scratch window jmux owns: a split (o.split, whole-tab with
-- o.toplevel), a new tab at the end (o.tab), or the current window adopted.
-- Window options are set scope-local because vim.wo writes the global value
-- too. on_close runs once on either death signal, since bufhidden=wipe kills
-- the pane with no WinClosed.
function M.pane(o)
  o = o or {}
  local win, buf
  if o.split then
    buf = vim.api.nvim_create_buf(false, true)
    win = vim.api.nvim_open_win(buf, true, { split = o.split, win = o.toplevel and -1 or 0 })
  elseif o.tab then
    buf = vim.api.nvim_create_buf(false, true)
    vim.api.nvim_open_tabpage(buf, true, { after = #vim.api.nvim_list_tabpages() })
    win = vim.api.nvim_get_current_win()
  else
    win = vim.api.nvim_get_current_win()
    buf = vim.api.nvim_get_current_buf()
  end
  vim.bo[buf].buftype = "nofile"
  vim.bo[buf].bufhidden = "wipe"
  for k, v in pairs(o.bo or {}) do
    vim.bo[buf][k] = v
  end
  local wo = vim.tbl_extend("force", { number = false, relativenumber = false, winfixbuf = true }, o.wo or {})
  if o.width then
    wo.winfixwidth = true
  end
  for k, v in pairs(wo) do
    vim.api.nvim_set_option_value(k, v, { scope = "local", win = win })
  end

  local p = { win = win, buf = buf, label = o.label, on_close = M.disposer() }

  local function set_winbar(wb)
    if vim.api.nvim_win_is_valid(win) then
      vim.api.nvim_set_option_value("winbar", wb, { scope = "local", win = win })
    end
  end
  if o.label then
    set_winbar(M.bar(o.label, {}))
  end

  if o.width then
    vim.api.nvim_win_set_width(win, o.width())
    local group = vim.api.nvim_create_augroup("jmux_pane_width_" .. win, { clear = true })
    -- UIEnter too: columns is still the 80 default during a mid-startup open.
    vim.api.nvim_create_autocmd({ "UIEnter", "VimResized" }, {
      group = group,
      callback = function()
        if vim.api.nvim_win_is_valid(win) then
          vim.api.nvim_win_set_width(win, o.width())
        end
      end,
    })
    p.on_close.add(function()
      vim.api.nvim_clear_autocmds { group = group }
    end)
  end

  local torn = false
  local function teardown()
    if torn then
      return
    end
    torn = true
    p.on_close.run()
  end
  vim.api.nvim_create_autocmd("WinClosed", {
    pattern = tostring(win),
    once = true,
    callback = vim.schedule_wrap(teardown),
  })
  vim.api.nvim_create_autocmd("BufWipeout", {
    buffer = buf,
    once = true,
    callback = vim.schedule_wrap(teardown),
  })

  function p.close()
    if vim.api.nvim_win_is_valid(win) then
      pcall(vim.api.nvim_win_close, win, true)
    end
  end

  -- fill paints a pane model into the pane; nil empties it.
  function p.fill(pmodel)
    if not vim.api.nvim_buf_is_valid(buf) then
      return
    end
    local lines = pmodel and pmodel.lines or {}
    vim.bo[buf].modifiable = true
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
    vim.bo[buf].modifiable = false
    vim.api.nvim_buf_clear_namespace(buf, PANE_NS, 0, -1)
    for _, m in ipairs(pmodel and pmodel.marks or {}) do
      pcall(vim.api.nvim_buf_set_extmark, buf, PANE_NS, m.rel, m.col, {
        end_col = m.col_end,
        hl_group = m.hl,
        priority = 200,
      })
    end
    local wb = o.label and M.bar(o.label, {}) or ""
    if pmodel then
      -- % escaped so header text can never expand as statusline items.
      local function esc(s)
        return (s:gsub("%%", "%%%%"))
      end
      local icon = pmodel.header:sub(1, pmodel.header_mark.col_end)
      local rest = pmodel.header:sub(pmodel.header_mark.col_end + 1)
      wb = ("%%#%s# %s%%* %s "):format(pmodel.header_mark.hl, esc(icon), esc(vim.trim(rest)))
    end
    set_winbar(wb)
  end

  return p
end

-- panegroup ties pane lifetimes together: the owner's death closes the rest,
-- or without an owner any member's death does.
function M.panegroup(o)
  local function close_others(dead)
    for _, p in ipairs(o.panes) do
      if p ~= dead then
        p.close()
      end
    end
  end
  if o.owner then
    o.owner.on_close.add(function()
      close_others(o.owner)
    end)
  else
    for _, p in ipairs(o.panes) do
      p.on_close.add(function()
        close_others(p)
      end)
    end
  end
  return {
    close = function()
      close_others(nil)
    end,
  }
end

-- hintbar drives a set_hints bar from a keyspec: collapsed shows `? keys`
-- plus contextual hints, ? flips to the full list. It injects the ? key into
-- the spec, so apply the spec after constructing the bar.
function M.hintbar(o)
  local bar = {}
  function bar.refresh()
    local out
    if o.state.hidden then
      out = { M.key_hint("?", "keys") }
      vim.list_extend(out, o.keys.ctx_hints())
    else
      out = o.keys.hints()
    end
    if o.extra then
      vim.list_extend(out, o.extra())
    end
    o.set_hints(out)
  end
  table.insert(o.keys.entries, {
    lhs = "?",
    rhs = function()
      o.state.hidden = not o.state.hidden
      bar.refresh()
    end,
    desc = "toggle key hints",
  })
  function bar.attach(buf)
    vim.api.nvim_create_autocmd("CursorMoved", {
      group = vim.api.nvim_create_augroup("jmux_hintbar_" .. buf, { clear = true }),
      buffer = buf,
      callback = function()
        if o.state.hidden then
          bar.refresh()
        end
      end,
    })
  end
  return bar
end

-- spin runs the loader in the current tab's title bar when it has one, else
-- as a plain tabline animation.
function M.spin(label)
  prune_bars()
  local bar = bars[vim.api.nvim_get_current_tabpage()]
  if bar then
    return bar(label)
  end
  local saved_tabline, saved_show = vim.o.tabline, vim.o.showtabline
  local stop = M.spinner(function(frame)
    vim.o.tabline = ("%%=%%#Title# %s %s %%*%%="):format(frame, label)
    vim.o.showtabline = 2
  end)
  return function()
    stop()
    vim.o.tabline, vim.o.showtabline = saved_tabline, saved_show
  end
end

-- act runs op(done) behind the spin loader; done(nil, err) notifies the
-- error, anything else reaches on_ok.
function M.act(label, op, on_ok)
  local stop = M.spin(label)
  op(function(result, err)
    stop()
    if result == nil then
      vim.notify(("%s: %s"):format(label, err or ""), vim.log.levels.ERROR)
      return
    end
    if on_ok then
      on_ok(result)
    end
  end)
end

-- confirm asks a no/yes question, no first so a reflexive <CR> is safe.
function M.confirm(prompt, on_yes, on_no)
  require("snacks").picker.select({ "no", "yes" }, { prompt = prompt }, function(choice)
    if choice == "yes" then
      on_yes()
    elseif on_no then
      on_no()
    end
  end)
end

-- lease tracks keymaps and extmark namespaces applied to buffers jmux doesn't
-- own, so one revoke() returns them all to their prior state.
function M.lease()
  local keys, spaces = {}, {}
  local l = {}
  function l.map(mode, lhs, rhs, opts)
    vim.keymap.set(mode, lhs, rhs, opts)
    keys[tostring(opts.buffer) .. "\0" .. lhs] = { mode = mode, lhs = lhs, buf = opts.buffer }
  end
  function l.ns(buf, ns)
    spaces[buf .. "\0" .. ns] = { buf = buf, ns = ns }
  end
  function l.revoke()
    for _, k in pairs(keys) do
      pcall(vim.keymap.del, k.mode, k.lhs, { buffer = k.buf })
    end
    for _, s in pairs(spaces) do
      if vim.api.nvim_buf_is_valid(s.buf) then
        vim.api.nvim_buf_clear_namespace(s.buf, s.ns, 0, -1)
      end
    end
    keys, spaces = {}, {}
  end
  return l
end

-- slot holds one replaceable resource: set closes the current occupant
-- through closer first, clear closes and empties.
function M.slot(closer)
  local s = {}
  function s.clear()
    if s.value ~= nil then
      closer(s.value)
      s.value = nil
    end
  end
  function s.set(v)
    s.clear()
    s.value = v
  end
  return s
end

-- title_slot holds one tab-title bar; set restores the previous first so the
-- new bar snapshots a clean tabline.
function M.title_slot()
  local s = M.slot(function(restore)
    restore()
  end)
  local slot = { clear = s.clear }
  function slot.set(buf, label, hints)
    s.clear()
    local _, restore = M.set_tab_title(buf, label, hints)
    s.value = restore
  end
  return slot
end

local hover_win = M.slot(function(w)
  w:close()
end)

-- hover shows lines in an unfocused float below the cursor, closed on cursor
-- movement or leaving the buffer.
function M.hover(lines, title)
  local src = vim.api.nvim_get_current_buf()
  local width = title and (vim.fn.strdisplaywidth(title) + 2) or 1
  for _, l in ipairs(lines) do
    width = math.max(width, vim.fn.strdisplaywidth(l))
  end
  hover_win.set(require("snacks").win {
    relative = "cursor",
    row = 1,
    col = 0,
    width = width,
    height = #lines,
    border = "rounded",
    title = title and (" " .. title .. " ") or nil,
    enter = false,
    focusable = false,
    backdrop = false,
    text = lines,
    bo = { bufhidden = "wipe" },
  })
  vim.api.nvim_create_autocmd({ "CursorMoved", "BufLeave" }, {
    buffer = src,
    once = true,
    callback = hover_win.clear,
  })
end

-- lang_of maps a file path to a treesitter language, or nil.
function M.lang_of(path)
  local ft = vim.filetype.match { filename = path }
  if not ft then
    return nil
  end
  return vim.treesitter.language.get_lang(ft) or ft
end

-- ts_highlight lays lang's syntax highlights for text onto buf from row base;
-- a missing parser or query leaves the lines plain.
function M.ts_highlight(buf, ns, lang, text, base, priority)
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

-- highlight_suggestion highlights a ```suggestion fence body in the reviewed
-- file's language; priority 200 keeps it above markdown's raw-block highlight.
local SUGGEST_NS = vim.api.nvim_create_namespace "jmux_pr_suggestion"
function M.highlight_suggestion(buf, lang)
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
  M.ts_highlight(buf, SUGGEST_NS, lang, table.concat(code, "\n"), open, 200)
end

return M
