-- The pv view: a tab of three panes — the conversation (description,
-- comments, reviews, inline threads) beside reviewers and checks panes fed
-- by their pure models — with refresh, reply, reactions, reviewer requests,
-- description editing, and merging. Panes are plain windows: <C-w> motions
-- move between them.

local checks = require "jmux.pr.checks"
local conversation = require "jmux.pr.conversation"
local diff = require "jmux.pr.diff"
local gh = require "jmux.pr.gh"
local merge = require "jmux.pr.merge"
local reactions = require "jmux.pr.reactions"
local reviewers = require "jmux.pr.reviewers"
local state = require "jmux.pr.state"
local ui = require "jmux.pr.ui"
local util = require "jmux.pr.util"

local nilable = util.nilable
local reltime = util.reltime

local M = {}

-- pv_tab is the `pv` view tab, tracked so re-invoking focuses the open tab
-- instead of stacking a duplicate. pv_refresh refetches into that tab, for
-- callers whose action just changed the conversation (e.g. a submitted review).
local pv_tab
local pv_refresh
-- The hint bar starts collapsed (contextual hints only); ? expands the full
-- key list. Module-owned so the preference outlives each render.
local hint_state = { hidden = true }

-- watch holds the poll timer that keeps pv fresh while checks run or an
-- auto-merge is armed; it dies with the tab or the state it waited for.
local watch = ui.slot(function(t)
  t:stop()
  t:close()
end)

-- VIEW_QUERY stitches the models' field fragments into one request: each
-- model owns the selection it reads (conversation also lends its connection
-- field lists to the pagination below); view owns only the wiring — viewer,
-- repo scalars, mine/assignee bits, and the pageInfo plumbing. The three
-- conversation connections page at GitHub's 100 cap; fetch_all chases their
-- cursors so a busy PR renders whole.
local VIEW_QUERY = ([[
query($owner:String!,$repo:String!,$number:Int!){
  viewer{ login }
  repository(owner:$owner,name:$repo){
    viewerDefaultMergeMethod
    pullRequest(number:$number){
      %s
      viewerDidAuthor assignees(first:20){ nodes{ login } }
      %s
      %s
      comments(first:100){ pageInfo{ hasNextPage endCursor } nodes{ %s } }
      reviews(first:100){ pageInfo{ hasNextPage endCursor } nodes{ %s } }
      reviewThreads(first:100){ pageInfo{ hasNextPage endCursor } nodes{ %s } }
      %s
    }
  }
}]]):format(
  conversation.PR_FIELDS,
  merge.FIELDS,
  reviewers.FIELDS,
  conversation.COMMENT_FIELDS,
  conversation.REVIEW_FIELDS,
  conversation.THREAD_FIELDS,
  checks.FIELDS
)

-- page_query fetches one more page of a single conversation connection.
local function page_query(key, fields)
  return ([[
query($owner:String!,$repo:String!,$number:Int!,$after:String!){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$number){
      %s(first:100,after:$after){ pageInfo{ hasNextPage endCursor } nodes{ %s } }
    }
  }
}]]):format(key, fields)
end

local CONNECTIONS = {
  { key = "comments", fields = conversation.COMMENT_FIELDS },
  { key = "reviews", fields = conversation.REVIEW_FIELDS },
  { key = "reviewThreads", fields = conversation.THREAD_FIELDS },
}

-- fetch_all runs VIEW_QUERY, then chases each conversation connection's
-- cursors — connections in parallel, pages within one in sequence — merging
-- every page's nodes before handing the whole response to cb. A failed tail
-- page renders what was gathered rather than failing the view.
local function fetch_all(vars, cb)
  gh.graphql(VIEW_QUERY, vars, function(data, err)
    local pr = data and vim.tbl_get(data, "data", "repository", "pullRequest")
    if type(pr) ~= "table" then
      cb(data, err)
      return
    end
    local pending = 0
    local function done()
      if pending == 0 then
        cb(data)
      end
    end
    local function page(conn, cursor)
      pending = pending + 1
      local q = page_query(conn.key, conn.fields)
      gh.graphql(q, vim.tbl_extend("force", vars, { after = cursor }), function(pd)
        pending = pending - 1
        local c = pd and vim.tbl_get(pd, "data", "repository", "pullRequest", conn.key)
        if type(c) == "table" then
          vim.list_extend(pr[conn.key].nodes, c.nodes or {})
          local pi = c.pageInfo or {}
          if pi.hasNextPage and nilable(pi.endCursor) then
            page(conn, pi.endCursor)
          end
        end
        done()
      end)
    end
    for _, conn in ipairs(CONNECTIONS) do
      local pi = vim.tbl_get(pr, conn.key, "pageInfo") or {}
      if pi.hasNextPage and nilable(pi.endCursor) then
        page(conn, pi.endCursor)
      end
    end
    done()
  end)
end

-- build_model composes the three pure models from the VIEW_QUERY response:
-- the conversation (header, description, time-ordered blocks + row-intent
-- maps) and the side panes' reviewers/checks (nil when the PR has neither).
local function build_model(data)
  local pr = vim.tbl_get(data, "data", "repository", "pullRequest")
  if type(pr) ~= "table" then
    return { header = { "  no PR data" }, blocks = {}, reactions = {}, subjects = {}, jumps = {} }
  end
  local m = conversation.model(pr)
  m.reviewers = reviewers.model(pr)
  m.checks = checks.model(pr)
  return m
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

-- request_review picks reviewers — assignable users or org teams — and adds
-- them via one `gh pr edit`. The author and anyone/any team already requested
-- are filtered out (excluded by login, excludedTeams by slug). Uses snacks'
-- picker directly rather than vim.ui.select so several reviewers can be
-- Tab-marked and confirmed together (<CR> on an unmarked item still single-picks).
-- on_done runs after success so the caller can refresh.
local function request_review(pr, excluded, excludedTeams, on_done)
  local owner = gh.owner_repo(pr.slug)
  local stop = ui.spin "fetching reviewers…"

  local function request(chosen)
    if #chosen == 0 then
      return
    end
    local values, labels = {}, {}
    for _, it in ipairs(chosen) do
      values[#values + 1] = it.value
      labels[#labels + 1] = it.label
    end
    local who = #labels > 3 and (#labels .. " reviewers") or table.concat(labels, ", ")
    ui.act("requesting " .. who .. "…", function(done)
      -- --add-reviewer takes a comma-separated list, so any number of picks is
      -- still one gh call.
      vim.system(
        { "gh", "pr", "edit", tostring(pr.number), "--repo", pr.slug, "--add-reviewer", table.concat(values, ",") },
        { text = true },
        function(r2)
          vim.schedule(function()
            done(r2.code == 0 or nil, vim.trim(r2.stderr or ""))
          end)
        end
      )
    end, function()
      vim.notify("requested review from " .. who)
      if on_done then
        on_done()
      end
    end)
  end

  local function pick(items)
    if #items == 0 then
      vim.notify("no reviewers left to request", vim.log.levels.WARN)
      return
    end
    require("snacks").picker {
      title = "request review · ⇥ marks several",
      items = items,
      format = "text",
      layout = { preset = "select" },
      confirm = function(picker)
        -- fallback keeps the single-pick flow: <CR> with nothing marked acts
        -- on the item under the cursor.
        local chosen = picker:selected { fallback = true }
        picker:close()
        request(chosen)
      end,
    }
  end

  -- users and teams fetch concurrently — neither depends on the other, so the
  -- picker's wait is the slower call, not their sum. Failures degrade to the
  -- empty list, so both always land and the picker always opens.
  local users_r, teams_r
  local function maybe_pick()
    if not (users_r and teams_r) then
      return
    end
    stop()
    -- text is what the picker shows and matches on; label is the short name
    -- used in notifications.
    local items = {}
    for _, u in ipairs(users_r) do
      if u.login and not excluded[u.login] then
        local name = nilable(u.name)
        local label = name and name ~= "" and ("%s (%s)"):format(u.login, name) or ("@" .. u.login)
        items[#items + 1] = {
          value = u.login,
          label = label,
          text = label,
          sort = u.login:lower(),
        }
      end
    end
    for _, t in ipairs(teams_r) do
      if t.slug and not excludedTeams[t.slug] then
        local label = nilable(t.name) or t.slug
        items[#items + 1] = {
          value = (owner or "") .. "/" .. t.slug,
          label = label,
          text = label .. "  [team]",
          sort = "~" .. t.slug:lower(), -- "~" sorts teams after users
        }
      end
    end
    table.sort(items, function(a, b)
      return a.sort < b.sort
    end)
    pick(items)
  end
  local _, repo = gh.owner_repo(pr.slug)
  gh.graphql(REVIEWERS_QUERY, { owner = owner, repo = repo }, function(data)
    users_r = vim.tbl_get(data or {}, "data", "repository", "assignableUsers", "nodes") or {}
    maybe_pick()
  end)
  gh.graphql(TEAMS_QUERY, { owner = owner }, function(data)
    teams_r = vim.tbl_get(data or {}, "data", "organization", "teams", "nodes") or {}
    maybe_pick()
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
  ui.prompt_multiline(("edit #%d description"):format(pr.number), function(text)
    ui.act("saving description…", function(done)
      vim.system(
        { "gh", "pr", "edit", tostring(pr.number), "--repo", pr.slug, "--body-file", "-" },
        { stdin = text, text = true },
        function(res)
          vim.schedule(function()
            done(res.code == 0 or nil, vim.trim(res.stderr or ""))
          end)
        end
      )
    end, function()
      vim.notify(("updated #%d description"):format(pr.number))
      if on_done then
        on_done()
      end
    end)
  end, body ~= "" and body or nil, nil, " :w save · :q close ")
end

-- view renders the full PR conversation — body, comments, reviews, inline review
-- threads, and checks — into a markdown tab. Fetched async (a "Loading…" stub
-- shows first) so opening is instant; q closes it.
function M.view(opts)
  if pv_tab and vim.api.nvim_tabpage_is_valid(pv_tab) then
    vim.api.nvim_set_current_tabpage(pv_tab)
    if opts and opts.refresh and pv_refresh then
      pv_refresh()
    end
    return
  end
  -- Open the tab and paint the loader first; resolving the PR and fetching the
  -- conversation both happen async, so the view shows instantly and the spinner
  -- keeps animating during any round trip.
  vim.cmd "tabnew"
  pv_tab = vim.api.nvim_get_current_tabpage()
  local win = vim.api.nvim_get_current_win()
  local buf = vim.api.nvim_get_current_buf()
  vim.bo[buf].buftype = "nofile"
  vim.bo[buf].bufhidden = "wipe"
  vim.bo[buf].filetype = "markdown"

  -- scope=local: vim.wo[win] writes the option's global value too, which
  -- would leak pv's cosmetics (numbers off, conceal on…) into every window
  -- opened for the rest of the session.
  local function scratch_opts(w, opts_tbl)
    for opt, val in pairs(opts_tbl) do
      vim.api.nvim_set_option_value(opt, val, { scope = "local", win = w })
    end
  end
  scratch_opts(win, {
    wrap = true,
    linebreak = true,
    breakindent = true,
    conceallevel = 2,
    -- concealed even on the cursor line: this is a read-only view, so
    -- revealing raw syntax on hover just makes lines jump around.
    concealcursor = "nvic",
    number = false,
    relativenumber = false,
  })

  -- The side column: reviewers over checks, plain splits so every native
  -- window motion (<C-w>hjkl, resize, close) just works. Each pane is a
  -- scratch buffer titled by its winbar and re-filled on every render.
  local function side_pane(label, cmd, width)
    vim.cmd(cmd)
    local w = vim.api.nvim_get_current_win()
    if width then
      vim.api.nvim_win_set_width(w, width)
    end
    local b = vim.api.nvim_create_buf(false, true)
    vim.api.nvim_win_set_buf(w, b)
    vim.bo[b].bufhidden = "wipe"
    scratch_opts(w, { number = false, relativenumber = false, wrap = false, winfixwidth = true })
    vim.api.nvim_set_option_value("winbar", ui.bar(label, {}), { scope = "local", win = w })
    -- q works even before the first render binds the full spec.
    vim.keymap.set("n", "q", "<cmd>tabclose<cr>", { buffer = b, nowait = true, desc = "close" })
    return { win = w, buf = b }
  end
  local panes = {
    reviewers = side_pane("reviewers", "botright vsplit", 36),
  }
  panes.checks = side_pane("checks", "belowright split")
  vim.api.nvim_set_current_win(win)

  local function fill(lines)
    vim.bo[buf].modifiable = true
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
    vim.bo[buf].modifiable = false
  end

  -- fill_pane paints a side pane from a pure model ({header, header_mark,
  -- lines, marks}), header first; nil models empty the pane. Row maps stay
  -- with the caller (reviewers' R needs one).
  local pane_ns = vim.api.nvim_create_namespace "jmux_pv_panes"
  local function fill_pane(pane, pmodel)
    if not (pane.buf and vim.api.nvim_buf_is_valid(pane.buf)) then
      return
    end
    local lines = {}
    if pmodel then
      lines[1] = pmodel.header
      vim.list_extend(lines, pmodel.lines)
    end
    vim.bo[pane.buf].modifiable = true
    vim.api.nvim_buf_set_lines(pane.buf, 0, -1, false, lines)
    vim.bo[pane.buf].modifiable = false
    vim.api.nvim_buf_clear_namespace(pane.buf, pane_ns, 0, -1)
    if not pmodel then
      return
    end
    local marks = { { rel = -1, col = 0, col_end = pmodel.header_mark.col_end, hl = pmodel.header_mark.hl } }
    vim.list_extend(marks, pmodel.marks)
    for _, m in ipairs(marks) do
      pcall(vim.api.nvim_buf_set_extmark, pane.buf, pane_ns, m.rel + 1, m.col, {
        end_col = m.col_end,
        hl_group = m.hl,
        priority = 200,
      })
    end
  end

  local _, restore, set_hints =
    ui.set_tab_title(buf, "jmux pr view", { ui.key_hint("⇥", "expand"), ui.key_hint("q", "close") })
  vim.api.nvim_create_autocmd("WinClosed", {
    pattern = tostring(win),
    once = true,
    callback = function()
      restore()
      watch.clear()
      pv_tab = nil
      pv_refresh = nil
    end,
  })
  vim.keymap.set("n", "q", "<cmd>tabclose<cr>", { buffer = buf, nowait = true, desc = "close" })

  -- Draw the first frame synchronously and force a redraw so the loader shows the
  -- instant the tab opens, rather than waiting for the first timer tick; the
  -- spinner animates it from there.
  local function draw_loader(frame)
    if not vim.api.nvim_buf_is_valid(buf) then
      return
    end
    vim.bo[buf].modifiable = true
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, { frame .. " Loading PR…" })
    vim.bo[buf].modifiable = false
  end
  draw_loader(ui.SPINNER[1])
  vim.cmd "redraw"
  local stop_loader = ui.spinner(draw_loader)

  gh.pr_info_async(function(pr, err)
    if not vim.api.nvim_buf_is_valid(buf) then
      stop_loader()
      return
    end
    if not pr then
      stop_loader()
      fill { "  failed to resolve PR: " .. (err or "") }
      return
    end
    local owner, repo = gh.owner_repo(pr.slug)

    -- the PR description starts open; everything else collapsed. Hoisted above
    -- fetch_and_render so expand/collapse state survives a merge-triggered refresh.
    local expanded = { pr = true }

    -- start_watch is defined after fetch_and_render (its tick refetches) but
    -- wired into the keys below, hence the forward declaration.
    local start_watch

    -- re-runnable so a successful merge can refresh the view in place. stop_spin,
    -- when passed, halts a merge spinner the instant the refreshed data arrives.
    local function fetch_and_render(stop_spin, cached)
      -- remember the cursor line so a refresh keeps your place across the
      -- re-render (expand state is preserved, so the line stays meaningful).
      local prev_line = (vim.api.nvim_win_is_valid(win) and vim.api.nvim_win_get_cursor(win)[1]) or 1

      -- finish renders one decoded response into the view and wires its keymaps.
      -- The render path is identical for a cached open and a live fetch — both
      -- hand over the decoded table; `age` (the cache's age in seconds, nil for a
      -- live fetch) is shown in the bar so a stale copy is obvious and signposts
      -- the <C-r> refresh.
      local function finish(data, err2, age)
        vim.schedule(function()
          stop_loader()
          if stop_spin then
            stop_spin()
          end
          if not vim.api.nvim_buf_is_valid(buf) then
            return
          end
          if not data then
            return fill { "", "  failed to load PR:", "", "  " .. (err2 or "") }
          end
          -- a live fetch (age nil) refreshes the on-disk cache; a cached open
          -- (age set) renders without rewriting what it just read.
          if not age then
            state.write_view_cache(vim.json.encode(data))
          end

          -- Collapsible tree: header → PR description → checks → conversation. Each
          -- item shows a ▸/▾ header; expanding a review reveals its body and inline
          -- threads, which expand in turn. Re-rendered on toggle; check-icon marks are
          -- repositioned since the description above them changes height.
          local model = build_model(data)
          local ns = vim.api.nvim_create_namespace "jmux_pr_view"
          local rows_by_id, id_by_row, jump_by_row, reviewer_by_row, subject_by_row = {}, {}, {}, {}, {}

          local function render()
            local lines = vim.list_extend({}, model.header)
            rows_by_id, id_by_row, jump_by_row, reviewer_by_row, subject_by_row = {}, {}, {}, {}, {}
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
                -- an expanded thread's first body line is its `path:line`,
                -- <CR>'s jump target.
                if model.jumps[b.id] then
                  jump_by_row[row + 1] = b.id
                end
                -- body rows react to their own message: a thread's per-line
                -- subjects (replies differ from the root), else the block's.
                local bsubj = model.subjects[b.id]
                for i = 1, #b.body do
                  local s = (b.line_subjects and b.line_subjects[i]) or bsubj
                  if s then
                    subject_by_row[row + i] = s
                  end
                end
                add_body(b.body, "  ", b.body_marks)
                for _, k in ipairs(b.kids) do
                  local krow = #lines
                  rows_by_id[k.id], id_by_row[krow] = krow, k.id
                  lines[#lines + 1] = "  " .. (expanded[k.id] and "▾ " or "▸ ") .. k.header
                  if expanded[k.id] then
                    if model.jumps[k.id] then
                      jump_by_row[krow + 1] = k.id
                    end
                    for i = 1, #k.lines do
                      local s = k.line_subjects and k.line_subjects[i]
                      if s then
                        subject_by_row[krow + i] = s
                      end
                    end
                    -- three spaces, not four — a 4-space indent after a blank
                    -- line is an indented code block to markdown.
                    add_body(k.lines, "   ", k.line_marks)
                  end
                end
              end
              lines[#lines + 1] = ""
            end

            if model.pr then
              emit(model.pr)
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

            -- the side panes re-fill from their models on every render, and
            -- the reviewers rows map for R (header on row 0, entry i on row i).
            fill_pane(panes.reviewers, model.reviewers)
            fill_pane(panes.checks, model.checks)
            reviewer_by_row = {}
            for i, e in ipairs(model.reviewers and model.reviewers.entries or {}) do
              reviewer_by_row[i] = e
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

          -- <CR> on a thread's `path:line` opens the diff there; elsewhere it
          -- toggles like <Tab>.
          local function toggle_or_jump()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= buf then
              return
            end
            local id = jump_by_row[vim.api.nvim_win_get_cursor(w)[1] - 1]
            local j = id and model.jumps[id]
            if j then
              diff.open_at(j.path, j.line)
              return
            end
            toggle()
          end

          pv_refresh = function()
            fetch_and_render(ui.spin "refreshing…")
          end

          -- who_reacted shows the reactor hover. Logins aren't in the main
          -- query (node limit), so they fetch on demand, memoized for the
          -- render's lifetime; the hover only pops if the cursor hasn't moved.
          local function who_reacted()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= buf then
              return
            end
            local row = vim.api.nvim_win_get_cursor(w)[1] - 1
            -- a message row hovers that message's reactors; a header row the
            -- block's (a thread header aggregates the whole thread).
            local subj, id = subject_by_row[row], id_by_row[row]
            local entry = (subj and model.reactions[subj.id]) or (id and model.reactions[id])
            if not entry then
              return
            end
            if entry.lines then
              ui.hover(entry.lines, "reactions")
              return
            end
            local ids = {}
            for _, it in ipairs(entry.items) do
              ids[#ids + 1] = it.id
            end
            ui.act("fetching reactions…", function(done)
              gh.graphql(reactions.REACTORS_QUERY, { ids = ids }, done)
            end, function(rdata)
              local by_id = {}
              for _, n in ipairs(vim.tbl_get(rdata, "data", "nodes") or {}) do
                if type(n) == "table" and n.id then
                  by_id[n.id] = reactions.who(n)
                end
              end
              local lines = {}
              for _, it in ipairs(entry.items) do
                local rw = by_id[it.id]
                if rw then
                  if it.prefix then
                    lines[#lines + 1] = it.prefix
                  end
                  vim.list_extend(lines, rw)
                end
              end
              if #lines == 0 then
                return
              end
              entry.lines = lines
              local w2 = vim.api.nvim_get_current_win()
              if vim.api.nvim_win_get_buf(w2) == buf and vim.api.nvim_win_get_cursor(w2)[1] - 1 == row then
                ui.hover(entry.lines, "reactions")
              end
            end)
          end

          -- + reacts via the emoji picker: every reaction is a toggle (the
          -- picker marks the ones already yours). `+` echoes GitHub's
          -- smiley-plus button.
          -- body rows resolve to their own message's subject; header rows to
          -- the block/thread's (a thread header reacts to its root comment).
          local function subject_under_cursor()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= buf then
              return nil
            end
            local row = vim.api.nvim_win_get_cursor(w)[1] - 1
            if subject_by_row[row] then
              return subject_by_row[row]
            end
            local id = id_by_row[row]
            return id and model.subjects[id] or nil
          end
          local function toggle_reaction(subject, content)
            local given = subject.mine[content] or false
            ui.act(given and "removing reaction…" or "reacting…", function(done)
              gh.graphql(
                given and reactions.REMOVE_MUTATION or reactions.ADD_MUTATION,
                { subject = subject.id, content = content },
                done
              )
            end, function()
              fetch_and_render(ui.spin "refreshing…")
            end)
          end
          local function react_pick()
            local subject = subject_under_cursor()
            if not subject then
              return
            end
            local choices = {}
            for _, content in ipairs(reactions.ORDER) do
              choices[#choices + 1] = { content = content, mine = subject.mine[content] or false }
            end
            vim.ui.select(choices, {
              prompt = "react",
              format_item = function(it)
                local label = ("%s %s"):format(reactions.EMOJI[it.content], it.content:lower():gsub("_", " "))
                return it.mine and (label .. " · remove") or label
              end,
            }, function(choice)
              if choice then
                toggle_reaction(subject, choice.content)
              end
            end)
          end

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
          -- threads keyed by id: the row resolvers run per cursor move for
          -- the contextual hints, so no linear scans there.
          local thread_by_id = {}
          for _, t in ipairs(vim.tbl_get(prnode or {}, "reviewThreads", "nodes") or {}) do
            if t.id then
              thread_by_id[t.id] = t
            end
          end

          -- cursor_row is the 0-indexed buffer row under the cursor, nil when
          -- focus is elsewhere; the row resolvers below key off it.
          local function cursor_row()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= buf then
              return nil
            end
            return vim.api.nvim_win_get_cursor(w)[1] - 1
          end

          -- thread_under_cursor resolves the cursor row to its reviewThreads
          -- node (threads are the kids whose id is the GraphQL thread id).
          local function thread_under_cursor()
            local row = cursor_row()
            local id = row and id_by_row[row]
            return id and thread_by_id[id] or nil
          end

          -- replies post immediately, then the view refreshes.
          local function reply()
            local t = thread_under_cursor()
            if not t then
              return
            end
            local root = vim.tbl_get(t, "comments", "nodes", 1) or {}
            diff.reply_to_thread({
              id = t.id,
              path = t.path,
              line = nilable(t.line),
              comments = { { login = vim.tbl_get(root, "author", "login") or "?" } },
            }, function()
              fetch_and_render(ui.spin "refreshing…")
            end)
          end

          -- reviewer_under_cursor resolves a reviewers-pane row to its entry
          -- (row 0 is the pane's summary header).
          local function reviewer_under_cursor()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= panes.reviewers.buf then
              return nil
            end
            return reviewer_by_row[vim.api.nvim_win_get_cursor(w)[1] - 1]
          end

          -- R on a reviewer's line re-requests that reviewer directly;
          -- anywhere else it opens the request picker. Awaiting rows no-op —
          -- GitHub already lists them as requested.
          local function request()
            local e = reviewer_under_cursor()
            if not e then
              request_review(pr, excluded, excludedTeams, function()
                fetch_and_render(ui.spin "refreshing…")
              end)
              return
            end
            if e.state == "AWAITING" then
              return vim.notify(e.name .. " is already awaiting review")
            end
            local owner = gh.owner_repo(pr.slug)
            local value = e.login or (e.slug and owner .. "/" .. e.slug)
            if not value then
              return
            end
            ui.act("re-requesting " .. e.name .. "…", function(done)
              vim.system(
                { "gh", "pr", "edit", tostring(pr.number), "--repo", pr.slug, "--add-reviewer", value },
                { text = true },
                function(res)
                  vim.schedule(function()
                    done(res.code == 0 or nil, vim.trim(res.stderr or ""))
                  end)
                end
              )
            end, function()
              vim.notify("re-requested review from " .. e.name)
              fetch_and_render(ui.spin "refreshing…")
            end)
          end

          -- x toggles the thread's resolved state.
          local function resolve()
            local t = thread_under_cursor()
            if not t then
              return
            end
            diff.resolve_thread({ id = t.id, resolved = t.isResolved == true }, function()
              fetch_and_render(ui.spin "refreshing…")
            end)
          end

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

          -- merging needs an open PR that's yours and a merge model with a safe
          -- gh flag; ready merges now, anything else arms auto-merge.
          local mst = merge.model(prnode, nilable(vim.tbl_get(data, "data", "repository", "viewerDefaultMergeMethod")))
          local how, flag, ready, armed = mst.how, mst.flag, mst.ready, mst.armed
          local pr_state = tostring(nilable(prnode.state) or ""):upper()
          local mergeable = mine and flag and pr_state == "OPEN"

          -- gh_merge runs `gh pr merge` with extra flags, confirming first.
          local function gh_merge(prompt, label, extra, on_ok)
            ui.confirm(prompt, function()
              ui.act(label, function(done)
                local cmd = { "gh", "pr", "merge", "--repo", pr.slug, flag }
                vim.list_extend(cmd, extra)
                cmd[#cmd + 1] = "--"
                cmd[#cmd + 1] = tostring(pr.number)
                vim.system(cmd, { text = true }, function(res)
                  vim.schedule(function()
                    done(res.code == 0 or nil, vim.trim(res.stderr or ""))
                  end)
                end)
              end, on_ok)
            end)
          end

          -- admin bypasses branch protection (failing checks, unresolved threads).
          local function do_merge(admin)
            local label = admin and (how .. ", admin") or how
            gh_merge(("merge #%d via %s?"):format(pr.number, label), ("merging #%d…"):format(pr.number), {
              admin and "--admin" or nil,
            }, function()
              vim.notify(("merged #%d (%s)"):format(pr.number, label))
              util.prompt_remove_workspace(function()
                fetch_and_render(ui.spin "refreshing…")
              end)
            end)
          end

          -- arm_auto enables GitHub's server-side auto-merge, then watches so
          -- the merge is noticed and the workspace prompt still fires.
          local function arm_auto()
            gh_merge(
              ("auto-merge #%d via %s when ready?"):format(pr.number, how),
              ("arming auto-merge #%d…"):format(pr.number),
              { "--auto" },
              function()
                vim.notify(("auto-merge armed for #%d"):format(pr.number))
                start_watch()
                fetch_and_render(ui.spin "refreshing…")
              end
            )
          end

          -- What the watcher was waiting for, evaluated on every render: a
          -- merge (armed or manual elsewhere) fires the same prompt as m; a
          -- watch without auto-merge stops when the checks settle.
          if watch.value then
            local running = model.checks and model.checks.running or 0
            local failed = model.checks and model.checks.failed or 0
            if pr_state == "MERGED" then
              watch.clear()
              vim.notify(("merged #%d"):format(pr.number))
              util.prompt_remove_workspace(function() end)
            elseif pr_state ~= "OPEN" then
              watch.clear()
            elseif not armed and running == 0 then
              watch.clear()
              if failed > 0 then
                vim.notify(("checks failed (%d)"):format(failed), vim.log.levels.ERROR)
              else
                vim.notify "checks passed"
              end
            end
          elseif armed and pr_state == "OPEN" then
            -- an armed PR (from a previous session or the web) is worth
            -- watching as soon as pv sees it.
            start_watch()
          end

          -- the view's keys: spec order is bar order, `when` gates the ones
          -- that need the PR to be yours (and open, for merging), and `ctx`
          -- marks the ones the collapsed bar shows for the cursor's row.
          local keys = ui.keyspec {
            {
              lhs = "m",
              -- merge when GitHub says ready, otherwise arm auto-merge.
              rhs = function()
                if ready then
                  do_merge(false)
                elseif armed then
                  vim.notify "auto-merge is already armed"
                else
                  arm_auto()
                end
              end,
              hint = function()
                return ready and "merge" or "auto-merge"
              end,
              when = function()
                return mergeable
              end,
              desc = "merge / arm auto-merge",
            },
            {
              lhs = "M",
              rhs = function()
                do_merge(true)
              end,
              hint = "merge (admin)",
              when = function()
                return mergeable
              end,
              desc = "merge (admin)",
            },
            { lhs = "<C-r>", rhs = pv_refresh, hint = "refresh", desc = "refresh" },
            {
              lhs = "R",
              rhs = request,
              hint = "request",
              desc = "request / re-request review",
              ctx = function()
                local e = reviewer_under_cursor()
                return e and e.state ~= "AWAITING" and "re-request"
              end,
            },
            {
              lhs = "r",
              rhs = reply,
              pane = "conversation",
              hint = "reply",
              desc = "reply to thread",
              ctx = function()
                return thread_under_cursor() ~= nil
              end,
            },
            {
              lhs = "x",
              rhs = resolve,
              pane = "conversation",
              hint = "resolve",
              desc = "resolve/unresolve thread",
              ctx = function()
                return thread_under_cursor() ~= nil
              end,
            },
            { lhs = "d", rhs = diff.diff, hint = "diff", desc = "open diff" },
            {
              lhs = "+",
              rhs = react_pick,
              pane = "conversation",
              hint = "react",
              desc = "react",
              ctx = function()
                return subject_under_cursor() ~= nil
              end,
            },
            {
              lhs = "K",
              rhs = who_reacted,
              pane = "conversation",
              hint = "reactions",
              desc = "who reacted",
              ctx = function()
                local row = cursor_row()
                if not row then
                  return false
                end
                local subj, id = subject_by_row[row], id_by_row[row]
                return (subj and model.reactions[subj.id] or id and model.reactions[id]) ~= nil
              end,
            },
            {
              lhs = "E",
              rhs = function()
                edit_description(pr, nilable(prnode and prnode.body) or "", function()
                  fetch_and_render(ui.spin "refreshing…")
                end)
              end,
              hint = "edit",
              when = function()
                return mine
              end,
              desc = "edit description",
            },
            {
              lhs = "<Tab>",
              rhs = toggle,
              lhs_hint = "⇥",
              hint = "expand",
              desc = "toggle thread",
              pane = "conversation",
              ctx = function()
                local row = cursor_row()
                return (row and id_by_row[row]) ~= nil
              end,
            },
            {
              lhs = "<CR>",
              rhs = toggle_or_jump,
              lhs_hint = "⏎",
              desc = "toggle / open in diff",
              pane = "conversation",
              ctx = function()
                local row = cursor_row()
                return (row and jump_by_row[row]) and "diff"
              end,
            },
            { lhs = "go", rhs = M.browser, desc = "open in browser" },
            { lhs = "q", rhs = "<cmd>tabclose<cr>", hint = "close", desc = "close" },
          }

          -- a cached paint flags its age so it's clear the data may be stale
          -- and <C-r> will refetch; a live fetch (age nil) shows no marker.
          local bar = ui.hintbar {
            keys = keys,
            set_hints = set_hints,
            state = hint_state,
            extra = function()
              return age and { ("%%#Comment#cached %s ago"):format(util.short_age(age)) } or {}
            end,
          }
          -- conversation gets the full spec; side panes only the global keys
          -- (pane = nil). The bar tracks the cursor in all three.
          keys.apply(buf)
          local function global_only(k)
            return k.pane == nil
          end
          for _, pane in pairs(panes) do
            if vim.api.nvim_buf_is_valid(pane.buf) then
              keys.apply(pane.buf, nil, global_only)
              bar.attach(pane.buf)
            end
          end
          bar.attach(buf)
          bar.refresh()

          pcall(vim.api.nvim_win_set_cursor, win, { math.min(prev_line, vim.api.nvim_buf_line_count(buf)), 0 })
        end)
      end

      if cached then
        finish(cached.data, nil, cached.age)
      else
        fetch_all({ owner = owner, repo = repo, number = pr.number }, finish)
      end
    end

    -- The watcher refetches every 15s while the tab lives; each render then
    -- decides whether the awaited state arrived (see finish). Silent — no
    -- spinner per tick, the bar would flicker.
    start_watch = function()
      if watch.value then
        return
      end
      local t = vim.uv.new_timer()
      t:start(
        15000,
        15000,
        vim.schedule_wrap(function()
          if pv_tab and vim.api.nvim_tabpage_is_valid(pv_tab) then
            fetch_and_render()
          else
            watch.clear()
          end
        end)
      )
      watch.set(t)
    end

    -- Paint from the on-disk cache when present so the view opens with no network
    -- round trip; <C-r> refetches and rewrites it. Fall through to a live fetch
    -- only when there's no cache yet.
    local cached_data, cached_age = state.read_view_cache()
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

return M
