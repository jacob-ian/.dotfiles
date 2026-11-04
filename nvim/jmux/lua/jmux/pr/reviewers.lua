-- Per-reviewer status as a pure model: a verdict for everyone who reviewed,
-- plus who's still awaited.

local util = require "jmux.pr.util"

local nilable = util.nilable
local reltime = util.reltime

local M = {}

M.FIELDS = [[reviewRequests(first:50){ nodes{ requestedReviewer{
        __typename ... on User{ login } ... on Team{ name slug } } } }
      latestOpinionatedReviews(first:50){ nodes{ state author{login} createdAt } }]]

-- order sorts the rows; signal ranks which state the header icon conveys.
local STATES = {
  CHANGES_REQUESTED = { order = 1, signal = 1, icon = "✗", hl = "DiagnosticError", label = "changes requested" },
  APPROVED = { order = 2, signal = 3, icon = "✓", hl = "DiagnosticOk", label = "approved" },
  COMMENTED = { order = 3, signal = 4, icon = "●", hl = "Comment", label = "commented" },
  AWAITING = { order = 4, signal = 2, icon = "◷", hl = "DiagnosticWarn", label = "awaiting" },
}
local ORDERED = { "CHANGES_REQUESTED", "APPROVED", "COMMENTED", "AWAITING" }

-- model distils the PR node into pane lines, or nil when the PR has no
-- reviewers in any state.
function M.model(pr)
  local author = nilable(vim.tbl_get(pr, "author", "login"))
  local entries, seen = {}, {}

  -- first writer wins, so each pass below supersedes the ones after it.
  local function add(login, entry)
    if login then
      if seen[login] then
        return
      end
      seen[login] = true
    end
    entries[#entries + 1] = entry
  end

  -- GitHub keeps a re-requested reviewer's old reviews, so requests supersede
  -- verdicts.
  for _, rr in ipairs(vim.tbl_get(pr, "reviewRequests", "nodes") or {}) do
    local r = rr.requestedReviewer
    if type(r) == "table" then
      local name = r.login and ("@" .. r.login) or nilable(r.name) or nilable(r.slug)
      if name then
        add(nilable(r.login), { name = name, state = "AWAITING", login = nilable(r.login), slug = nilable(r.slug) })
      end
    end
  end

  for _, r in ipairs(vim.tbl_get(pr, "latestOpinionatedReviews", "nodes") or {}) do
    local login = nilable(vim.tbl_get(r, "author", "login"))
    local state = tostring(r.state or ""):upper()
    if login and STATES[state] then
      add(login, { name = "@" .. login, state = state, at = nilable(r.createdAt), login = login })
    end
  end

  local newest = {}
  for _, r in ipairs(vim.tbl_get(pr, "reviews", "nodes") or {}) do
    local login = nilable(vim.tbl_get(r, "author", "login"))
    if login and login ~= author then
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

  local head
  for _, state in ipairs(ORDERED) do
    if counts[state] and (not head or STATES[state].signal < STATES[head].signal) then
      head = state
    end
  end

  local icon, hl = STATES[head].icon, STATES[head].hl
  -- reviewDecision reflects the branch's required approvals, so awaited
  -- reviewers don't hold the icon back once those are met.
  if tostring(nilable(pr.reviewDecision) or ""):upper() == "APPROVED" and head ~= "CHANGES_REQUESTED" then
    icon, hl = STATES.APPROVED.icon, STATES.APPROVED.hl
  end
  return {
    header = ("%s Reviewers"):format(icon),
    header_mark = { col = 0, col_end = #icon, hl = hl },
    lines = lines,
    marks = marks,
    entries = entries,
  }
end

return M
