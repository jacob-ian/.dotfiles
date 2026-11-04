-- The pv view: a tab with the conversation beside reviewers and checks panes.

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

local M = {}

local pv_tab
local pv_refresh
-- module-owned so the ? preference outlives each render.
local hint_state = { hidden = true }

-- watch is the poll timer that runs while checks run or an auto-merge is armed.
local watch = ui.slot(function(t)
  t:stop()
  t:close()
end)

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

-- fetch_all runs VIEW_QUERY then pages each connection past GitHub's 100 cap;
-- a failed tail page renders what was gathered.
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

local REVIEWERS_QUERY = [[
query($owner:String!,$repo:String!){
  repository(owner:$owner,name:$repo){
    assignableUsers(first:100){ nodes{ login name } }
  }
}]]

-- Best-effort: a user-owned repo has no organization, so this can fail and
-- the picker then offers only users.
local TEAMS_QUERY = [[
query($owner:String!){
  organization(login:$owner){ teams(first:100){ nodes{ slug name } } }
}]]

-- request_review picks users or teams and requests them in one gh call. It
-- uses snacks' picker directly because vim.ui.select can't multi-select.
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
        local chosen = picker:selected { fallback = true }
        picker:close()
        request(chosen)
      end,
    }
  end

  local users_r, teams_r
  local function maybe_pick()
    if not (users_r and teams_r) then
      return
    end
    stop()
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

-- edit_description edits the PR body in the multiline prompt and saves it on
-- :w. An empty buffer never saves, so the description can't be cleared here.
local function edit_description(pr, body, on_done)
  -- GitHub returns the body with CRLF endings.
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

-- edit_title prompts for a new title; cancel, empty or unchanged is a no-op.
local function edit_title(pr, current, on_done)
  vim.ui.input({ prompt = "title> ", default = current }, function(text)
    if text == nil or vim.trim(text) == "" or text == current then
      return
    end
    ui.act("saving title…", function(done)
      vim.system(
        { "gh", "pr", "edit", tostring(pr.number), "--repo", pr.slug, "--title", text },
        { text = true },
        function(res)
          vim.schedule(function()
            done(res.code == 0 or nil, vim.trim(res.stderr or ""))
          end)
        end
      )
    end, function()
      vim.notify(("updated #%d title"):format(pr.number))
      if on_done then
        on_done()
      end
    end)
  end)
end

-- view opens (or focuses) the PR conversation tab.
function M.view(opts)
  if pv_tab and vim.api.nvim_tabpage_is_valid(pv_tab) then
    vim.api.nvim_set_current_tabpage(pv_tab)
    if opts and opts.refresh and pv_refresh then
      pv_refresh()
    end
    return
  end
  local conv = ui.pane {
    tab = true,
    bo = { filetype = "markdown" },
    wo = {
      wrap = true,
      linebreak = true,
      breakindent = true,
      conceallevel = 2,
      -- read-only view, so revealing raw syntax on the cursor line only jumps.
      concealcursor = "nvic",
    },
  }
  local win, buf = conv.win, conv.buf
  pv_tab = vim.api.nvim_win_get_tabpage(win)

  local function side_width()
    return math.min(42, math.floor(vim.o.columns / 3))
  end
  local panes = {
    reviewers = ui.pane {
      split = "right",
      toplevel = true,
      label = "reviewers",
      width = side_width,
      wo = { wrap = false },
    },
  }
  panes.checks = ui.pane { split = "below", label = "checks", wo = { wrap = false } }
  ui.panegroup { owner = conv, panes = { conv, panes.reviewers, panes.checks } }
  for _, pane in pairs(panes) do
    vim.keymap.set("n", "q", "<cmd>tabclose<cr>", { buffer = pane.buf, nowait = true, desc = "close" })
  end
  vim.api.nvim_set_current_win(win)

  local function fill(lines)
    vim.bo[buf].modifiable = true
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
    vim.bo[buf].modifiable = false
  end

  local _, restore, set_hints =
    ui.set_tab_title(buf, "jmux pr view", { ui.key_hint("⇥", "expand"), ui.key_hint("q", "close") })
  conv.on_close.add(restore)
  conv.on_close.add(watch.clear)
  conv.on_close.add(function()
    pv_tab = nil
    pv_refresh = nil
  end)
  vim.keymap.set("n", "q", "<cmd>tabclose<cr>", { buffer = buf, nowait = true, desc = "close" })

  local function draw_loader(frame)
    if not vim.api.nvim_buf_is_valid(buf) then
      return
    end
    vim.bo[buf].modifiable = true
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, { frame .. " Loading PR…" })
    vim.bo[buf].modifiable = false
  end
  draw_loader(ui.SPINNER[1])
  -- show the loader before the first timer tick.
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

    -- outside fetch_and_render so expand state survives a refresh.
    local expanded = { pr = true }

    local start_watch

    local function fetch_and_render(stop_spin, cached)
      local prev_line = (vim.api.nvim_win_is_valid(win) and vim.api.nvim_win_get_cursor(win)[1]) or 1

      -- finish renders one response and wires the keys; age is the cache's age
      -- in seconds, nil for a live fetch.
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
          if not age then
            state.write_view_cache(vim.json.encode(data))
          end

          local model = build_model(data)
          local ns = vim.api.nvim_create_namespace "jmux_pr_view"
          local rows_by_id, id_by_row, jump_by_row, reviewer_by_row, check_by_row, subject_by_row =
            {}, {}, {}, {}, {}, {}

          local function render()
            local lines = vim.list_extend({}, model.header)
            rows_by_id, id_by_row, jump_by_row, reviewer_by_row, check_by_row, subject_by_row = {}, {}, {}, {}, {}, {}
            local marks = {}
            -- blank lines stay blank so markdown paragraph breaks survive.
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
                if model.jumps[b.id] then
                  jump_by_row[row + 1] = b.id
                end
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
                    -- three spaces: a four-space indent after a blank line is a
                    -- markdown code block.
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

            panes.reviewers.fill(model.reviewers)
            panes.checks.fill(model.checks)
            reviewer_by_row, check_by_row = {}, {}
            for i, e in ipairs(model.reviewers and model.reviewers.entries or {}) do
              reviewer_by_row[i - 1] = e
            end
            for i, e in ipairs(model.checks and model.checks.entries or {}) do
              check_by_row[i - 1] = e
            end
          end

          -- the buffer can outlive the captured win (:split then close the
          -- first), so every key resolves the window from the keypress.
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

          -- who_reacted fetches reactor logins on demand (they're not in the
          -- main query) and memoizes them for the render's lifetime.
          local function who_reacted()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= buf then
              return
            end
            local row = vim.api.nvim_win_get_cursor(w)[1] - 1
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
          -- keyed by id: the row resolvers run on every cursor move.
          local thread_by_id = {}
          for _, t in ipairs(vim.tbl_get(prnode or {}, "reviewThreads", "nodes") or {}) do
            if t.id then
              thread_by_id[t.id] = t
            end
          end

          local function cursor_row()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= buf then
              return nil
            end
            return vim.api.nvim_win_get_cursor(w)[1] - 1
          end

          local function thread_under_cursor()
            local row = cursor_row()
            local id = row and id_by_row[row]
            return id and thread_by_id[id] or nil
          end

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

          local function check_under_cursor()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= panes.checks.buf then
              return nil
            end
            local e = check_by_row[vim.api.nvim_win_get_cursor(w)[1] - 1]
            return e and e.url and e or nil
          end

          local function reviewer_under_cursor()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= panes.reviewers.buf then
              return nil
            end
            return reviewer_by_row[vim.api.nvim_win_get_cursor(w)[1] - 1]
          end

          local function edit_target()
            local row = cursor_row()
            if not row then
              return nil
            end
            if row <= 1 then
              return "title"
            end
            if id_by_row[row] == "pr" or (subject_by_row[row] ~= nil and subject_by_row[row] == model.subjects.pr) then
              return "description"
            end
          end

          -- on a reviewer's row R re-requests them; elsewhere it opens the picker.
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

          local function resolve()
            local t = thread_under_cursor()
            if not t then
              return
            end
            diff.resolve_thread({ id = t.id, resolved = t.isResolved == true }, function()
              fetch_and_render(ui.spin "refreshing…")
            end)
          end

          -- mine gates editing and merging: authored or assigned.
          local mine = prnode and prnode.viewerDidAuthor
          if prnode and not mine then
            for _, a in ipairs(vim.tbl_get(prnode, "assignees", "nodes") or {}) do
              if a.login == viewer then
                mine = true
                break
              end
            end
          end

          local mst = merge.model(prnode, nilable(vim.tbl_get(data, "data", "repository", "viewerDefaultMergeMethod")))
          local how, flag, ready, armed = mst.how, mst.flag, mst.ready, mst.armed
          local pr_state = tostring(nilable(prnode.state) or ""):upper()
          local mergeable = mine and flag and pr_state == "OPEN"

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

          -- arm_auto enables server-side auto-merge, then watches so the merge
          -- is noticed and the workspace prompt still fires.
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

          -- what the watcher waited for, evaluated on every render.
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
            start_watch()
          end

          -- spec order is bar order.
          local keys = ui.keyspec {
            {
              lhs = "m",
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
              lhs = "i",
              rhs = function()
                local what = edit_target()
                if what == "title" then
                  edit_title(pr, nilable(prnode and prnode.title) or "", function()
                    fetch_and_render(ui.spin "refreshing…")
                  end)
                elseif what == "description" then
                  edit_description(pr, nilable(prnode and prnode.body) or "", function()
                    fetch_and_render(ui.spin "refreshing…")
                  end)
                end
              end,
              when = function()
                return mine
              end,
              ctx = function()
                local what = edit_target()
                return what and ("edit " .. what)
              end,
              desc = "edit title/description under cursor",
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
            {
              lhs = "<CR>",
              rhs = function()
                local e = check_under_cursor()
                if e then
                  vim.ui.open(e.url)
                end
              end,
              lhs_hint = "⏎",
              desc = "open check in browser",
              pane = "checks",
              ctx = function()
                return check_under_cursor() ~= nil and "open"
              end,
            },
            { lhs = "go", rhs = M.browser, desc = "open in browser" },
            {
              lhs = "gy",
              rhs = function()
                local url = nilable(prnode.url)
                  or ("https://github.com/%s/pull/%s"):format(pr.slug, tostring(pr.number))
                vim.fn.setreg("+", url)
                vim.notify("copied " .. url)
              end,
              hint = "copy url",
              desc = "copy PR url",
            },
            { lhs = "q", rhs = "<cmd>tabclose<cr>", hint = "close", desc = "close" },
          }

          local bar = ui.hintbar {
            keys = keys,
            set_hints = set_hints,
            state = hint_state,
            extra = function()
              return age and { ("%%#Comment#cached %s ago"):format(util.short_age(age)) } or {}
            end,
          }
          local function for_pane(name)
            return function(k)
              return k.pane == nil or k.pane == name
            end
          end
          keys.apply(buf, nil, for_pane "conversation")
          for name, pane in pairs(panes) do
            if vim.api.nvim_buf_is_valid(pane.buf) then
              keys.apply(pane.buf, nil, for_pane(name))
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

    -- silent refetch every 15s: a spinner per tick would flicker the bar.
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

    local cached_data, cached_age = state.read_view_cache()
    if cached_data then
      fetch_and_render(nil, { data = cached_data, age = cached_age })
    else
      fetch_and_render()
    end
  end)
end

-- browser opens the current branch's PR on github.com.
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
