-- The pv conversation view: the whole PR — description, checks, comments,
-- reviews, inline threads — rendered as a collapsible markdown tab, with
-- refresh, reply, reviewer requests, description editing, and merging.

local diff = require "jmux.pr.diff"
local gh = require "jmux.pr.gh"
local state = require "jmux.pr.state"
local ui = require "jmux.pr.ui"
local util = require "jmux.pr.util"

local nilable = util.nilable
local reltime = util.reltime

local M = {}

-- pv_tab is the `pv` view tab, tracked so re-invoking focuses the open tab
-- instead of stacking a duplicate.
local pv_tab

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
      reviewThreads(first:100){ nodes{ id isResolved isOutdated path line
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
  local state_str = nilable(node.conclusion) or nilable(node.state)
  if not state_str then
    return "●", "DiagnosticWarn"
  end
  state_str = tostring(state_str):upper()
  if state_str == "SUCCESS" then
    return "✓", "DiagnosticOk"
  elseif state_str == "PENDING" or state_str == "EXPECTED" then
    return "●", "DiagnosticWarn"
  elseif state_str == "NEUTRAL" or state_str == "SKIPPED" or state_str == "CANCELLED" or state_str == "STALE" then
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
  local icon, hl, state_str = "●", "DiagnosticWarn", "pending"
  if pr.isDraft then
    icon, hl, state_str = "○", "Comment", "draft"
  elseif mergeable == "CONFLICTING" or mss == "DIRTY" then
    icon, hl, state_str = "✗", "DiagnosticError", "conflicts"
  elseif decision == "CHANGES_REQUESTED" then
    icon, hl, state_str = "✗", "DiagnosticError", "changes requested"
  elseif mss == "CLEAN" then
    icon, hl, state_str = "✓", "DiagnosticOk", "ready to merge"
  elseif mss == "BEHIND" then
    icon, hl, state_str = "●", "DiagnosticWarn", "behind base"
  elseif mss == "UNSTABLE" then
    icon, hl, state_str = "●", "DiagnosticWarn", "checks failing"
  elseif decision == "REVIEW_REQUIRED" then
    icon, hl, state_str = "●", "DiagnosticWarn", "review required"
  elseif mss == "BLOCKED" then
    icon, hl, state_str = "●", "DiagnosticWarn", "blocked"
  elseif decision == "APPROVED" then
    icon, hl, state_str = "✓", "DiagnosticOk", "approved"
  end
  local parts = { approvals == 1 and "1 approval" or (approvals .. " approvals") }
  if unresolved > 0 then
    parts[#parts + 1] = unresolved .. " unresolved"
  end
  parts[#parts + 1] = state_str
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
      local dur = util.duration(c.startedAt, c.completedAt)
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
    if t.isOutdated then
      status = status .. " · outdated"
    end
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

-- request_review picks reviewers — assignable users or org teams — and adds
-- them via one `gh pr edit`. The author and anyone/any team already requested
-- are filtered out (excluded by login, excludedTeams by slug). Uses snacks'
-- picker directly rather than vim.ui.select so several reviewers can be
-- Tab-marked and confirmed together (<CR> on an unmarked item still single-picks).
-- on_done runs after success so the caller can refresh. spin, when given, is a
-- fn(label)→stop animating a loader through the fetch and the edit; without it
-- a notify marks the start.
local function request_review(pr, excluded, excludedTeams, on_done, spin)
  local owner = gh.owner_repo(pr.slug)
  local stop = spin and spin "fetching reviewers…" or nil
  if not stop then
    vim.notify "fetching reviewers…"
  end

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
    local stop2 = spin and spin("requesting " .. who .. "…") or nil
    -- --add-reviewer takes a comma-separated list, so any number of picks is
    -- still one gh call.
    vim.system(
      { "gh", "pr", "edit", tostring(pr.number), "--repo", pr.slug, "--add-reviewer", table.concat(values, ",") },
      { text = true },
      function(r2)
        vim.schedule(function()
          if stop2 then
            stop2()
          end
          if r2.code ~= 0 then
            vim.notify("request review: " .. vim.trim(r2.stderr or ""), vim.log.levels.ERROR)
            return
          end
          vim.notify("requested review from " .. who)
          if on_done then
            on_done()
          end
        end)
      end
    )
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
    if stop then
      stop()
    end
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
-- description can't be cleared from here — edit on the web for that. spin, when
-- given, is a fn(label)→stop animating a loader while the save is in flight.
local function edit_description(pr, body, on_done, spin)
  -- GitHub returns the body with CRLF endings; strip the \r so the edit buffer
  -- doesn't show a trailing ^M on every line.
  body = (body or ""):gsub("\r", "")
  ui.prompt_multiline(("edit #%d description"):format(pr.number), function(text)
    local stop = spin and spin "saving description…" or nil
    vim.system(
      { "gh", "pr", "edit", tostring(pr.number), "--repo", pr.slug, "--body-file", "-" },
      { stdin = text, text = true },
      function(res)
        vim.schedule(function()
          if stop then
            stop()
          end
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
  -- conversation both happen async, so the view shows instantly and the spinner
  -- keeps animating during any round trip.
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
    ui.set_tab_title(buf, "jmux pr view", { ui.key_hint("⇥", "expand"), ui.key_hint("q", "close") })
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
            return ui.spinner(function(frame)
              set_hints { ("%%#Title#%s %s"):format(frame, label) }
            end)
          end

          -- hints is filled below (it depends on `mine`), but declared here so
          -- spin's closure can capture it. spin is the loader for every
          -- pv-triggered request: the title spinner while in flight, and the
          -- hint bar restored on stop (a bare title_spinner stop would leave
          -- the last frame frozen in the bar).
          local hints = {}
          local function spin(label)
            local stop = title_spinner(label)
            return function()
              stop()
              set_hints(hints)
            end
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
            end, spin)
          end, { buffer = buf, nowait = true, desc = "request review" })

          -- r replies to the thread whose header row the cursor is on (threads
          -- are the kids whose id is the GraphQL thread id; other rows no-op).
          -- Replies post immediately, then the view refreshes to show them.
          vim.keymap.set("n", "r", function()
            local w = vim.api.nvim_get_current_win()
            if vim.api.nvim_win_get_buf(w) ~= buf then
              return
            end
            local id = id_by_row[vim.api.nvim_win_get_cursor(w)[1] - 1]
            if not id then
              return
            end
            for _, t in ipairs(vim.tbl_get(prnode or {}, "reviewThreads", "nodes") or {}) do
              if t.id == id then
                local root = vim.tbl_get(t, "comments", "nodes", 1) or {}
                diff.reply_to_thread({
                  id = t.id,
                  path = t.path,
                  line = nilable(t.line),
                  comments = { { login = vim.tbl_get(root, "author", "login") or "?" } },
                }, function()
                  fetch_and_render(title_spinner "refreshing…")
                end, spin)
                return
              end
            end
          end, { buffer = buf, nowait = true, desc = "reply to thread" })

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
              end, spin)
            end, { buffer = buf, nowait = true, desc = "edit description" })
          end

          -- m merges — only on open PRs you authored or are assigned to, using the
          -- repo's default merge method. Hints are rebuilt here every render, so the
          -- merge spinner is cleanly replaced once it resolves.
          hints = { ui.key_hint("<C-r>", "refresh"), ui.key_hint("R", "request"), ui.key_hint("r", "reply") }
          if mine then
            hints[#hints + 1] = ui.key_hint("E", "edit")
          end
          hints[#hints + 1] = ui.key_hint("⇥", "expand")
          hints[#hints + 1] = ui.key_hint("q", "close")
          -- a cached paint flags its age so it's clear the data may be stale and
          -- <C-r> will refetch; a live fetch (age nil) shows no such marker.
          if age then
            hints[#hints + 1] = ("%%#Comment#cached %s ago"):format(util.short_age(age))
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
                  local stop_spin2 = title_spinner(("merging #%d…"):format(pr.number))
                  local cmd = { "gh", "pr", "merge", "--repo", pr.slug, flag }
                  if admin then
                    cmd[#cmd + 1] = "--admin"
                  end
                  cmd[#cmd + 1] = "--"
                  cmd[#cmd + 1] = tostring(pr.number)
                  vim.system(cmd, { text = true }, function(res)
                    vim.schedule(function()
                      if res.code ~= 0 then
                        stop_spin2()
                        set_hints(hints)
                        vim.notify("merge failed:\n" .. vim.trim(res.stderr or ""), vim.log.levels.ERROR)
                      else
                        vim.notify(("merged #%d (%s)"):format(pr.number, label))
                        -- keep the spinner up until the refreshed view renders.
                        fetch_and_render(stop_spin2)
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
            table.insert(hints, 1, ui.key_hint("M", "merge (admin)"))
            table.insert(hints, 1, ui.key_hint("m", "merge"))
          end
          set_hints(hints)

          pcall(vim.api.nvim_win_set_cursor, win, { math.min(prev_line, vim.api.nvim_buf_line_count(buf)), 0 })
        end)
      end

      if cached then
        finish(cached.data, nil, cached.age)
      else
        gh.graphql(VIEW_QUERY, { owner = owner, repo = repo, number = pr.number }, function(data, err2)
          finish(data, err2)
        end)
      end
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
