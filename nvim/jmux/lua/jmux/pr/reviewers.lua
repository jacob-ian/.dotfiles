-- Per-reviewer status derived from the pv query's PR node: a verdict for
-- everyone who reviewed, plus who's still awaited. Pure model, no rendering —
-- pv shows it as a collapsible block today, and a future dashboard pane can
-- render the same lines/marks into its own window.

local util = require "jmux.pr.util"

local nilable = util.nilable
local reltime = util.reltime

local M = {}

-- FIELDS is the PR selection this model reads (view's request picker also
-- reads reviewRequests for its exclusions).
M.FIELDS = [[reviewRequests(first:50){ nodes{ requestedReviewer{
        __typename ... on User{ login } ... on Team{ name slug } } } }
      latestOpinionatedReviews(first:50){ nodes{ state author{login} createdAt } }]]

-- Display order: blocking verdicts first, then approvals, comment-only
-- reviewers, and the still-awaited last. signal ranks what the collapsed
-- header's icon should convey — blocked, then still-waiting, then done.
local STATES = {
  CHANGES_REQUESTED = { order = 1, signal = 1, icon = "✗", hl = "DiagnosticError", label = "changes requested" },
  APPROVED = { order = 2, signal = 3, icon = "✓", hl = "DiagnosticOk", label = "approved" },
  COMMENTED = { order = 3, signal = 4, icon = "●", hl = "Comment", label = "commented" },
  AWAITING = { order = 4, signal = 2, icon = "◷", hl = "DiagnosticWarn", label = "awaiting" },
}
local ORDERED = { "CHANGES_REQUESTED", "APPROVED", "COMMENTED", "AWAITING" }

-- model distils the PR node into { header, header_mark, lines, marks,
-- entries }, or nil when the PR has no reviewers in any state. lines/marks
-- use the body_marks shape ({ rel, col, col_end, hl }) so any renderer can
-- indent and place them; entries[i] describes lines[i]'s reviewer (state,
-- login/slug) for renderers that act on a reviewer row.
function M.model(pr)
  local author = nilable(vim.tbl_get(pr, "author", "login"))
  local entries, seen = {}, {}

  -- one entry per reviewer, first writer wins — so each add_* below
  -- supersedes the ones after it.
  local function add(login, entry)
    if login then
      if seen[login] then
        return
      end
      seen[login] = true
    end
    entries[#entries + 1] = entry
  end

  -- Requests supersede verdicts: GitHub keeps a re-requested reviewer's old
  -- reviews, and their actionable state is the new ask. Teams render by name
  -- and never have verdicts.
  for _, rr in ipairs(vim.tbl_get(pr, "reviewRequests", "nodes") or {}) do
    local r = rr.requestedReviewer
    if type(r) == "table" then
      local name = r.login and ("@" .. r.login) or nilable(r.name) or nilable(r.slug)
      if name then
        add(nilable(r.login), { name = name, state = "AWAITING", login = nilable(r.login), slug = nilable(r.slug) })
      end
    end
  end

  -- latestOpinionatedReviews is the current approve/request-changes per
  -- author, matching how GitHub tallies verdicts.
  for _, r in ipairs(vim.tbl_get(pr, "latestOpinionatedReviews", "nodes") or {}) do
    local login = nilable(vim.tbl_get(r, "author", "login"))
    local state = tostring(r.state or ""):upper()
    if login and STATES[state] then
      add(login, { name = "@" .. login, state = state, at = nilable(r.createdAt), login = login })
    end
  end

  -- reviewers whose latest word was only a comment: any review author with no
  -- opinionated verdict (skipping the PR author's own review comments).
  local newest = {}
  for _, r in ipairs(vim.tbl_get(pr, "reviews", "nodes") or {}) do
    local login = nilable(vim.tbl_get(r, "author", "login"))
    if login and login ~= author then
      -- reviews arrive time-ordered, so the last write is the newest.
      newest[login] = nilable(r.createdAt) or newest[login]
    end
  end
  for login, at in pairs(newest) do
    add(login, { name = "@" .. login, state = "COMMENTED", at = at, login = login })
  end

  if #entries == 0 then
    return nil
  end

  table.sort(entries, function(a, b)
    if STATES[a.state].order ~= STATES[b.state].order then
      return STATES[a.state].order < STATES[b.state].order
    end
    return a.name < b.name
  end)

  local lines, marks, counts = {}, {}, {}
  for _, e in ipairs(entries) do
    local s = STATES[e.state]
    counts[e.state] = (counts[e.state] or 0) + 1
    local line = ("%s %s · %s"):format(s.icon, e.name, s.label)
    if e.at then
      line = line .. " · " .. reltime(e.at)
    end
    lines[#lines + 1] = line
    marks[#marks + 1] = { rel = #lines - 1, col = 0, col_end = #s.icon, hl = s.hl }
  end

  -- summary mirrors the display order; the header icon carries the highest
  -- signal present, so a collapsed block still says where review stands.
  local summary, head = {}, nil
  for _, state in ipairs(ORDERED) do
    if counts[state] then
      summary[#summary + 1] = ("%d %s"):format(counts[state], STATES[state].label)
      if not head or STATES[state].signal < STATES[head].signal then
        head = state
      end
    end
  end

  local hs = STATES[head]
  return {
    header = ("%s Reviewers · %s"):format(hs.icon, table.concat(summary, " · ")),
    header_mark = { col = 0, col_end = #hs.icon, hl = hs.hl },
    lines = lines,
    marks = marks,
    entries = entries,
  }
end

return M
