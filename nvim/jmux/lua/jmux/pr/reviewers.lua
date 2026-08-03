-- Per-reviewer status derived from the pv query's PR node: a verdict for
-- everyone who reviewed, plus who's still awaited. Pure model, no rendering —
-- pv shows it as a collapsible block today, and a future dashboard pane can
-- render the same lines/marks into its own window.

local util = require "jmux.pr.util"

local nilable = util.nilable
local reltime = util.reltime

local M = {}

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

-- model distils the PR node into { header, header_mark, lines, marks }, or nil
-- when the PR has no reviewers in any state. lines/marks use the body_marks
-- shape ({ rel, col, col_end, hl }) so any renderer can indent and place them.
function M.model(pr)
  local author = nilable(vim.tbl_get(pr, "author", "login"))
  local entries, seen = {}, {}

  -- latestOpinionatedReviews is the current approve/request-changes per
  -- author, matching how GitHub tallies verdicts.
  for _, r in ipairs(vim.tbl_get(pr, "latestOpinionatedReviews", "nodes") or {}) do
    local login = nilable(vim.tbl_get(r, "author", "login"))
    local state = tostring(r.state or ""):upper()
    if login and STATES[state] then
      seen[login] = true
      entries[#entries + 1] = { name = "@" .. login, state = state, at = nilable(r.createdAt) }
    end
  end

  -- reviewers whose latest word was only a comment: any review author with no
  -- opinionated verdict (skipping the PR author's own review comments).
  local commented = {}
  for _, r in ipairs(vim.tbl_get(pr, "reviews", "nodes") or {}) do
    local login = nilable(vim.tbl_get(r, "author", "login"))
    if login and not seen[login] and login ~= author then
      -- keep the newest timestamp per author (reviews arrive time-ordered).
      commented[login] = nilable(r.createdAt) or commented[login]
    end
  end
  for login, at in pairs(commented) do
    entries[#entries + 1] = { name = "@" .. login, state = "COMMENTED", at = at }
  end

  -- GitHub drops a request once that reviewer submits, so requests are the
  -- awaited set by definition. Teams render by name and never have verdicts.
  for _, rr in ipairs(vim.tbl_get(pr, "reviewRequests", "nodes") or {}) do
    local r = rr.requestedReviewer
    if type(r) == "table" then
      local name = r.login and ("@" .. r.login) or nilable(r.name) or nilable(r.slug)
      if name then
        entries[#entries + 1] = { name = name, state = "AWAITING" }
      end
    end
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
  }
end

return M
