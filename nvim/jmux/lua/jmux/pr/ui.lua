-- Shared UI atoms for the pr modules: the multiline prompt, the braille
-- spinner, bar/tab-title helpers, and treesitter-based syntax highlighting for
-- code spans embedded in scratch buffers.

local M = {}

-- SPINNER frames for in-flight loaders.
M.SPINNER = { "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏" }

-- spinner drives the braille loader: on_frame(frame) fires every 80ms (first
-- tick immediate, on the main loop) until the returned stop fn runs. No frames
-- fire after stop, so on_frame needs no own teardown guard.
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

-- key_hint renders an emphasized key glyph followed by a muted label for a
-- status bar; stock highlight groups so it tracks whatever colorscheme is active.
function M.key_hint(key, label)
  return ("%%#Special#%s %%#Comment#%s"):format(key, label)
end

-- bar builds a statusline-syntax string (for tabline or winbar): a Title-styled
-- label on the left and the hints flush right.
function M.bar(label, hints)
  return ("%%#Title# %s %%*%%=%s "):format(label, table.concat(hints, "  "))
end

-- set_tab_title shows a full-width jmux bar (label + hints) in the tabline, but
-- scoped to its own tabpage: the tabline is a global option, so without this it
-- would bleed onto other tabs (e.g. pv left open while you switch to pd). A
-- TabEnter watcher re-applies the bar on this tab and restores the previous
-- tabline on any other; BufWipeout tears it all down. Returns the bar string (for
-- callers that re-show it after a transient overlay) and a restore fn.
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
  -- set_hints rebuilds the bar (e.g. to reveal a key once data confirms it
  -- applies) and re-applies it if this tab is current.
  local function set_hints(new_hints)
    title = M.bar(label, new_hints)
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

-- prompt_multiline opens a floating scratch buffer for a multi-line body and
-- behaves like editing a file: :w runs on_save(text) (re-runnable to update),
-- :q closes, :q! discards, and :q on unsaved edits warns as usual. Used because
-- vim.ui.input — and snacks' own input — are single-line. An optional `initial`
-- string prefills the buffer (e.g. a suggestion block to edit); on_open(buf), if
-- given, runs once the buffer exists (used to attach live suggestion highlights).
-- footer overrides the default hint line (e.g. "save" vs "stage").
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

-- lang_of maps a file path to a treesitter language (or nil), so each code block
-- can be highlighted with the right parser regardless of the others.
function M.lang_of(path)
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

-- highlight_suggestion finds the ```suggestion fence in a comment buffer and
-- syntax-highlights its body in the reviewed file's language, so a suggestion
-- reads like code rather than plain markdown. Re-run on edit to stay live; a
-- high priority keeps it above markdown's own raw-block highlighting.
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
